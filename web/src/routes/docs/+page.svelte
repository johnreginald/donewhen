<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { projects } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';

	let docs = $state([]);
	let sel = $state(null);
	let editing = $state(false);
	let titleDraft = $state('');
	let bodyDraft = $state('');

	onMount(load);
	async function load() {
		docs = (await api.documents()) || [];
	}
	async function open(d) {
		sel = await api.document(d.id);
		titleDraft = sel.title;
		bodyDraft = sel.bodyMd || '';
		editing = false;
	}
	async function create() {
		sel = await api.saveDocument({
			title: 'Untitled document',
			bodyMd: '# Untitled\n\n```mermaid\nflowchart LR\n  A --> B\n```\n'
		});
		titleDraft = sel.title;
		bodyDraft = sel.bodyMd;
		editing = true;
		await load();
	}
	async function save() {
		sel = await api.saveDocument({ id: sel.id, title: titleDraft, bodyMd: bodyDraft });
		editing = false;
		await load();
		showToast('Saved');
	}
	async function del() {
		if (!sel || !confirm('Delete document?')) return;
		await api.deleteDocument(sel.id);
		sel = null;
		await load();
	}
</script>

<div class="docs">
	<aside class="doc-list">
		<button class="btn primary full" onclick={create}>+ New document</button>
		{#each docs as d (d.id)}
			<button class="doc-item" class:active={sel && sel.id === d.id} onclick={() => open(d)}>
				{d.title}
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
					<button class="btn primary" onclick={save}>Save</button>
				{:else}
					<button class="btn" onclick={() => (editing = true)}>Edit</button>
				{/if}
				<button class="btn ghost" onclick={del}>Delete</button>
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
		width: 240px;
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
		padding: 8px 10px;
		border-radius: 6px;
		font-size: 13.5px;
	}
	.doc-item:hover,
	.doc-item.active {
		background: var(--bg-hover);
		color: var(--text);
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
	}
	.spacer {
		flex: 1;
	}
	.editor {
		width: 100%;
		min-height: 60vh;
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
			width: 150px;
		}
	}
</style>
