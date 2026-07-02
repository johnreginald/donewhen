<script>
	import { api } from '$lib/api.js';
	import { states, projects, labels as allLabels, PRIORITIES } from '$lib/store.js';
	import { panelIssueId, closeIssue, showToast } from '$lib/ui.js';
	import Markdown from './Markdown.svelte';
	import LabelPill from './LabelPill.svelte';

	let issue = $state(null);
	let comments = $state([]);
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
					<label>Project</label>
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
