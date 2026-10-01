import { writable, derived, get } from 'svelte/store';
import { api, getWorkspace, setWorkspace, notify } from './api.js';
import { belongsInView, coalesce } from './live.js';

export const workspaces = writable([]); // the caller's memberships
export const activeWorkspace = writable(null); // the one everything is scoped to
// What the active workspace calls its AI actor (Settings → Workspace).
export const aiName = derived(activeWorkspace, (w) => w?.aiName || 'Clanker');
export const states = writable([]);
export const projects = writable([]); // active epics only; archived ones are hidden
export const archivedProjects = writable([]);
export const initiatives = writable([]);
export const labels = writable([]);
export const labelGroups = writable([]); // { id, name, exclusive }
// The quick-capture chips (bug/feature/chore/tech-debt) are the workspace's
// "type" group specifically — every other group (repo, platform, …) is left
// to the full label picker on the issue page.
export const typeLabels = derived([labels, labelGroups], ([ls, groups]) => {
	const typeGroup = groups.find((g) => g.name === 'type');
	return typeGroup ? ls.filter((l) => l.groupId === typeGroup.id) : [];
});
// "Blocked by" links in the workspace: { issueId, blockerId, done }.
export const blockLinks = writable([]);
export async function loadBlockLinks() {
	const gen = wsGen;
	try {
		const list = (await api.get('/blockers')) || [];
		if (gen === wsGen) blockLinks.set(list);
	} catch {
		/* keep the links we have rather than blanking every blocked badge */
	}
}
// The blockers of each ticket still in its way, by issue id.
export function openBlockersByIssue(links) {
	const m = {};
	for (const l of links) if (!l.done) (m[l.issueId] ||= []).push(l.blockerId);
	return m;
}
// Generation counters: every load remembers the generation it started in and
// drops its result if a newer one began meanwhile (a second workspace switch,
// a filter changed again), so a slow old response never overwrites a newer one.
let wsGen = 0; // bumped by every workspace switch
let metaGen = 0;
let issuesGen = 0;
export const issues = writable([]);
// Every issue in the workspace, whatever the epic / label filter says. The
// sidebar's per-epic totals read this one, and live events keep it current, so
// a drag never has to refetch the list to update a count.
export const allIssues = writable([]);
// Issues with a move in flight: live events for them are ignored until the move
// returns, so a stale echo cannot undo the optimistic position.
export const movingIds = new Set();
export const appConfig = writable({});
export const me = writable(null);
export const activeProject = writable(''); // '' = all (epic-level filter)
export const activeInitiative = writable(''); // '' = all (Project-level filter)
export const activeLabel = writable(''); // '' = all (label filter)
export const inboxCount = writable(0); // needs-review queue size (sidebar badge)
// True from the moment a workspace switch starts until the server has recorded
// it. The live stream stays closed meanwhile (see the layout).
export const switching = writable(false);

// inboxTotal is the one inbox formula: what needs review plus what is waiting.
export function inboxTotal(r) {
	return (r?.needsReview || []).length + (r?.waiting || []).length;
}
// refreshInbox re-reads the inbox and updates the badge. Returns the inbox so
// the inbox page can render from the same call.
export async function refreshInbox() {
	const r = await api.inbox();
	inboxCount.set(inboxTotal(r));
	return r;
}
export const issueQuery = writable(''); // the search box above every issue view

// workspaceCounts mirrors inboxCount but for every membership at once, keyed
// by workspace id — what the workspace switcher's badge shows. Same signal
// (needs-review + waiting): "things that need you", not a raw issue total.
export const workspaceCounts = writable({});
export async function loadWorkspaceCounts() {
	const list = get(workspaces);
	const entries = await Promise.all(
		list.map(async (w) => {
			try {
				const r = await api.get('/inbox', w.slug);
				return [w.id, inboxTotal(r)];
			} catch {
				return [w.id, 0];
			}
		})
	);
	workspaceCounts.set(Object.fromEntries(entries));
}

// visibleIssues is the issue list narrowed by the search box, by title or key.
export const visibleIssues = derived([issues, issueQuery], ([list, q]) => {
	const needle = q.trim().toLowerCase();
	if (!needle) return list;
	return list.filter((i) => i.title.toLowerCase().includes(needle) || i.key.toLowerCase().includes(needle));
});

// loadWorkspaces resolves which workspaces the caller can reach and settles on
// one. It must run before loadMeta: every other request is scoped to the result.
export async function loadWorkspaces() {
	const list = (await api.workspaces()) || [];
	workspaces.set(list);
	if (!list.length) {
		activeWorkspace.set(null);
		setWorkspace('');
		return null;
	}
	// Prefer the stored choice, but only if it is still one of ours.
	const stored = getWorkspace();
	const chosen = list.find((w) => w.slug === stored || w.id === stored) || list[0];
	activeWorkspace.set(chosen);
	setWorkspace(chosen.slug);
	return chosen;
}

// switchWorkspace changes what every view is looking at. The per-workspace
// filters are cleared because their ids belong to the workspace being left.
export async function switchWorkspace(slug) {
	const list = get(workspaces);
	const target = list.find((w) => w.slug === slug || w.id === slug);
	if (!target) return;
	const gen = ++wsGen;
	setWorkspace(target.slug);
	activeWorkspace.set(target);
	activeInitiative.set('');
	activeProject.set('');
	activeLabel.set('');
	issues.set([]);
	allIssues.set([]);
	issueQuery.set('');
	// Remember the choice server-side so a new session lands here too, and only
	// then let the live stream reopen on the new workspace.
	switching.set(true);
	try {
		await api.activateWorkspace(target.id);
	} catch {
		/* the X-Workspace header already scopes every request */
	} finally {
		if (gen === wsGen) switching.set(false);
	}
	if (gen !== wsGen) return; // the user already moved on to another workspace
	try {
		await loadMeta();
		await loadIssues();
	} catch (e) {
		if (e?.status !== 401) notify("Couldn't load the workspace: " + (e?.message || e));
		return;
	}
	if (gen === wsGen) refreshInbox().catch(() => {});
}

// loadMeta loads the workspace's states, epics, initiatives, labels and config
// side by side. One failing part does not blank the others: the rest is shown
// and the toast names what is missing. Only when every part fails does it throw.
export async function loadMeta() {
	const gen = wsGen;
	const mine = ++metaGen;
	const parts = {
		states: api.states(),
		epics: Promise.all([api.projects(), api.projects('', '1').catch(() => [])]),
		initiatives: api.initiatives(),
		labels: Promise.all([api.labels(), api.labelGroups()]),
		settings: api.config()
	};
	const names = Object.keys(parts);
	const results = await Promise.allSettled(Object.values(parts));
	if (gen !== wsGen || mine !== metaGen) return; // stale: a newer load owns the stores
	const got = Object.fromEntries(names.map((n, i) => [n, results[i]]));
	const failed = names.filter((n) => got[n].status === 'rejected');
	if (failed.length === names.length) throw got[names[0]].reason;
	loadBlockLinks();
	if (!failed.includes('states')) states.set(got.states.value || []);
	if (!failed.includes('epics')) {
		projects.set(got.epics.value[0] || []);
		archivedProjects.set(got.epics.value[1] || []);
	}
	if (!failed.includes('initiatives')) initiatives.set(got.initiatives.value || []);
	if (!failed.includes('labels')) {
		labels.set(got.labels.value[0] || []);
		labelGroups.set(got.labels.value[1] || []);
	}
	if (!failed.includes('settings')) appConfig.set(got.settings.value || {});
	if (failed.length) notify(`Couldn't load ${failed.join(', ')}`);
}

// refreshAll also re-reads the unfiltered list; a filter change does not need it,
// live events keep that list current.
export async function loadIssues(refreshAll = false) {
	const initiative = get(activeInitiative);
	const project = get(activeProject);
	const label = get(activeLabel);
	const f = {};
	if (initiative) f.initiative = initiative;
	else if (project) f.project = project;
	if (label) f.label = label;
	const filtered = Object.keys(f).length > 0;
	const gen = wsGen;
	const mine = ++issuesGen;
	const stale = () => gen !== wsGen || mine !== issuesGen;
	try {
		const list = (await api.issues(f)) || [];
		if (stale()) return false;
		// Unfiltered, the one fetch serves both; filtered, the totals need their own.
		let all = list;
		if (filtered) all = refreshAll || get(allIssues).length === 0 ? (await api.issues()) || [] : get(allIssues);
		if (stale()) return false;
		issues.set(list);
		allIssues.set(all);
		return true;
	} catch (e) {
		// Keep what is on screen. A 401 is already on its way to the login page,
		// and a network failure has been announced by the API client.
		if (!stale() && e?.status !== 401 && !e?.network) notify("Couldn't load issues: " + (e?.message || e));
		return false;
	}
}

// catchUp refetches everything a live event could have changed. It runs after
// the stream reconnects, says `resync`, or the tab comes back; calls made while
// one is running share it.
export const catchUp = coalesce(async () => {
	await Promise.all([
		loadIssues(true),
		loadBlockLinks(),
		refreshInbox().catch(() => {}),
		loadWorkspaceCounts()
	]);
});

// applyEvent reconciles a live SSE event into the issues store. Events for
// another workspace are dropped, and an issue outside the active epic / label
// filter is removed from the list rather than added to it.
export function applyEvent(ev) {
	if (!ev) return;
	const wsId = get(activeWorkspace)?.id;
	const evWs = ev.workspaceId || ev.issue?.workspaceId;
	if (wsId && evWs && evWs !== wsId) return;
	// A ticket moving can free (or re-block) the tickets it blocks, and can
	// change what is waiting on the user.
	if (ev.type === 'issue.blockers' || ev.type === 'issue.state_changed') loadBlockLinks();
	if (ev.type === 'issue.state_changed') scheduleInboxRefresh();
	const id = ev.issue?.id || ev.issueId;
	if (id && movingIds.has(id)) return;
	if (ev.type === 'issue.deleted') {
		issues.update((l) => l.filter((i) => i.id !== ev.issueId));
		allIssues.update((l) => l.filter((i) => i.id !== ev.issueId));
		return;
	}
	if (ev.issue) {
		const fits = belongsInView(ev.issue, {
			workspaceId: wsId,
			project: get(activeProject),
			initiative: get(activeInitiative),
			label: get(activeLabel),
			projects: get(projects)
		});
		issues.update((l) => upsert(l, ev.issue, fits));
		allIssues.update((l) => upsert(l, ev.issue, true));
	}
}

// upsert puts one issue into a list (replacing it by id) or, when it does not
// belong, takes it out. Always returns a new array and never edits an issue.
function upsert(list, issue, keep) {
	const idx = list.findIndex((i) => i.id === issue.id);
	if (!keep) return idx >= 0 ? list.filter((i) => i.id !== issue.id) : list;
	if (idx < 0) return [...list, issue];
	const copy = [...list];
	copy[idx] = issue;
	return copy;
}

// moveIssueTo is a board drop: move the card optimistically, send one request,
// and on failure put the original back (and rethrow). prev / next are the cards
// above and below the drop point as the board shows them.
export async function moveIssueTo(id, stateId, prev, next) {
	const original = get(allIssues).find((x) => x.id === id) || get(issues).find((x) => x.id === id);
	if (!original) return null;
	const position = prev && next ? (prev.position + next.position) / 2 : prev ? prev.position + 1 : next ? next.position - 1 : 0;
	movingIds.add(id);
	replaceIssue({ ...original, stateId, position });
	try {
		const saved = await api.moveIssue(id, { state: stateId, after: prev?.id ?? null, before: next?.id ?? null });
		replaceIssue(saved);
		return { original, saved };
	} catch (err) {
		replaceIssue(original);
		throw err;
	} finally {
		movingIds.delete(id);
	}
}

// The sidebar badge follows state changes; a burst of them is one refresh.
let inboxTimer;
function scheduleInboxRefresh() {
	clearTimeout(inboxTimer);
	inboxTimer = setTimeout(() => refreshInbox().catch(() => {}), 500);
}

// replaceIssue swaps one issue in both lists without touching the others.
export function replaceIssue(issue) {
	issues.update((l) => l.map((i) => (i.id === issue.id ? issue : i)));
	allIssues.update((l) => l.map((i) => (i.id === issue.id ? issue : i)));
}

// archiveProject / unarchiveProject flip an epic's archived status, then
// reload the epics and issues so every view drops (or regains) it.
export async function archiveProject(id) {
	if (get(activeProject) === id) activeProject.set('');
	const p = await api.archiveProject(id);
	await loadMeta();
	await loadIssues();
	return p;
}

export async function unarchiveProject(id) {
	const p = await api.unarchiveProject(id);
	await loadMeta();
	await loadIssues();
	return p;
}

export function stateById(id) {
	return get(states).find((s) => s.id === id);
}

export function projectById(id) {
	return get(projects).find((p) => p.id === id);
}

export const PRIORITIES = [
	{ value: 0, label: 'No priority' },
	{ value: 1, label: 'Urgent' },
	{ value: 2, label: 'High' },
	{ value: 3, label: 'Medium' },
	{ value: 4, label: 'Low' }
];

// ---- Board-only filters: priority + a couple of client-side "saved views".
// Layered on top of visibleIssues (search, already shared with every issue
// view) rather than folded into it, so List/Tasks/Links keep their current
// behaviour untouched.
export const activePriority = writable(''); // '' = all, else 1..4
export const activeSavedView = writable(''); // '' | 'my-review' | 'blocked'

export const SAVED_VIEWS = [
	{ id: 'my-review', label: 'My In Review' },
	{ id: 'blocked', label: 'Blocked' }
];

export const boardVisibleIssues = derived(
	[visibleIssues, activePriority, activeSavedView, states, me],
	([list, pri, view, sts, meUser]) => {
		let out = list;
		if (pri !== '') out = out.filter((i) => i.priority === Number(pri));
		if (view === 'blocked') {
			const s = sts.find((x) => x.name.toLowerCase() === 'blocked');
			out = out.filter((i) => i.stateId === s?.id);
		} else if (view === 'my-review') {
			const s = sts.find((x) => x.name.toLowerCase() === 'in review');
			out = out.filter((i) => i.stateId === s?.id && meUser && i.assigneeId === meUser.id);
		}
		return out;
	}
);

export function clearBoardFilters() {
	activePriority.set('');
	activeSavedView.set('');
}
