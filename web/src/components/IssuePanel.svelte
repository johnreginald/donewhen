<script>
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { states, projects, labels as allLabels, PRIORITIES } from '$lib/store.js';
	import { panelIssueId, closeIssue, openIssue, showToast } from '$lib/ui.js';
	import Markdown from './Markdown.svelte';
	import LabelPill from './LabelPill.svelte';
	import StateIcon from './StateIcon.svelte';

	let issue = $state(null);
	let comments = $state([]);
	let docs = $state([]);
	let children = $state([]); // sub-issues (this issue is their parent/epic)
	let parent = $state(null); // the epic/parent this issue belongs to
	const stOf = (c) => $states.find((s) => s.id === c.stateId);
	const doneChildren = $derived(children.filter((c) => stOf(c)?.category === 'completed').length);
	const stateCat = $derived($states.find((s) => s.id === issue?.stateId)?.category);
	const DOC_ICON = { change: '⟳', feature: '◈', decision: '◆', overview: '◇', reference: '▤' };
	let editingDesc = $state(false);
	let descDraft = $state('');
	let titleDraft = $state('');
	let newComment = $state('');
	let loading = $state(false);

	$effect(() => {
		const id = $panelIssueId;
		if (!id) {
			issue = null;
			return;
		}
		load(id);
	});

	async function load(id) {
		loading = true;
		try {
			issue = await api.issue(id);
			titleDraft = issue.title;
			descDraft = issue.descriptionMd || '';
			comments = (await api.comments(issue.id)) || [];
			docs = (await api.documents({ issue: issue.id })) || [];
			children = issue.childCount > 0 ? (await api.issues({ parent: issue.key })) || [] : [];
			parent = issue.parentKey ? await api.issue(issue.parentKey).catch(() => null) : null;
		} catch (e) {
			showToast('Load failed: ' + e.message, 'error');
			closeIssue();
		} finally {
			loading = false;
		}
	}

	async function patch(body) {
		try {
			issue = await api.updateIssue(issue.id, body);
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
	async function setState(e) {
		await patch({ stateId: e.target.value });
	}
	async function setPriority(e) {
		await patch({ priority: Number(e.target.value) });
	}
	async function setProject(e) {
		await patch({ projectId: e.target.value });
	}
	function toggleLabel(labelId) {
		const has = issue.labels.some((l) => l.id === labelId);
		const ids = has
			? issue.labels.filter((l) => l.id !== labelId).map((l) => l.id)
			: [...issue.labels.map((l) => l.id), labelId];
		patch({ labelIds: ids });
	}
	async function addComment() {
		if (!newComment.trim()) return;
		try {
			const c = await api.addComment(issue.id, newComment.trim());
			comments = [...comments, c];
			newComment = '';
		} catch (e) {
			showToast('Comment failed: ' + e.message, 'error');
		}
	}
	async function del() {
		if (!confirm(`Delete ${issue.key}?`)) return;
		await api.deleteIssue(issue.id);
		showToast(`${issue.key} deleted`);
		closeIssue();
	}

	let labelPickerOpen = $state(false);
	function fmtDate(s) {
		return new Date(s).toLocaleString();
	}
</script>

{#if $panelIssueId}
	<div class="backdrop" role="presentation" onclick={closeIssue}></div>
	<aside class="drawer">
		{#if issue}
			<header>
				<span class="key">{issue.key}</span>
				<div class="spacer"></div>
				<button class="btn ghost" onclick={del} title="Delete">🗑</button>
				<button class="btn ghost" onclick={closeIssue} title="Close">✕</button>
			</header>

			{#if parent}
				<button class="parent-crumb" onclick={() => openIssue(parent.key)} title="Open parent issue">
					<span class="pc-ic">⤴</span>
					<span class="pc-key">{parent.key}</span>
					<span class="pc-title">{parent.title}</span>
				</button>
			{/if}

			<input
				class="title-input"
				bind:value={titleDraft}
				onblur={saveTitle}
				onkeydown={(e) => e.key === 'Enter' && e.target.blur()}
			/>

			<div class="props">
				<div class="prop">
					<label>Status</label>
					<select class="input" value={issue.stateId} onchange={setState}>
						{#each $states as s (s.id)}
							<option value={s.id}>{s.name}</option>
						{/each}
					</select>
				</div>
				<div class="prop">
					<label>Priority</label>
					<select class="input" value={issue.priority} onchange={setPriority}>
						{#each PRIORITIES as p}
							<option value={p.value}>{p.label}</option>
						{/each}
					</select>
				</div>
				<div class="prop">
					<label>Epic</label>
					<select class="input" value={issue.projectId || ''} onchange={setProject}>
						<option value="">— none —</option>
						{#each $projects as p (p.id)}
							<option value={p.id}>{p.name}</option>
						{/each}
					</select>
				</div>
			</div>

			<div class="prop">
				<label>Labels</label>
				<div class="labels-row">
					{#each issue.labels as l (l.id)}
						<button class="pill-btn" onclick={() => toggleLabel(l.id)}>
							<LabelPill label={l} /><span class="x">✕</span>
						</button>
					{/each}
					<button class="btn ghost sm" onclick={() => (labelPickerOpen = !labelPickerOpen)}
						>+ label</button
					>
				</div>
				{#if labelPickerOpen}
					<div class="label-picker">
						{#each $allLabels as l (l.id)}
							<button
								class="picker-item"
								class:on={issue.labels.some((x) => x.id === l.id)}
								onclick={() => toggleLabel(l.id)}
							>
								<span class="dot" style:background={l.color}></span>{l.name}
							</button>
						{/each}
					</div>
				{/if}
			</div>

			<div class="desc">
				<div class="desc-head">
					<label>Description</label>
					<button class="btn ghost sm" onclick={() => (editingDesc = !editingDesc)}>
						{editingDesc ? 'preview' : 'edit'}
					</button>
				</div>
				{#if editingDesc}
					<textarea class="input desc-area" bind:value={descDraft} onblur={saveDesc}></textarea>
				{:else}
					<div class="desc-view" role="button" tabindex="0" ondblclick={() => (editingDesc = true)}>
						{#if issue.descriptionMd}
							<Markdown source={issue.descriptionMd} />
						{:else}
							<span class="faint">No description. Double-click to add.</span>
						{/if}
					</div>
				{/if}
			</div>

			{#if children.length}
				<div class="subs-sec">
					<label>Sub-issues <span class="sub-prog">{doneChildren}/{children.length}</span></label>
					<div class="sub-bar"><span style="width:{(doneChildren / children.length) * 100}%"></span></div>
					{#each children as c (c.id)}
						<button class="sub-link" onclick={() => openIssue(c.key)}>
							<StateIcon category={stOf(c)?.category} color={stOf(c)?.color} />
							<span class="sub-key">{c.key}</span>
							<span class="sub-title">{c.title}</span>
						</button>
					{/each}
				</div>
			{/if}

			<div class="docs-sec">
				<label>Documents</label>
				{#each docs as d (d.id)}
					<button class="doc-link" onclick={() => goto(`/docs?doc=${d.id}`)}>
						<span class="dl-ic">{DOC_ICON[d.type] || '▤'}</span>
						<span class="dl-t">{d.title}</span>
						{#if d.author === 'ai'}<span class="dl-ai">✦ AI</span>{/if}
					</button>
				{:else}
					<div class="doc-empty faint">
						{stateCat === 'completed' || stateCat === 'started'
							? 'No implementation doc yet — Claude writes one via MCP when the work lands.'
							: 'No documents attached.'}
					</div>
				{/each}
			</div>

			<div class="comments">
				<label>Comments</label>
				{#each comments as c (c.id)}
					<div class="comment">
						<div class="comment-meta">
							<span class="actor" class:ai={c.actor === 'ai'}>{c.actor}</span>
							<span class="faint">{fmtDate(c.createdAt)}</span>
						</div>
						<Markdown source={c.bodyMd} />
					</div>
				{/each}
				<div class="add-comment">
					<textarea class="input" rows="2" placeholder="Add a comment…" bind:value={newComment}
					></textarea>
					<button class="btn primary" onclick={addComment}>Comment</button>
				</div>
			</div>
		{:else if loading}
			<div class="loading">Loading…</div>
		{/if}
	</aside>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		z-index: 40;
	}
	.drawer {
		position: fixed;
		top: 0;
		right: 0;
		height: 100%;
		width: min(560px, 100vw);
		background: var(--bg-elev);
		border-left: 1px solid var(--border);
		box-shadow: var(--shadow);
		z-index: 41;
		overflow-y: auto;
		padding: 16px 20px 60px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	header {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.key {
		font-family: var(--mono);
		color: var(--text-dim);
	}
	.spacer {
		flex: 1;
	}
	.title-input {
		background: transparent;
		border: none;
		font-size: 20px;
		font-weight: 600;
		outline: none;
		padding: 2px 0;
	}
	.props {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 10px;
	}
	.prop {
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	.prop label,
	.desc label,
	.comments > label {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-faint);
	}
	select.input {
		padding: 6px 8px;
	}
	.labels-row {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
	}
	.pill-btn {
		background: none;
		border: none;
		padding: 0;
		display: inline-flex;
		align-items: center;
	}
	.pill-btn .x {
		font-size: 9px;
		color: var(--text-faint);
		margin-left: 3px;
	}
	.btn.sm {
		padding: 3px 8px;
		font-size: 12px;
	}
	.label-picker {
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
		margin-top: 8px;
		padding: 8px;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.picker-item {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 20px;
		padding: 3px 9px;
		font-size: 12px;
		color: var(--text);
	}
	.picker-item.on {
		border-color: var(--accent);
	}
	.desc-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.desc-area {
		min-height: 160px;
		font-family: var(--mono);
		font-size: 13px;
		resize: vertical;
	}
	.desc-view {
		min-height: 40px;
	}
	.parent-crumb {
		display: flex;
		align-items: center;
		gap: 7px;
		background: none;
		border: none;
		color: var(--text-dim);
		font-size: 12px;
		padding: 0 0 2px;
		text-align: left;
		width: 100%;
	}
	.parent-crumb:hover {
		color: var(--text);
	}
	.pc-ic {
		color: var(--accent2);
		flex: none;
	}
	.pc-key {
		font-family: var(--mono);
		color: var(--text-faint);
		flex: none;
	}
	.pc-title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.subs-sec {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.sub-prog {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--text-faint);
		margin-left: 4px;
	}
	.sub-bar {
		height: 3px;
		border-radius: 3px;
		background: var(--border);
		overflow: hidden;
		margin: 1px 0 3px;
	}
	.sub-bar span {
		display: block;
		height: 100%;
		background: var(--accent);
		transition: width 0.3s ease;
	}
	.sub-link {
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 6px 10px;
		color: var(--text);
		font-size: 12.5px;
		text-align: left;
	}
	.sub-link:hover {
		border-color: var(--border-strong);
		background: var(--bg-elev2);
	}
	.sub-key {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--text-faint);
		flex: none;
	}
	.sub-title {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.docs-sec {
		display: flex;
		flex-direction: column;
		gap: 7px;
	}
	.doc-link {
		display: flex;
		align-items: center;
		gap: 9px;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 11px;
		color: var(--text);
		font-size: 13px;
		text-align: left;
	}
	.doc-link:hover {
		border-color: var(--border-strong);
		background: var(--bg-elev2);
	}
	.dl-ic {
		color: var(--accent);
		flex: none;
	}
	.dl-t {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.dl-ai {
		font-size: 10px;
		color: var(--accent2);
		flex: none;
	}
	.doc-empty {
		font-size: 12px;
		line-height: 1.5;
		padding: 2px 2px 4px;
	}
	.comments {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.comment {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 8px 12px;
	}
	.comment-meta {
		display: flex;
		gap: 8px;
		font-size: 11px;
		margin-bottom: 4px;
	}
	.actor {
		text-transform: uppercase;
		font-size: 10px;
		letter-spacing: 0.04em;
		color: var(--text-dim);
	}
	.actor.ai {
		color: var(--accent);
	}
	.add-comment {
		display: flex;
		flex-direction: column;
		gap: 6px;
		align-items: flex-end;
	}
	.add-comment textarea {
		width: 100%;
	}
	.loading {
		color: var(--text-dim);
		padding: 40px;
		text-align: center;
	}
</style>
