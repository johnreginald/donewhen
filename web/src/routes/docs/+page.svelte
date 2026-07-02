<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { projects, initiatives, issues, labels as allLabels } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';
	import LabelPill from '$components/LabelPill.svelte';

	let docs = $state([]);
	let sel = $state(null);
	let editing = $state(false);
	let titleDraft = $state('');
	let bodyDraft = $state('');
	let attachType = $state(''); // '' | initiative | project | issue
	let attachId = $state('');
	let labelPickerOpen = $state(false);

	onMount(load);
	async function load() {
		docs = (await api.documents()) || [];
	}

	function attachOf(d) {
		if (d.initiativeId) return { type: 'initiative', id: d.initiativeId };
		if (d.projectId) return { type: 'project', id: d.projectId };
		if (d.issueId) return { type: 'issue', id: d.issueId };
		return { type: '', id: '' };
	}

	async function open(d) {
		sel = await api.document(d.id);
		titleDraft = sel.title;
		bodyDraft = sel.bodyMd || '';
		const a = attachOf(sel);
		attachType = a.type;
		attachId = a.id;
		editing = false;
		labelPickerOpen = false;
	}

	async function create() {
		sel = await api.saveDocument({
			title: 'Untitled document',
			bodyMd: '# Untitled\n\nWrite markdown here. ```mermaid diagrams render inline.\n'
		});
		titleDraft = sel.title;
		bodyDraft = sel.bodyMd;
		attachType = '';
		attachId = '';
		editing = true;
		await load();
	}

	// persist merges a partial patch onto the current doc and saves.
	async function persist(extra) {
		if (!sel) return;
		sel = await api.saveDocument({ id: sel.id, title: sel.title, bodyMd: sel.bodyMd, ...extra });
		await load();
	}

	async function saveBody() {
		editing = false;
		await persist({ title: titleDraft, bodyMd: bodyDraft });
		showToast('Saved');
	}

	// Attach: set exactly one target, clear the others.
	async function applyAttach() {
		await persist({
			projectId: attachType === 'project' ? attachId : '',
			initiativeId: attachType === 'initiative' ? attachId : '',
			issueId: attachType === 'issue' ? attachId : ''
		});
	}
	function onAttachType() {
		attachId = '';
		if (attachType === '') applyAttach();
	}

	function toggleLabel(id) {
		const has = sel.labels.some((l) => l.id === id);
		const ids = has
			? sel.labels.filter((l) => l.id !== id).map((l) => l.id)
			: [...sel.labels.map((l) => l.id), id];
		persist({ labelIds: ids });
	}

	async function del() {
		if (!sel || !confirm('Delete document?')) return;
		await api.deleteDocument(sel.id);
		sel = null;
		await load();
	}

	function optionsFor(type) {
		if (type === 'initiative') return $initiatives.map((i) => ({ id: i.id, label: i.name }));
		if (type === 'project') return $projects.map((p) => ({ id: p.id, label: p.name }));
		if (type === 'issue') return $issues.map((i) => ({ id: i.id, label: `${i.key}  ${i.title}` }));
		return [];
	}

	// attach context label for a doc in the list
	function attachLabel(d) {
		if (d.initiativeId)
			return '◈ ' + ($initiatives.find((i) => i.id === d.initiativeId)?.name ?? 'Initiative');
		if (d.projectId) return '▢ ' + ($projects.find((p) => p.id === d.projectId)?.name ?? 'Project');
		if (d.issueId) return $issues.find((i) => i.id === d.issueId)?.key ?? 'Issue';
		return '';
	}
</script>

<div class="docs">
	<aside class="doc-list">
		<button class="btn primary full" onclick={create}>+ New document</button>
		{#each docs as d (d.id)}
			<button class="doc-item" class:active={sel && sel.id === d.id} onclick={() => open(d)}>
				<div class="di-title">{d.title}</div>
				<div class="di-meta">
					{#if attachLabel(d)}<span class="di-attach">{attachLabel(d)}</span>{/if}
					{#each d.labels ?? [] as l (l.id)}
						<span class="di-dot" style:background={l.color}></span>
					{/each}
				</div>
			</button>
		{:else}
			<div class="faint empty">No documents.</div>
		{/each}
	</aside>

	<section class="doc-main">
		{#if sel}
			<div class="doc-head">
				{#if editing}
					<input class="input title" bind:value={titleDraft} />
				{:else}
					<h1>{sel.title}</h1>
				{/if}
				<div class="spacer"></div>
				{#if editing}
					<button class="btn primary" onclick={saveBody}>Save</button>
				{:else}
					<button class="btn" onclick={() => (editing = true)}>Edit</button>
				{/if}
				<button class="btn ghost" onclick={del}>Delete</button>
			</div>

			<div class="doc-props">
				<div class="prop">
					<label>Attach to</label>
					<div class="attach-row">
						<select class="input sel" bind:value={attachType} onchange={onAttachType}>
							<option value="">Nothing</option>
							<option value="initiative">Initiative</option>
							<option value="project">Project (epic)</option>
							<option value="issue">Issue (ticket)</option>
						</select>
						{#if attachType}
							<select class="input sel" bind:value={attachId} onchange={applyAttach}>
								<option value="" disabled>Select…</option>
								{#each optionsFor(attachType) as o (o.id)}
									<option value={o.id}>{o.label}</option>
								{/each}
							</select>
						{/if}
					</div>
				</div>
				<div class="prop">
					<label>Labels</label>
					<div class="labels-row">
						{#each sel.labels as l (l.id)}
							<button class="pill-btn" onclick={() => toggleLabel(l.id)}>
								<LabelPill label={l} /><span class="x">✕</span>
							</button>
						{/each}
						<button class="btn ghost sm" onclick={() => (labelPickerOpen = !labelPickerOpen)}>+ label</button>
					</div>
					{#if labelPickerOpen}
						<div class="label-picker">
							{#each $allLabels as l (l.id)}
								<button
									class="picker-item"
									class:on={sel.labels.some((x) => x.id === l.id)}
									onclick={() => toggleLabel(l.id)}
								>
									<span class="dot" style:background={l.color}></span>{l.name}
								</button>
							{/each}
						</div>
					{/if}
				</div>
			</div>

			{#if editing}
				<textarea class="input editor" bind:value={bodyDraft}></textarea>
			{:else}
				<div class="rendered"><Markdown source={sel.bodyMd} /></div>
			{/if}
		{:else}
			<div class="placeholder faint">Select or create a document. Markdown + ```mermaid supported.</div>
		{/if}
	</section>
</div>

<style>
	.docs {
		display: flex;
		height: 100%;
	}
	.doc-list {
		width: 260px;
		border-right: 1px solid var(--border);
		padding: 10px;
		display: flex;
		flex-direction: column;
		gap: 4px;
		overflow-y: auto;
	}
	.full {
		justify-content: center;
		margin-bottom: 8px;
	}
	.doc-item {
		text-align: left;
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 9px 10px;
		border-radius: 8px;
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	.doc-item:hover,
	.doc-item.active {
		background: var(--bg-hover);
		color: var(--text);
	}
	.di-title {
		font-size: 13.5px;
	}
	.di-meta {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.di-attach {
		font-size: 11px;
		color: var(--text-faint);
	}
	.di-dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
	}
	.doc-main {
		flex: 1;
		padding: 20px 28px;
		overflow-y: auto;
		min-width: 0;
	}
	.doc-head {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 14px;
	}
	.title {
		font-size: 20px;
		font-weight: 600;
		font-family: var(--disp);
	}
	h1 {
		font-family: var(--disp);
		font-size: 22px;
		margin: 0;
	}
	.spacer {
		flex: 1;
	}
	.doc-props {
		display: flex;
		gap: 22px;
		flex-wrap: wrap;
		padding: 12px 14px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		margin-bottom: 16px;
	}
	.prop {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 220px;
	}
	.prop label {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-faint);
	}
	.attach-row {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.sel {
		width: auto;
		padding: 6px 8px;
		font-size: 13px;
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
	.editor {
		width: 100%;
		min-height: 55vh;
		font-family: var(--mono);
		font-size: 13.5px;
		line-height: 1.6;
		resize: vertical;
	}
	.placeholder,
	.empty {
		padding: 40px 10px;
	}
	@media (max-width: 720px) {
		.doc-list {
			width: 160px;
		}
		.doc-main {
			padding: 16px;
		}
	}
</style>
