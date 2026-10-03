<script>
	import { onMount } from 'svelte';
	import { aiName, activeWorkspace } from '$lib/store.js';
	import { gateFailure } from '$lib/gate.js';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { safeHref } from '$lib/url.js';
	import { states, projects, archivedProjects, blockLinks, unarchiveProject } from '$lib/store.js';
	import { onLive, showToast, blockedReasonFor } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';
	import InlineCode from '$components/InlineCode.svelte';
	import StateIcon from '$components/StateIcon.svelte';
	import PriorityMenu from '$components/PriorityMenu.svelte';
	import StatusMenu from '$components/StatusMenu.svelte';
	import EpicMenu from '$components/EpicMenu.svelte';
	import LabelPicker from '$components/LabelPicker.svelte';
	import IssueTimeline from '$components/IssueTimeline.svelte';
	import Blockers from '$components/Blockers.svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import Tick from '$components/Tick.svelte';
	import { rel } from '$lib/format.js';

	let issue = $state(null);
	let notFound = $state(false);
	let loading = $state(false);
	let pane = $state('task'); // which half a narrow window shows
	const blocksCount = $derived(issue ? $blockLinks.filter((l) => l.blockerId === issue.id).length : 0);
	let docs = $state([]);
	let children = $state([]);
	let parent = $state(null);
	let criteria = $state([]);
	let commits = $state([]);
	let newCrit = $state('');
	const doneCrit = $derived(criteria.filter((c) => c.done).length);
	// judgment criteria are advisory — shown, but never the reason a move is blocked.
	const blockingOpen = $derived(criteria.filter((c) => !c.done && c.kind !== 'judgment').length);
	// The Approve banner: shown while the issue waits in In Review.
	const inReview = $derived($states.find((x) => x.id === issue?.stateId)?.name === 'In Review');
	const doneStateId = $derived($states.find((x) => x.name === 'Done')?.id);
	const approveBlocked = $derived(!doneStateId || criteria.length === 0 || blockingOpen > 0);
	let approving = $state(false);
	async function approve() {
		if (approveBlocked || approving) return;
		approving = true;
		try {
			await patch({ stateId: doneStateId });
		} finally {
			approving = false;
		}
	}
	// A criterion unticked after the issue moved to In Review/Done (allowed): warn.
	const reopened = $derived(
		blockingOpen > 0 && ['in review', 'done'].includes($states.find((s) => s.id === issue?.stateId)?.name?.toLowerCase())
	);
	let titleDraft = $state('');
	let confirmDel = $state(false);
	let delBtnEl = $state(null);
	let propsSheetOpen = $state(false);

	// Description: explicit Edit/Save/Cancel, never click-to-edit.
	let editingDesc = $state(false);
	let descDraft = $state('');
	// The issue's updatedAt when the edit started. If the server's updatedAt has
	// since moved on, something changed this issue while we were editing — the
	// one stale-edit signal we can detect without PP-185's server-side check.
	let descEditBase = $state(null);
	let descSaveState = $state('idle'); // idle | failed
	let savingDesc = $state(false);
	let justSavedDesc = $state(false);
	let descSavedTimer;
	// Title edit: the issue's updatedAt when the title field got focus, so a save
	// can tell whether the issue moved on meanwhile.
	let editingTitle = $state(false);
	let titleEditBase = $state(null);
	// The server refused a save, or we saw a newer edit while a draft was open.
	// The drafts are kept; the banner offers Reload (drop them) or Keep mine.
	let conflict = $state(false);
	// A secondary fetch (docs, criteria, commits...) failed: shown inline, the
	// page stays on the issue.
	let loadError = $state('');

	const stOf = (c) => $states.find((s) => s.id === c.stateId);
	const archivedEpic = $derived(issue ? $archivedProjects.find((p) => p.id === issue.projectId) : null);
	const epic = $derived(issue ? ($projects.find((p) => p.id === issue.projectId) ?? archivedEpic) : null);
	async function unarchiveEpic() {
		try {
			await unarchiveProject(archivedEpic.id);
			showToast('Epic unarchived');
		} catch (e) {
			showToast(e.message || 'Failed to unarchive', 'error');
		}
	}
	const doneChildren = $derived(children.filter((c) => stOf(c)?.category === 'completed').length);
	const DOC_ICON = { change: '⟳', feature: '◈', decision: '◆', overview: '◇', reference: '▤' };
	const descConflict = $derived(
		Boolean(editingDesc && descEditBase && issue?.updatedAt && issue.updatedAt !== descEditBase)
	);

	// Guards against the loads-race: navigating A → B must never let A's
	// slower response land after B's and overwrite it. Every load() captures
	// the sequence number current at its start and checks it's still current
	// before applying anything it fetched.
	let loadSeq = 0;

	// Reload whenever the :key param changes (also handles navigating between a
	// parent and its sub-issues).
	$effect(() => {
		const key = $page.params.key;
		if (key) load(key);
	});

	async function load(key) {
		const seq = ++loadSeq;
		loading = true;
		notFound = false;
		issue = null;
		confirmDel = false;
		editingDesc = false;
		editingTitle = false;
		conflict = false;
		loadError = '';
		descSaveState = 'idle';
		propsSheetOpen = false;

		let fresh;
		try {
			fresh = await api.issue(key);
		} catch (e) {
			if (seq !== loadSeq) return; // a newer navigation has since started
			loading = false;
			if (e.status === 404) {
				notFound = true;
			} else {
				showToast('Load failed: ' + e.message, 'error');
				goto('/board');
			}
			return;
		}
		if (seq !== loadSeq) return; // stale: a newer key was requested meanwhile

		issue = fresh;
		titleDraft = issue.title;
		descDraft = issue.descriptionMd || '';
		descEditBase = issue.updatedAt;
		loading = false;

		await loadSecondary(issue, seq);
	}

	// Secondary reads — docs, children, parent, criteria, commits. None of these
	// may redirect on failure: only a failed primary fetch does. The activity
	// timeline refreshes itself (it is keyed on the issue).
	async function loadSecondary(iss, seq) {
		const results = await Promise.allSettled([
			api.documents({ issue: iss.id }),
			iss.childCount > 0 ? api.issues({ parent: iss.key }) : Promise.resolve([]),
			iss.parentKey ? api.issue(iss.parentKey) : Promise.resolve(null),
			api.criteria(iss.id),
			api.commits(iss.id)
		]);
		if (seq !== loadSeq) return; // stale: this issue is no longer the one on screen
		const [d, c, p, cr, co] = results;
		docs = d.status === 'fulfilled' ? d.value || [] : [];
		children = c.status === 'fulfilled' ? c.value || [] : [];
		parent = p.status === 'fulfilled' ? p.value : null;
		criteria = cr.status === 'fulfilled' ? cr.value || [] : [];
		commits = co.status === 'fulfilled' ? co.value || [] : [];
		const failed = [['documents', d], ['sub-issues', c], ['parent', p], ['done-when', cr], ['commits', co]]
			.filter(([, r]) => r.status === 'rejected')
			.map(([n]) => n);
		loadError = failed.length ? `Could not load ${failed.join(', ')}.` : '';
	}

	// Keep the page live: a save from another tab, the MCP server, or the AI
	// updates this same record elsewhere. Refetch rather than trust the event
	// payload, and never clobber an in-progress description draft.
	async function refreshIssue() {
		if (!issue) return;
		const key = issue.key;
		const seq = loadSeq;
		let fresh;
		try {
			fresh = await api.issue(key);
		} catch {
			return; // a transient failure here shouldn't disturb what's on screen
		}
		if (!issue || issue.key !== key || seq !== loadSeq) return; // navigated away meanwhile
		issue = fresh;
		if (!editingTitle) titleDraft = issue.title;
		if (!editingDesc) {
			descDraft = issue.descriptionMd || '';
			descEditBase = issue.updatedAt;
		}
		// else: leave descDraft/descEditBase alone. The gap between descEditBase
		// and the fresh issue.updatedAt is exactly the stale-edit conflict.
		if (editingTitle && titleEditBase && issue.updatedAt !== titleEditBase) conflict = true;
		loadSecondary(issue, seq);
	}

	onMount(() =>
		onLive((ev) => {
			const id = ev.issue?.id || ev.issueId;
			if (!issue || id !== issue.id) return;
			if (ev.type === 'issue.deleted') {
				showToast(`${issue.key} was deleted`, 'error');
				goto('/board');
				return;
			}
			if (ev.type === 'issue.updated' || ev.type === 'issue.state_changed') refreshIssue();
		})
	);

	// A move the done-when gate refused: shown inline with the open items. An
	// owner/admin may push it through; the server records the override.
	let gateBlock = $state(null);
	const canForce = $derived(['owner', 'admin'].includes($activeWorkspace?.role));
	async function patch(body) {
		if (body.stateId && !body.blockedReason) {
			const reason = await blockedReasonFor(issue.stateId, body.stateId, issue.key);
			if (reason === null) return;
			if (reason) body = { ...body, blockedReason: reason };
		}
		try {
			issue = await api.updateIssue(issue.id, { expectedUpdatedAt: issue.updatedAt, ...body });
			gateBlock = null;
			conflict = false;
			// This is our own change, not a conflict — advance the edit's base
			// so an in-progress description edit doesn't get falsely flagged.
			if (editingDesc) descEditBase = issue.updatedAt;
		} catch (e) {
			if (staleFailure(e)) return;
			const g = gateFailure(e);
			if (g) {
				gateBlock = { ...g, body };
				return;
			}
			showToast('Update failed: ' + e.message, 'error');
		}
	}
	// The server refused a save because the issue changed since we loaded it.
	// Take the current issue so the next save is not stale, keep every draft, and
	// show the banner; nothing is overwritten until the user chooses.
	function staleFailure(e) {
		if (e.status !== 409 || e.code !== 'stale') return false;
		if (e.issue && e.issue.id === issue.id) issue = e.issue;
		conflict = true;
		return true;
	}
	function startEditTitle() {
		editingTitle = true;
		titleEditBase = issue.updatedAt;
	}
	async function saveTitle() {
		editingTitle = false;
		const next = titleDraft.trim();
		if (!next || next === issue.title) {
			if (!next) titleDraft = issue.title;
			return;
		}
		// Changed elsewhere since the field got focus: keep the draft, ask first.
		if (titleEditBase && issue.updatedAt !== titleEditBase) {
			conflict = true;
			return;
		}
		await patch({ title: next });
		if (conflict) editingTitle = true; // the draft is still unsaved
	}
	function cancelEditTitle() {
		titleDraft = issue.title;
		editingTitle = false;
		titleEditBase = null;
	}
	function titleKeydown(e) {
		if (e.key === 'Enter') {
			e.preventDefault();
			e.target.blur();
		} else if (e.key === 'Escape') {
			e.preventDefault();
			cancelEditTitle();
			e.target.blur();
		}
	}
	// Reload: drop every draft and show what is stored now.
	async function reloadFromServer() {
		conflict = false;
		editingTitle = false;
		editingDesc = false;
		descSaveState = 'idle';
		const key = issue.key;
		try {
			issue = await api.issue(key);
		} catch (e) {
			showToast('Reload failed: ' + e.message, 'error');
			return;
		}
		titleDraft = issue.title;
		descDraft = issue.descriptionMd || '';
		descEditBase = issue.updatedAt;
		loadSecondary(issue, loadSeq);
	}
	// Keep mine: the user has seen the newer edit and wants their title anyway.
	async function keepMyTitle() {
		titleEditBase = issue.updatedAt;
		conflict = false;
		await patch({ title: titleDraft.trim() });
	}

	function startEditDesc() {
		descDraft = issue.descriptionMd || '';
		descEditBase = issue.updatedAt;
		descSaveState = 'idle';
		editingDesc = true;
	}
	function cancelEditDesc() {
		editingDesc = false;
		descSaveState = 'idle';
	}
	async function saveDesc() {
		savingDesc = true;
		try {
			issue = await api.updateIssue(issue.id, { descriptionMd: descDraft, expectedUpdatedAt: issue.updatedAt });
			descEditBase = issue.updatedAt;
			editingDesc = false;
			conflict = false;
			descSaveState = 'idle';
			justSavedDesc = true;
			clearTimeout(descSavedTimer);
			descSavedTimer = setTimeout(() => (justSavedDesc = false), 2500);
		} catch (e) {
			// A failed save restores the draft: stay in edit mode with exactly
			// what was typed, rather than reverting to the last-saved text.
			if (!staleFailure(e)) descSaveState = 'failed';
		} finally {
			savingDesc = false;
		}
	}
	function descKeydown(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			cancelEditDesc();
		} else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			saveDesc();
		}
	}
	function reviewConflict() {
		descDraft = issue.descriptionMd || '';
		descEditBase = issue.updatedAt;
		editingDesc = false;
		descSaveState = 'idle';
	}

	async function del() {
		if (!confirmDel) {
			confirmDel = true;
			return;
		}
		try {
			await api.deleteIssue(issue.id);
			showToast(`${issue.key} deleted`);
			goto('/board');
		} catch (e) {
			showToast('Delete failed: ' + e.message, 'error');
			confirmDel = false;
		}
	}
	// Two-step delete cancels itself the moment you click anywhere else.
	$effect(() => {
		if (!confirmDel) return;
		function onDocClick(e) {
			if (delBtnEl && !delBtnEl.contains(e.target)) confirmDel = false;
		}
		document.addEventListener('click', onDocClick, true);
		return () => document.removeEventListener('click', onDocClick, true);
	});

	function autofocus(node) {
		node.focus();
	}
	async function addCrit() {
		if (!newCrit.trim()) return;
		try {
			const c = await api.addCriterion(issue.id, newCrit.trim());
			criteria = [...criteria, c];
			newCrit = '';
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function toggleCrit(c) {
		try {
			const u = await api.updateCriterion(c.id, { done: !c.done });
			criteria = criteria.map((x) => (x.id === c.id ? u : x));
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function delCrit(c) {
		try {
			await api.deleteCriterion(c.id);
			criteria = criteria.filter((x) => x.id !== c.id);
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	const shortSha = (s) => (s || '').slice(0, 7);
	// Only the PR url is stored, so the label is "#number" read from it.
	const prLabel = (u) => {
		const m = /\/pull\/(\d+)/.exec(u || '');
		return m ? '#' + m[1] : 'Pull request';
	};
</script>

{#snippet propsRows()}
	<div class="prow">
		<span class="plabel">Status</span>
		<span class="pval">
			<StatusMenu value={issue.stateId} onchange={(v) => patch({ stateId: v })} />
			{#if blockingOpen > 0}
				<span class="gate-badge" title="{blockingOpen} done-when item{blockingOpen === 1 ? '' : 's'} still open — judgment criteria don't count">
					{reopened ? 'Reopened: ' : ''}{blockingOpen} open
				</span>
			{/if}
		</span>
	</div>
	<div class="prow">
		<span class="plabel">Priority</span>
		<span class="pval"><PriorityMenu value={issue.priority} onchange={(v) => patch({ priority: v })} /></span>
	</div>
	<div class="prow">
		<span class="plabel">Epic</span>
		<span class="pval">
			<EpicMenu value={issue.projectId || ''} options={archivedEpic ? [...$projects, archivedEpic] : $projects} none="No epic" onchange={(v) => patch({ projectId: v })} />
		</span>
	</div>
	<div class="prow wide">
		<span class="plabel">Labels</span>
		<span class="pval"><LabelPicker selected={issue.labels.map((l) => l.id)} onchange={(ids) => patch({ labelIds: ids })} /></span>
	</div>
	{#if children.length}
		<div class="prow top">
			<span class="plabel">Sub-issues</span>
			<span class="pval">
				<div class="subwrap">
					<div class="subhead"><b>{doneChildren}/{children.length} done</b></div>
					<div class="sub-bar"><span style="width:{(doneChildren / children.length) * 100}%"></span></div>
					{#each children as c (c.id)}
						<button class="sub-link" onclick={() => goto('/issue/' + c.key)}>
							<StateIcon category={stOf(c)?.category} color={stOf(c)?.color} />
							<span class="sub-key">{c.key}</span>
							<span class="sub-title" class:done={stOf(c)?.category === 'completed'}>{c.title}</span>
						</button>
					{/each}
				</div>
			</span>
		</div>
	{/if}
	<div class="prow top">
		<span class="plabel">Blocked by</span>
		<span class="pval">{#key issue.id}<Blockers {issue} />{/key}</span>
	</div>
	{#if blocksCount}
		<div class="prow">
			<span class="plabel">Blocks</span>
			<span class="pval">{#key issue.id}<Blockers {issue} which="blocking" />{/key}</span>
		</div>
	{/if}
{/snippet}

{#if notFound}
	<div class="notfound-wrap">
		<div class="nf-card">
			<div class="nf-code">404 · {$page.params.key}</div>
			<div class="nf-title">Issue not found</div>
			<div class="nf-body">It doesn't exist, or you don't have access to it in this workspace.</div>
			<button class="btn sd" onclick={() => goto('/board')}>Back to board</button>
		</div>
	</div>
{:else if !issue}
	<div class="detail">
		<PageHeader crumbs={[{ label: 'Tasks', href: '/board' }, { label: $page.params.key, upper: false }]} />
		<div class="skel-main" aria-hidden="true">
			<span class="sk" style="width:30%;height:12px"></span>
			<span class="sk" style="width:65%;height:30px;margin-top:10px"></span>
			<span class="sk" style="width:100%;height:120px;margin-top:20px"></span>
			<span class="sk" style="width:90%;height:16px;margin-top:26px"></span>
			<span class="sk" style="width:60%;height:16px;margin-top:10px"></span>
			<span class="sk" style="width:100%;height:90px;margin-top:20px"></span>
		</div>
	</div>
{:else}
	<div class="detail">
		<PageHeader
			crumbs={[
				{ label: 'Tasks', href: '/board' },
				...(epic ? [{ label: epic.name, upper: false }] : []),
				{ label: issue.key, upper: false }
			]}
		>
			<button class="btn danger sm" bind:this={delBtnEl} onclick={del}>{confirmDel ? 'Confirm delete' : 'Delete'}</button>
		</PageHeader>

		{#if archivedEpic}
			<div class="banner warn" role="status">
				<span class="bic">▤</span>
				<span class="btext"><b>This epic is archived.</b> Its issues stay readable.</span>
				<span class="sp"></span>
				<button class="btn sd sm" onclick={unarchiveEpic}>Unarchive</button>
			</div>
		{/if}

		{#if gateBlock}
			<div class="gate-block" role="alert">
				<div class="gate-head">
					{gateBlock.code === 'criteria_missing'
						? `Can't move to ${gateBlock.state}: no done-when criteria yet.`
						: `Can't move to ${gateBlock.state}: ${gateBlock.open.length} done-when item${gateBlock.open.length === 1 ? '' : 's'} not ticked.`}
				</div>
				{#each gateBlock.open as o (o.index)}
					<div class="gate-item"><span class="gate-box"></span><span><span class="mono">{o.index}.</span> {o.text}</span></div>
				{/each}
				<div class="gate-block-actions">
					<button class="btn sm gho" onclick={() => (gateBlock = null)}>Got it</button>
					{#if canForce}
						<button class="btn sm danger" onclick={() => patch({ ...gateBlock.body, force: true })}>Move anyway</button>
					{/if}
				</div>
			</div>
		{/if}

		{#if conflict}
			<div class="conflict-banner top-conflict" role="alert">
				<span class="bic">✦</span>
				<span class="btext"><b>Changed elsewhere.</b> This issue was edited after you opened it. Your draft is kept.</span>
				<span class="sp"></span>
				<button class="btn sd sm" onclick={reloadFromServer}>Reload</button>
				{#if titleDraft.trim() && titleDraft.trim() !== issue.title}
					<button class="btn danger sm" onclick={keepMyTitle}>Keep my title</button>
				{/if}
			</div>
		{/if}

		{#if inReview}
			<div class="conflict-banner top-conflict" role="status">
				<span class="btext">
					<b>In review.</b>
					{#if criteria.length === 0}No done-when criteria yet.
					{:else if blockingOpen > 0}{blockingOpen} done-when item{blockingOpen === 1 ? '' : 's'} still open.
					{:else}All done-when items are done. Read the code, then approve.{/if}
				</span>
				<span class="sp"></span>
				<button class="btn primary sm" onclick={approve} disabled={approveBlocked || approving}>
					{approving ? 'Approving…' : 'Approve'}
				</button>
			</div>
		{/if}

		{#if loadError}
			<div class="banner danger" role="status"><span class="bic">⚠</span><span class="btext"><b>Couldn't load everything.</b> {loadError}</span></div>
		{/if}

		<div class="panes" role="tablist">
			<button role="tab" aria-selected={pane === 'task'} class:on={pane === 'task'} onclick={() => (pane = 'task')}>Task</button>
			<button role="tab" aria-selected={pane === 'chat'} class:on={pane === 'chat'} onclick={() => (pane = 'chat')}>
				Activity
			</button>
		</div>

		<div class="dbody" data-pane={pane}>
			<main class="dmain">
				<div class="dmain-inner">
					{#if parent}
						<button class="parent-crumb" onclick={() => goto('/issue/' + parent.key)}>
							<span class="pc-ic">⤴</span><span class="pc-key">{parent.key}</span>
							<span class="pc-title">{parent.title}</span>
						</button>
					{/if}

					<textarea
						class="textarea title-input"
						bind:value={titleDraft}
						rows="1"
						onfocus={startEditTitle}
						onblur={saveTitle}
						onkeydown={titleKeydown}
					></textarea>

					<div class="mobile-chiprow">
						<StatusMenu value={issue.stateId} onchange={(v) => patch({ stateId: v })} />
						{#if blockingOpen > 0}<span class="gate-badge">{reopened ? 'Reopened: ' : ''}{blockingOpen} open</span>{/if}
						<PriorityMenu value={issue.priority} onchange={(v) => patch({ priority: v })} />
					</div>
					<button class="propsbtn" onclick={() => (propsSheetOpen = true)}>
						Epic, labels, blockers…<span class="chev">▾</span>
					</button>

					<div class="panel desktop-panel">
						{@render propsRows()}
					</div>

					<div class="desc block">
						<div class="bh">
							<h2>Description</h2>
							<span class="sp"></span>
							{#if editingDesc}
								{#if justSavedDesc}<span class="saved">✓ Saved just now</span>{/if}
								{#if descSaveState === 'failed'}
									<span class="failed">⚠ Failed to save</span>
									<button class="retry-link" onclick={saveDesc}>Retry</button>
								{/if}
								<button class="btn ghost sm" onclick={cancelEditDesc}>Cancel</button>
								<button class="btn primary sm" onclick={saveDesc} disabled={savingDesc}>Save</button>
							{:else}
								{#if justSavedDesc}<span class="saved">✓ Saved just now</span>{/if}
								<button class="btn sd sm" onclick={startEditDesc}>Edit</button>
							{/if}
						</div>

						{#if descConflict}
							<div class="conflict-banner">
								<span class="bic">✦</span>
								<span class="btext"><b>{$aiName} updated this issue</b> while you were editing the description.</span>
								<span class="sp"></span>
								<button class="btn sd sm" onclick={reviewConflict}>Review changes</button>
								<button class="btn danger sm" onclick={saveDesc}>Overwrite</button>
							</div>
						{/if}

						{#if editingDesc}
							<textarea class="textarea desc-area editing" bind:value={descDraft} onkeydown={descKeydown} use:autofocus></textarea>
							<div class="edithint"><span class="kbd">Esc</span> to cancel · <span class="kbd">⌘</span><span class="kbd">↵</span> to save</div>
						{:else if issue.descriptionMd}
							<div class="prose"><Markdown source={issue.descriptionMd} /></div>
						{:else}
							<span class="faint">No description.</span>
						{/if}
					</div>

					<section class="block">
						<div class="bh">
							<h2>Done when</h2>{#if criteria.length}<span class="n">{doneCrit}/{criteria.length}</span>{/if}
						</div>
						{#if criteria.length}
							<div class="sub-bar"><span style="width:{(doneCrit / criteria.length) * 100}%"></span></div>
						{/if}
						{#each criteria as c (c.id)}
							<div class="crit">
								<button class="crit-box" class:on={c.done} onclick={() => toggleCrit(c)} aria-label="toggle">
									{#if c.done}<Tick size={12} />{/if}
								</button>
								<span class="crit-text" class:done={c.done}><InlineCode text={c.body} /></span>
								<span class="crit-kind" class:adv={c.kind === 'judgment'}>{c.kind}</span>
								{#if c.evidenceRef}<span class="crit-ev" title={c.evidenceRef}>{c.evidenceRef}</span>{/if}
								<button class="crit-del" onclick={() => delCrit(c)} title="Remove">✕</button>
							</div>
						{/each}
						<input
							class="input crit-add"
							bind:value={newCrit}
							placeholder="Add acceptance criterion…"
							onkeydown={(e) => e.key === 'Enter' && addCrit()}
						/>
					</section>

					{#if issue.gitBranch || issue.prUrl || commits.length}
						<section class="block">
							<div class="bh"><h2>Development</h2></div>
							{#if issue.gitBranch}
								<div class="dev-row"><span class="dev-lbl">branch</span><span class="mono">{issue.gitBranch}</span></div>
							{/if}
							{#if issue.prUrl}
								{#if safeHref(issue.prUrl)}
									<a class="dev-row link" href={safeHref(issue.prUrl)} target="_blank" rel="noopener noreferrer">
										<span class="dev-lbl">PR</span><span class="pr">{prLabel(issue.prUrl)}</span>
									</a>
								{:else}
									<div class="dev-row"><span class="dev-lbl">PR</span><span class="cmsg">{issue.prUrl}</span></div>
								{/if}
							{/if}
							{#each commits as c, i (c.id)}
								{#if safeHref(c.url)}
									<a class="dev-row link" href={safeHref(c.url)} target="_blank" rel="noopener noreferrer">
										<span class="dev-lbl">{i === 0 ? 'commit' : ''}</span><span class="mono sha">{shortSha(c.sha)}</span><span class="cmsg">{c.message}</span><span class="ctime">{rel(c.createdAt)}</span>
									</a>
								{:else}
									<div class="dev-row">
										<span class="dev-lbl">{i === 0 ? 'commit' : ''}</span><span class="mono sha">{shortSha(c.sha)}</span><span class="cmsg">{c.message}</span><span class="ctime">{rel(c.createdAt)}</span>
									</div>
								{/if}
							{/each}
						</section>
					{/if}

					{#if docs.length}
						<section class="block">
							<div class="bh"><h2>Documents</h2><span class="n">{docs.length}</span></div>
							{#each docs as d (d.id)}
								<button class="doc-link" onclick={() => goto(`/artifacts?doc=${d.id}`)}>
									<span class="dl-ic">{DOC_ICON[d.type] || '▤'}</span>
									<span class="dl-t">{d.title}</span>
									{#if d.author === 'ai'}<span class="dl-ai">✦ {$aiName}</span>{/if}
								</button>
							{/each}
						</section>
					{/if}

				</div>
			</main>

			<aside class="chat">
				{#key issue.id}<IssueTimeline {issue} />{/key}
			</aside>
		</div>
	</div>

	{#if propsSheetOpen}
		<div class="backdrop" role="presentation" onclick={() => (propsSheetOpen = false)}></div>
		<div class="sheet">
			<div class="handle"></div>
			<div class="sheethead"><b>Properties</b><span class="sp"></span><button onclick={() => (propsSheetOpen = false)}>Done</button></div>
			<div class="sheetrows panel in-sheet">
				{@render propsRows()}
			</div>
		</div>
	{/if}
{/if}

<style>
	.detail {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.dbody {
		flex: 1;
		display: flex;
		min-height: 0;
		overflow: hidden;
	}
	.dmain {
		flex: 1 1 0;
		min-width: 0;
		overflow-y: auto;
	}
	.dmain-inner {
		max-width: 820px;
		margin: 0 auto;
		padding: 28px 36px 64px;
		display: flex;
		flex-direction: column;
		gap: 22px;
	}
	@media (max-width: 640px) {
		.dmain-inner {
			padding: 20px 20px 48px;
		}
	}
	/* Comments, side by side with the task. */
	.chat {
		flex: 0 0 min(400px, 42%);
		min-width: 0;
		border-left: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		background: var(--paper);
	}

	/* Properties: one vertical panel — every field a row, dashed dividers. */
	.panel {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 6px 16px;
		display: flex;
		flex-direction: column;
	}
	.prow {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: 38px;
		padding: 7px 0;
		border-bottom: 1px dashed var(--line);
	}
	.prow:last-child {
		border-bottom: none;
	}
	.prow.top {
		align-items: flex-start;
	}
	.plabel {
		width: 96px;
		flex: none;
		font-size: var(--t-xs);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding-top: 2px;
	}
	.pval {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		font-size: var(--t-base);
		font-weight: 500;
	}
	/* The menus read as values, not form fields, until pointed at. Their .dd
	   wrapper has no intrinsic width, so it shrinks to its content unless told
	   to fill the row — give it the row's space before the button's own
	   width:100% (app.css) can use it. */
	.pval :global(.dd) {
		flex: 1;
		min-width: 0;
	}
	.panel :global(.dd-btn) {
		border-color: transparent;
		background: none;
		padding: 5px 7px;
		margin-left: -7px;
		font-size: var(--t-base);
		font-weight: 500;
	}
	.panel :global(.dd-btn:hover) {
		background: var(--hover);
	}
	.gate-badge {
		display: inline-flex;
		align-items: center;
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--danger);
		background: var(--danger-soft);
		border-radius: 999px;
		padding: 2px 9px;
		white-space: nowrap;
	}
	.subwrap {
		display: flex;
		flex-direction: column;
		gap: 6px;
		width: 100%;
		padding: 4px 0;
	}
	.subhead {
		font-size: var(--t-sm);
		color: var(--ink);
	}

	/* Mobile-only quick status/priority row + the properties-sheet trigger.
	   Hidden on desktop; the desktop .panel is hidden on mobile instead. */
	.mobile-chiprow,
	.propsbtn {
		display: none;
	}
	@media (max-width: 640px) {
		.desktop-panel {
			display: none;
		}
		.mobile-chiprow {
			display: flex;
			align-items: center;
			gap: 8px;
			flex-wrap: wrap;
		}
		.propsbtn {
			display: flex;
			align-items: center;
			justify-content: space-between;
			width: 100%;
			height: 38px;
			padding: 0 13px;
			border-radius: var(--r);
			border: 1px dashed var(--line-strong);
			background: var(--surface);
			color: var(--ink-2);
			font-size: var(--t-base);
		}
		.propsbtn .chev {
			color: var(--ink-3);
		}
	}
	.backdrop {
		position: fixed;
		inset: 0;
		background: oklch(0.1 0 0 / 0.42);
		z-index: 50;
	}
	.sheet {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		background: var(--surface);
		border-top-left-radius: var(--r-lg);
		border-top-right-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		padding: 10px 18px calc(18px + env(safe-area-inset-bottom, 0px));
		z-index: 51;
		display: flex;
		flex-direction: column;
		max-height: 74%;
		box-sizing: border-box;
	}
	.handle {
		width: 36px;
		height: 4px;
		border-radius: var(--r-sm);
		background: var(--line-strong);
		margin: 2px auto 12px;
	}
	.sheethead {
		display: flex;
		align-items: center;
		margin-bottom: 8px;
	}
	.sheethead b {
		font-size: var(--t-base);
		font-weight: 600;
	}
	.sheethead button {
		background: none;
		border: none;
		color: var(--accent);
		font-size: var(--t-sm);
		font-weight: 500;
	}
	.sheetrows {
		overflow-y: auto;
		border: none;
		padding: 0;
	}

	.parent-crumb {
		display: flex;
		align-items: center;
		gap: 7px;
		background: none;
		border: none;
		color: var(--ink-2);
		font-size: var(--t-sm);
		padding: 0;
		text-align: left;
		width: fit-content;
	}
	.parent-crumb:hover {
		color: var(--ink);
	}
	.pc-ic {
		color: var(--accent);
	}
	.pc-key {
		font-family: var(--mono);
		color: var(--ink-3);
	}
	.title-input {
		font-family: var(--serif);
		font-weight: 500;
		font-size: var(--t-xl);
		line-height: 1.3;
		resize: none;
		letter-spacing: -0.005em;
		padding: 4px 8px;
		margin-left: -9px;
		width: calc(100% + 18px);
		field-sizing: content;
		/* always-on heading editor: reads as plain text at rest, full .textarea border + ring on hover/focus */
		background: transparent;
		border-color: transparent;
	}
	.title-input:hover {
		border-color: var(--line-strong);
	}
	.title-input:focus {
		border-color: var(--accent);
		background: var(--surface);
	}
	.bh {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.bh h2 {
		font-size: var(--t-lg);
		font-weight: 600;
		margin: 0;
		color: var(--ink);
	}
	.bh .sp {
		flex: 1;
	}
	.saved {
		font-size: var(--t-sm);
		color: var(--st-done);
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.failed {
		font-size: var(--t-sm);
		color: var(--danger);
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.retry-link {
		color: var(--accent);
		font-size: var(--t-sm);
		font-weight: 500;
		background: none;
		border: none;
	}
	/* README banner: icon, bold lead-in, body at --t-sm. Info is accent-soft
	   (.conflict-banner), warn is the soft amber of --st-progress, danger is
	   danger-soft. */
	.conflict-banner,
	.banner {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 12px 16px;
		background: var(--accent-soft);
		border: 1px solid color-mix(in oklch, var(--accent) 35%, var(--line));
		border-radius: var(--r);
		font-size: var(--t-sm);
	}
	.banner {
		margin: 12px 24px 0;
	}
	.banner.warn {
		background: color-mix(in oklch, var(--st-progress) 14%, var(--surface));
		border-color: color-mix(in oklch, var(--st-progress) 40%, var(--line));
	}
	.banner.warn .bic {
		color: var(--st-progress);
	}
	.banner.danger {
		background: var(--danger-soft);
		border-color: color-mix(in oklch, var(--danger) 35%, var(--line));
	}
	.banner.danger .bic {
		color: var(--danger);
	}
	.btext b {
		font-weight: 600;
	}
	.top-conflict {
		margin: 12px 24px;
	}
	.bic {
		color: var(--accent);
		font-size: var(--t-md);
		flex: none;
	}
	.btext {
		color: var(--ink);
	}
	.banner .sp,
	.conflict-banner .sp {
		flex: 1;
	}
	.desc {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.desc-area {
		min-height: 160px;
		padding: 11px 13px;
		font-family: var(--serif);
		line-height: 1.6;
	}
	.edithint {
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.edithint .kbd {
		font-family: var(--mono);
		font-size: var(--t-xs);
		padding: 1px 4px;
		border: 1px solid var(--line-strong);
		border-bottom-width: 2px;
		border-radius: var(--r-sm);
		margin: 0 2px;
	}
	.prose {
		font-size: var(--t-base);
		line-height: 1.65;
		color: var(--ink);
	}
	.block {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.bh .n {
		font-family: var(--mono);
		color: var(--ink-3);
		font-size: var(--t-xs);
	}
	.sub-bar {
		height: 4px;
		border-radius: var(--r-sm);
		background: var(--line);
		overflow: hidden;
	}
	.sub-bar span {
		display: block;
		height: 100%;
		background: var(--st-done);
		transition: width 0.3s ease;
	}
	.crit {
		display: flex;
		align-items: flex-start;
		gap: 9px;
		padding: 3px 0;
	}
	.crit-box {
		width: 18px;
		height: 18px;
		border-radius: var(--r-sm);
		border: 1.5px solid var(--line-strong);
		background: var(--paper);
		color: var(--accent-ink);
		display: grid;
		place-items: center;
		font-size: var(--t-xs);
		flex: none;
		margin-top: 1px;
	}
	.crit-box.on {
		background: var(--accent);
		border-color: var(--accent);
	}
	.crit-kind {
		flex: none;
		font-size: var(--t-xs);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		color: var(--ink-3);
		border: 1px solid var(--line);
		border-radius: 999px;
		padding: 1.5px 7px;
		font-family: var(--mono);
	}
	.crit-kind.adv {
		color: var(--accent);
		border-color: color-mix(in oklch, var(--accent) 35%, var(--line));
	}
	.crit-ev {
		flex: none;
		max-width: 160px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.crit-text {
		flex: 1;
		font-size: var(--t-base);
		line-height: 1.45;
	}
	.crit-text.done {
		color: var(--ink-3);
		text-decoration: line-through;
	}
	.crit-del {
		opacity: 0;
		background: none;
		border: none;
		color: var(--ink-3);
		font-size: var(--t-xs);
		flex: none;
	}
	.crit:hover .crit-del {
		opacity: 1;
	}
	.crit-del:hover {
		color: var(--danger);
	}
	.crit-add {
		margin-top: 8px;
	}
	.dev-row {
		display: flex;
		align-items: baseline;
		gap: 10px;
		font-size: var(--t-base);
		color: var(--ink-2);
		padding: 2px 0;
	}
	.dev-row.link:hover {
		color: var(--ink);
	}
	.dev-lbl {
		width: 44px;
		flex: none;
		font-family: var(--mono);
		font-size: var(--t-xs);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--ink-3);
	}
	.dev-row .mono {
		font-family: var(--mono);
	}
	.dev-row .sha {
		font-size: var(--t-sm);
		color: var(--ink-3);
	}
	.dev-row .pr {
		color: var(--accent);
	}
	.cmsg {
		color: var(--ink);
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ctime {
		color: var(--ink-3);
		font-size: var(--t-sm);
		flex: none;
	}
	.sub-link,
	.doc-link {
		display: flex;
		align-items: center;
		gap: 9px;
		color: var(--ink);
		font-size: var(--t-base);
		text-align: left;
	}
	.sub-link {
		background: none;
		border: none;
		padding: 2px 0;
	}
	.sub-link:hover .sub-title {
		color: var(--ink);
	}
	.doc-link {
		background: var(--sunken);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 9px 13px;
	}
	.doc-link:hover {
		border-color: var(--line-strong);
	}
	.sub-key {
		font-family: var(--mono);
		font-size: var(--t-sm);
		color: var(--ink-3);
		flex: none;
	}
	.sub-title,
	.dl-t {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sub-title {
		color: var(--ink-2);
	}
	.sub-title.done {
		color: var(--ink-3);
		text-decoration: line-through;
	}
	.dl-ic {
		color: var(--accent);
	}
	.dl-ai {
		font-size: var(--t-xs);
		color: var(--accent);
	}
	.btn.sm {
		padding: 5px 10px;
		font-size: var(--t-sm);
	}
	/* Local "secondary" button (surface + line-strong border) — the design's
	   "sd" variant. Scoped here rather than added to the shared app.css. */
	.btn.sd {
		background: var(--surface);
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.btn.sd:hover {
		background: var(--hover);
	}
	.faint {
		color: var(--ink-3);
	}
	/* Gate refusal: danger-soft card with the open items as empty checkboxes. */
	.gate-block {
		display: flex;
		flex-direction: column;
		gap: 9px;
		margin: 12px 24px 0;
		padding: 13px 15px;
		background: var(--danger-soft);
		border: 1px solid color-mix(in oklch, var(--danger) 35%, var(--line));
		border-radius: var(--r);
	}
	.gate-head {
		font-size: var(--t-base);
		font-weight: 600;
		color: var(--danger);
	}
	.gate-item {
		display: flex;
		align-items: flex-start;
		gap: 8px;
		font-size: var(--t-sm);
		color: var(--ink-2);
	}
	.gate-box {
		width: 15px;
		height: 15px;
		border-radius: var(--r-sm);
		border: 1.5px solid var(--line-strong);
		flex: none;
		margin-top: 1px;
		box-sizing: border-box;
	}
	.gate-block-actions {
		display: flex;
		gap: var(--s2);
		align-items: center;
	}
	.btn.gho {
		background: transparent;
		color: var(--ink-2);
		border-color: var(--line);
	}
	.btn.gho:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.panes {
		display: none;
	}
	/* Too narrow for both side by side: show one at a time, switched by tabs,
	   so the comments are never buried under the whole task. */
	@media (max-width: 900px) {
		.panes {
			display: flex;
			gap: 4px;
			padding: 8px 16px 0;
			border-bottom: 1px solid var(--line);
		}
		.panes button {
			background: none;
			border: none;
			border-bottom: 2px solid transparent;
			color: var(--ink-2);
			font-size: var(--t-sm);
			padding: 6px 10px;
		}
		.panes button.on {
			color: var(--ink);
			border-bottom-color: var(--accent);
		}
		.chat {
			flex: 1 1 auto;
			border-left: none;
		}
		.dbody[data-pane='task'] .chat,
		.dbody[data-pane='chat'] .dmain {
			display: none;
		}
	}

	/* Not found */
	.notfound-wrap {
		height: 100%;
		display: grid;
		place-items: center;
		padding: 24px;
	}
	.nf-card {
		max-width: 360px;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
	}
	.nf-code {
		font-family: var(--mono);
		font-size: var(--t-sm);
		color: var(--ink-3);
		letter-spacing: 0.05em;
	}
	.nf-title {
		font-family: var(--serif);
		font-size: var(--t-xl);
		color: var(--ink);
		margin: 2px 0 2px;
	}
	.nf-body {
		font-size: var(--t-base);
		color: var(--ink-2);
		margin-bottom: 10px;
	}

	/* Loading skeleton */
	.skel-main {
		flex: 1;
		padding: 28px clamp(20px, 3vw, 40px);
		max-width: 820px;
		margin: 0 auto;
		width: 100%;
		box-sizing: border-box;
	}
	.sk {
		display: block;
		border-radius: var(--r-sm);
		background: linear-gradient(90deg, var(--hover) 25%, var(--sunken) 50%, var(--hover) 75%);
		background-size: 200% 100%;
		animation: sk-sweep 1.6s ease-in-out infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.sk {
			animation: none;
			background: var(--hover);
		}
	}
	@keyframes sk-sweep {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}
</style>
