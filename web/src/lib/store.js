import { writable, derived, get } from 'svelte/store';
import { api, getWorkspace, setWorkspace } from './api.js';

export const workspaces = writable([]); // the caller's memberships
export const activeWorkspace = writable(null); // the one everything is scoped to
// What the active workspace calls its AI actor (Settings → Workspace).
export const aiName = derived(activeWorkspace, (w) => w?.aiName || 'Clanker');
export const states = writable([]);
export const projects = writable([]);
export const initiatives = writable([]);
export const labels = writable([]);
// "Blocked by" links in the workspace: { issueId, blockerId, done }.
export const blockLinks = writable([]);
export async function loadBlockLinks() {
	blockLinks.set((await api.get('/blockers').catch(() => [])) || []);
}
// The blockers of each ticket still in its way, by issue id.
export function openBlockersByIssue(links) {
	const m = {};
	for (const l of links) if (!l.done) (m[l.issueId] ||= []).push(l.blockerId);
	return m;
}
export const issues = writable([]);
export const appConfig = writable({});
export const me = writable(null);
export const activeProject = writable(''); // '' = all (epic-level filter)
export const activeInitiative = writable(''); // '' = all (Project-level filter)
export const activeLabel = writable(''); // '' = all (label filter)
export const inboxCount = writable(0); // needs-review queue size (sidebar badge)
export const issueQuery = writable(''); // the search box above every issue view

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
	setWorkspace(target.slug);
	activeWorkspace.set(target);
	activeInitiative.set('');
	activeProject.set('');
	activeLabel.set('');
	issues.set([]);
	// Remember the choice server-side so a new session lands here too.
	api.activateWorkspace(target.id).catch(() => {});
	await loadMeta();
	await loadIssues();
}

export async function loadMeta() {
	const [st, pr, ini, lb, cfg] = await Promise.all([
		api.states(),
		api.projects(),
		api.initiatives(),
		api.labels(),
		api.config()
	]);
	loadBlockLinks();
	states.set(st || []);
	projects.set(pr || []);
	initiatives.set(ini || []);
	labels.set(lb || []);
	appConfig.set(cfg || {});
}

export async function loadIssues() {
	const initiative = get(activeInitiative);
	const project = get(activeProject);
	const label = get(activeLabel);
	const f = {};
	if (initiative) f.initiative = initiative;
	else if (project) f.project = project;
	if (label) f.label = label;
	const list = await api.issues(f);
	issues.set(list || []);
}

// applyEvent reconciles a live SSE event into the issues store.
export function applyEvent(ev) {
	if (!ev) return;
	// A ticket moving can free (or re-block) the tickets it blocks.
	if (ev.type === 'issue.blockers' || ev.type === 'issue.state_changed') loadBlockLinks();
	if (ev.type === 'issue.deleted') {
		issues.update((l) => l.filter((i) => i.id !== ev.issueId));
		return;
	}
	if (ev.issue) {
		issues.update((l) => {
			const idx = l.findIndex((i) => i.id === ev.issue.id);
			if (idx >= 0) {
				const copy = [...l];
				copy[idx] = ev.issue;
				return copy;
			}
			return [...l, ev.issue];
		});
	}
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
