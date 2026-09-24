<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { states, projects, labels as allLabels, PRIORITIES } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';
	import LabelPill from '$components/LabelPill.svelte';
	import StateIcon from '$components/StateIcon.svelte';
	import PriorityMenu from '$components/PriorityMenu.svelte';
	import StatusMenu from '$components/StatusMenu.svelte';
	import EpicMenu from '$components/EpicMenu.svelte';
	import LabelPicker from '$components/LabelPicker.svelte';
	import ActivityFeed from '$components/ActivityFeed.svelte';
	import RunTicket from '$components/RunTicket.svelte';
	import Conversation from '$components/Conversation.svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import { touchRecent } from '$lib/recent.js';
	import { activeWorkspace, agents } from '$lib/store.js';
	import { GitBranch, GitPullRequestArrow, GitCommitHorizontal, Bot } from '@lucide/svelte';

	let issue = $state(null);
	let docs = $state([]);
	let children = $state([]);
	let parent = $state(null);
	let activity = $state([]);
	let criteria = $state([]);
	let commits = $state([]);
	let newCrit = $state('');
	const doneCrit = $derived(criteria.filter((c) => c.done).length);
	let loading = $state(false);
	let editingDesc = $state(false);
	let descDraft = $state('');
	let titleDraft = $state('');
	let labelPickerOpen = $state(false);
	let confirmDel = $state(false);

	const stOf = (c) => $states.find((s) => s.id === c.stateId);
	const epic = $derived(issue ? $projects.find((p) => p.id === issue.projectId) : null);
	const doneChildren = $derived(children.filter((c) => stOf(c)?.category === 'completed').length);
	const DOC_ICON = { change: '⟳', feature: '◈', decision: '◆', overview: '◇', reference: '▤' };

	// Reload whenever the :key param changes (also handles navigating between a
	// parent and its sub-issues).
	$effect(() => {
		const key = $page.params.key;
		if (key) load(key);
	});

	async function load(key) {
		loading = true;
		confirmDel = false;
		try {
			issue = await api.issue(key);
			titleDraft = issue.title;
			touchRecent($activeWorkspace?.slug, issue);
			descDraft = issue.descriptionMd || '';
			docs = (await api.documents({ issue: issue.id })) || [];
			children = issue.childCount > 0 ? (await api.issues({ parent: issue.key })) || [] : [];
			parent = issue.parentKey ? await api.issue(issue.parentKey).catch(() => null) : null;
			activity = (await api.issueActivity(issue.id)) || [];
			criteria = (await api.criteria(issue.id)) || [];
			commits = (await api.commits(issue.id)) || [];
		} catch (e) {
			showToast('Load failed: ' + e.message, 'error');
			goto('/tasks');
		} finally {
			loading = false;
		}
	}

	async function patch(body) {
		try {
			issue = await api.updateIssue(issue.id, body);
			activity = (await api.issueActivity(issue.id)) || [];
		} catch (e) {
			showToast('Update failed: ' + e.message, 'error');
		}
	}
	async function saveTitle() {
		if (titleDraft.trim() && titleDraft !== issue.title) await patch({ title: titleDraft.trim() });
	}
	async function saveDesc() {
		editingDesc = false;
		if (descDraft !== issue.descriptionMd) await patch({ descriptionMd: descDraft });
	}
	const setState = (e) => patch({ stateId: e.target.value });
	const setPriority = (e) => patch({ priority: Number(e.target.value) });
	const setProject = (e) => patch({ projectId: e.target.value });
	function toggleLabel(id) {
		const has = issue.labels.some((l) => l.id === id);
		patch({
			labelIds: has
				? issue.labels.filter((l) => l.id !== id).map((l) => l.id)
				: [...issue.labels.map((l) => l.id), id]
		});
	}
	async function del() {
		if (!confirmDel) {
			confirmDel = true;
			return;
		}
		try {
			await api.deleteIssue(issue.id);
			showToast(`${issue.key} deleted`);
			goto('/tasks');
		} catch (e) {
			showToast('Delete failed: ' + e.message, 'error');
		}
	}
	const fmtDate = (s) => new Date(s).toLocaleString();
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

</script>

{#if issue}
	<div class="detail">
		<PageHeader
			crumbs={[
				{ label: 'Tasks', href: '/tasks' },
				...(epic ? [{ label: epic.name, upper: false }] : []),
				{ label: issue.key, upper: false }
			]}
		>
			{#key issue.id}<RunTicket {issue} />{/key}
			<button class="btn danger sm" onclick={del}>{confirmDel ? 'Confirm delete' : 'Delete'}</button>
		</PageHeader>

		<div class="dbody">
			<main class="dmain">
				<div class="dmain-inner">
				{#if parent}
					<button class="parent-crumb" onclick={() => goto('/issue/' + parent.key)}>
						<span class="pc-ic">⤴</span><span class="pc-key">{parent.key}</span>
						<span class="pc-title">{parent.title}</span>
					</button>
				{/if}

				<textarea
					class="title-input"
					bind:value={titleDraft}
					rows="1"
					onblur={saveTitle}
					onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), e.target.blur())}
				></textarea>

				<div class="desc">
					{#if editingDesc}
						<textarea class="desc-area" bind:value={descDraft} onblur={saveDesc} use:autofocus></textarea>
					{:else}
						<div
							class="desc-view"
							role="button"
							tabindex="0"
							onclick={() => (editingDesc = true)}
							onkeydown={(e) => e.key === 'Enter' && (editingDesc = true)}
						>
							{#if issue.descriptionMd}
								<Markdown source={issue.descriptionMd} />
							{:else}
								<span class="faint">Add description…</span>
							{/if}
						</div>
					{/if}
				</div>

				<section class="block">
					<div class="rh">
						Done-when{#if criteria.length}<span class="prog">{doneCrit}/{criteria.length}</span>{/if}
					</div>
					{#if criteria.length}
						<div class="sub-bar"><span style="width:{(doneCrit / criteria.length) * 100}%"></span></div>
					{/if}
					{#each criteria as c (c.id)}
						<div class="crit">
							<button class="crit-box" class:on={c.done} onclick={() => toggleCrit(c)} aria-label="toggle">
								{#if c.done}✓{/if}
							</button>
							<span class="crit-text" class:done={c.done}>{c.body}</span>
							<button class="crit-del" onclick={() => delCrit(c)} title="Remove">✕</button>
						</div>
					{/each}
					<input
						class="crit-add"
						bind:value={newCrit}
						placeholder="Add acceptance criterion…"
						onkeydown={(e) => e.key === 'Enter' && addCrit()}
					/>
				</section>

				{#if children.length}
					<section class="block">
						<div class="rh">Sub-issues <span class="prog">{doneChildren}/{children.length}</span></div>
						<div class="sub-bar"><span style="width:{(doneChildren / children.length) * 100}%"></span></div>
						{#each children as c (c.id)}
							<button class="sub-link" onclick={() => goto('/issue/' + c.key)}>
								<StateIcon category={stOf(c)?.category} color={stOf(c)?.color} />
								<span class="sub-key">{c.key}</span>
								<span class="sub-title">{c.title}</span>
							</button>
						{/each}
					</section>
				{/if}

				{#if issue.gitBranch || issue.prUrl || commits.length}
					<section class="block">
						<div class="rh">Development</div>
						{#if issue.gitBranch}
							<div class="dev-row"><GitBranch size={14} strokeWidth={2} /><span class="mono">{issue.gitBranch}</span></div>
						{/if}
						{#if issue.prUrl}
							<a class="dev-row link" href={issue.prUrl} target="_blank" rel="noreferrer">
								<GitPullRequestArrow size={14} strokeWidth={2} />Pull request
							</a>
						{/if}
						{#each commits as c (c.id)}
							{#if c.url}
								<a class="dev-row link" href={c.url} target="_blank" rel="noreferrer">
									<GitCommitHorizontal size={14} strokeWidth={2} /><span class="mono">{shortSha(c.sha)}</span><span class="cmsg">{c.message}</span>
								</a>
							{:else}
								<div class="dev-row">
									<GitCommitHorizontal size={14} strokeWidth={2} /><span class="mono">{shortSha(c.sha)}</span><span class="cmsg">{c.message}</span>
								</div>
							{/if}
						{/each}
					</section>
				{/if}

				{#if docs.length}
					<section class="block">
						<div class="rh">Artifacts</div>
						{#each docs as d (d.id)}
							<button class="doc-link" onclick={() => goto(`/artifacts?doc=${d.id}`)}>
								<span class="dl-ic">{DOC_ICON[d.type] || '▤'}</span>
								<span class="dl-t">{d.title}</span>
								{#if d.author === 'ai'}<span class="dl-ai">✦ Clanker</span>{/if}
							</button>
						{/each}
					</section>
				{/if}

				{#key issue.id}<Conversation {issue} />{/key}

				{#if activity.length}
					<section class="block">
						<div class="rh">Activity</div>
						<ActivityFeed items={activity} />
					</section>
				{/if}
				</div>
			</main>

			<aside class="drail">
				<div class="rail-prop">
					<span class="rl">Status</span>
					<StatusMenu value={issue.stateId} onchange={(v) => patch({ stateId: v })} />
				</div>
				<div class="rail-prop">
					<span class="rl">Priority</span>
					<PriorityMenu value={issue.priority} onchange={(v) => patch({ priority: v })} />
				</div>
				<div class="rail-prop">
					<span class="rl">Agent</span>
					<EpicMenu value={issue.agentId || ''} options={$agents} icon={Bot} none="Unassigned" onchange={(v) => patch({ agentId: v })} />
				</div>
				<div class="rail-prop">
					<span class="rl">Epic</span>
					<EpicMenu value={issue.projectId || ''} options={$projects} onchange={(v) => patch({ projectId: v })} />
				</div>
				<div class="rail-prop">
					<span class="rl">Labels</span>
					<LabelPicker
						selected={issue.labels.map((l) => l.id)}
						onchange={(ids) => patch({ labelIds: ids })}
					/>
				</div>
				<div class="rail-prop">
					<span class="rl">Created</span>
					<span class="rv faint">{fmtDate(issue.createdAt)}</span>
				</div>
			</aside>
		</div>
	</div>
{:else}
	<div class="loading">{loading ? 'Loading…' : ''}</div>
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
		justify-content: center;
		min-height: 0;
		overflow: hidden;
	}
	.dmain {
		flex: 0 1 760px;
		min-width: 0;
		overflow-y: auto;
	}
	.dmain-inner {
		padding: 28px clamp(20px, 4vw, 48px);
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.parent-crumb {
		display: flex;
		align-items: center;
		gap: 7px;
		background: none;
		border: none;
		color: var(--text-dim);
		font-size: 13px;
		padding: 0;
		text-align: left;
		width: fit-content;
	}
	.parent-crumb:hover {
		color: var(--text);
	}
	.pc-ic {
		color: var(--accent2);
	}
	.pc-key {
		font-family: var(--mono);
		color: var(--text-faint);
	}
	.title-input {
		width: 100%;
		background: transparent;
		border: none;
		outline: none;
		color: var(--text);
		font-family: var(--disp);
		font-size: 26px;
		font-weight: 600;
		line-height: 1.25;
		resize: none;
		letter-spacing: -0.01em;
		padding: 0;
		field-sizing: content;
	}
	.rh {
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-faint);
		font-weight: 500;
	}
	.prog {
		font-family: var(--mono);
		color: var(--text-faint);
		margin-left: 4px;
	}
	.desc {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.desc-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.desc-area {
		width: 100%;
		min-height: 120px;
		background: transparent;
		border: none;
		color: var(--text);
		padding: 6px 8px;
		margin: -6px -8px;
		font-size: 14.5px;
		font-family: inherit;
		line-height: 1.65;
		outline: none;
		resize: none;
		field-sizing: content;
	}
	.desc-view {
		font-size: 14.5px;
		line-height: 1.65;
		color: var(--text);
		min-height: 32px;
		cursor: text;
		padding: 6px 8px;
		margin: -6px -8px;
		border-radius: 6px;
	}
	.desc-view:hover {
		background: color-mix(in srgb, var(--bg-elev) 45%, transparent);
	}
	.block {
		display: flex;
		flex-direction: column;
		gap: 9px;
		border-top: 1px solid var(--border);
		padding-top: 18px;
	}
	.sub-bar {
		height: 4px;
		border-radius: 4px;
		background: var(--border);
		overflow: hidden;
	}
	.sub-bar span {
		display: block;
		height: 100%;
		background: var(--accent);
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
		border-radius: 5px;
		border: 1.5px solid var(--border-strong);
		background: var(--bg);
		color: #fff;
		display: grid;
		place-items: center;
		font-size: 11px;
		flex: none;
		margin-top: 1px;
	}
	.crit-box.on {
		background: var(--st-done);
		border-color: var(--st-done);
	}
	.crit-text {
		flex: 1;
		font-size: 14px;
		line-height: 1.45;
	}
	.crit-text.done {
		color: var(--text-faint);
		text-decoration: line-through;
	}
	.crit-del {
		opacity: 0;
		background: none;
		border: none;
		color: var(--text-faint);
		font-size: 11px;
		flex: none;
	}
	.crit:hover .crit-del {
		opacity: 1;
	}
	.crit-del:hover {
		color: #f87171;
	}
	.crit-add {
		width: 100%;
		background: transparent;
		border: none;
		border-top: 1px dashed var(--border);
		color: var(--text);
		padding: 9px 0 2px;
		font-size: 13.5px;
		outline: none;
		margin-top: 4px;
	}
	.crit-add::placeholder {
		color: var(--text-faint);
	}
	.dev-row {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13.5px;
		color: var(--text-dim);
		padding: 3px 0;
	}
	.dev-row.link:hover {
		color: var(--text);
	}
	.dev-row .mono {
		font-family: var(--mono);
		font-size: 12.5px;
		color: var(--text-faint);
	}
	.cmsg {
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sub-link,
	.doc-link {
		display: flex;
		align-items: center;
		gap: 9px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 9px 12px;
		color: var(--text);
		font-size: 13.5px;
		text-align: left;
	}
	.sub-link:hover,
	.doc-link:hover {
		border-color: var(--border-strong);
		background: var(--bg-elev2);
	}
	.sub-key {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--text-faint);
		flex: none;
	}
	.sub-title,
	.dl-t {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.dl-ic {
		color: var(--accent);
	}
	.dl-ai {
		font-size: 11px;
		color: var(--accent2);
	}
	.drail {
		flex: 0 0 300px;
		border-left: 1px solid var(--border);
		padding: 28px 22px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.rail-prop {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.rl {
		font-size: 11.5px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-faint);
	}
	.rail-prop select {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 7px;
		color: var(--text);
		padding: 8px 10px;
		font-size: 13.5px;
		outline: none;
	}
	.rail-prop select:hover {
		border-color: var(--border-strong);
	}
	.rv {
		font-size: 13px;
	}
	.rail-labels {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
	}
	.pill-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: none;
		border: none;
		padding: 0;
	}
	.pill-btn .x {
		font-size: 9px;
		color: var(--text-faint);
	}
	.pill-btn:hover .x {
		color: #f87171;
	}
	.label-picker {
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 8px;
		padding: 5px;
		max-height: 240px;
		overflow-y: auto;
	}
	.picker-item {
		display: flex;
		align-items: center;
		gap: 8px;
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 6px 8px;
		border-radius: 6px;
		font-size: 13px;
		text-align: left;
	}
	.picker-item:hover {
		background: var(--bg-hover);
	}
	.picker-item.on {
		color: var(--text);
	}
	.picker-item .dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
	}
	.btn.sm {
		padding: 5px 10px;
		font-size: 12.5px;
	}
	.faint {
		color: var(--text-faint);
	}
	.loading {
		display: grid;
		place-items: center;
		height: 100%;
		color: var(--text-dim);
	}
	@media (max-width: 800px) {
		.dbody {
			flex-direction: column;
			overflow-y: auto;
		}
		.drail {
			width: 100%;
			border-left: none;
			border-top: 1px solid var(--border);
			flex-direction: row;
			flex-wrap: wrap;
			gap: 14px;
		}
		.rail-prop {
			flex: 1;
			min-width: 130px;
		}
	}
</style>
