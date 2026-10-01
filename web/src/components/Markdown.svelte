<script>
	import { renderMarkdown, renderMermaid } from '$lib/markdown.js';
	import DiagramViewer from './DiagramViewer.svelte';

	let { source = '' } = $props();
	let el = $state(null);
	let viewer = $state(null); // { svg, source } while the overlay is open

	// One delegated handler serves every diagram in this Markdown: the Expand
	// button and a click on the diagram itself both open the viewer.
	function onclick(e) {
		const t = e.target;
		if (!(t instanceof Element)) return;
		const box = t.closest('.mermaid-container');
		if (!box || t.closest('.mermaid-toggle')) return;
		if (!t.closest('.mermaid-expand') && !t.closest('.mermaid-rendered')) return;
		const svg = box.querySelector('.mermaid-rendered svg');
		if (!svg) return;
		viewer = { svg: svg.outerHTML, source: box.dataset.source || '' };
	}

	$effect(() => {
		const src = source;
		if (!el) return;
		el.innerHTML = renderMarkdown(src);
		renderMermaid(el);
	});
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="markdown" bind:this={el} {onclick}></div>

{#if viewer}
	<DiagramViewer svg={viewer.svg} source={viewer.source} onclose={() => (viewer = null)} />
{/if}

<style>
	/* The record reads as prose: issue descriptions (and comments, via the same
	   component) are serif body text. Headings and tabular data stay sans —
	   they read as structure, not prose. */
	.markdown {
		font-family: var(--serif);
		font-size: var(--t-md);
		line-height: 1.68;
		color: var(--ink);
	}
	.markdown :global(h1),
	.markdown :global(h2),
	.markdown :global(h3) {
		font-family: var(--font);
		font-weight: 600;
		margin: 1.1em 0 0.5em;
		line-height: 1.3;
		letter-spacing: 0.01em;
	}
	.markdown :global(h1) {
		font-size: 1.3em;
	}
	.markdown :global(h2) {
		font-size: 1.15em;
	}
	.markdown :global(h3) {
		font-size: 1.05em;
	}
	.markdown :global(p) {
		margin: 0 0 0.85em;
	}
	.markdown :global(ul),
	.markdown :global(ol) {
		padding-left: 1.4em;
		line-height: 1.6;
		margin: 0 0 0.85em;
	}
	.markdown :global(code) {
		background: var(--sunken);
		padding: 1.5px 5px;
		border-radius: var(--r-sm);
		font-family: var(--mono);
		font-size: 0.86em;
		color: var(--ink);
	}
	.markdown :global(pre) {
		background: var(--sunken);
		border: 1px solid var(--line);
		padding: 12px 14px;
		border-radius: var(--r);
		overflow-x: auto;
		margin: 0.9em 0;
	}
	.markdown :global(pre code) {
		background: none;
		padding: 0;
		font-size: 0.82em;
		line-height: 1.6;
	}
	.markdown :global(a) {
		color: var(--accent);
	}
	.markdown :global(blockquote) {
		border-left: 3px solid var(--line-strong);
		margin: 0.5em 0;
		padding-left: 1em;
		color: var(--ink-2);
	}
	.markdown :global(.mermaid-container) {
		position: relative;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 12px;
		margin: 0.8em 0;
	}
	.markdown :global(.mermaid-actions) {
		position: absolute;
		top: 8px;
		right: 8px;
		display: flex;
		gap: 6px;
		z-index: 2;
	}
	.markdown :global(.mermaid-toggle),
	.markdown :global(.mermaid-expand) {
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--ink-2);
		border-radius: var(--r-sm);
		font-size: var(--t-xs);
		padding: 2px 8px;
		font-family: var(--font);
		cursor: pointer;
	}
	.markdown :global(.mermaid-toggle:hover),
	.markdown :global(.mermaid-expand:hover) {
		background: var(--hover);
		color: var(--ink);
	}
	.markdown :global(.mermaid-error) {
		background: var(--danger-soft);
		border: 1px solid var(--danger);
		border-radius: var(--r);
		padding: 10px 12px;
		margin: 0.8em 0;
	}
	.markdown :global(.mermaid-error-msg) {
		font-family: var(--font);
		font-size: var(--t-sm);
		font-weight: 600;
		color: var(--danger);
	}
	.markdown :global(.mermaid-error-details) {
		margin-top: 6px;
		font-family: var(--font);
		font-size: var(--t-sm);
		color: var(--ink-2);
	}
	.markdown :global(.mermaid-error-details summary) {
		cursor: pointer;
	}
	.markdown :global(.mermaid-error-details pre) {
		margin: 6px 0 0;
		white-space: pre-wrap;
		font-family: var(--mono);
	}
	.markdown :global(.mermaid-error .mermaid-source) {
		margin: 8px 0 0;
	}
	.markdown :global(.mermaid-rendered) {
		cursor: zoom-in;
	}
	.markdown :global(.mermaid-rendered svg) {
		max-width: 100%;
		height: auto;
	}
	.markdown :global(.mermaid-source) {
		white-space: pre-wrap;
		font-family: var(--mono);
		font-size: var(--t-sm);
	}
	.markdown :global(table) {
		font-family: var(--font);
		font-size: var(--t-sm);
		border-collapse: collapse;
		margin: 0.6em 0 1em;
	}
	.markdown :global(th),
	.markdown :global(td) {
		border: 1px solid var(--line);
		padding: 6px 11px;
		text-align: left;
	}
	.markdown :global(th) {
		background: var(--sunken);
		font-weight: 600;
		color: var(--ink-2);
	}
	.markdown :global(td) {
		color: var(--ink-2);
	}
	.markdown :global(td code),
	.markdown :global(th code) {
		font-size: var(--t-sm);
	}
</style>
