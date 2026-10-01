<script>
	import { renderMarkdown, renderMermaid } from '$lib/markdown.js';

	let { source = '' } = $props();
	let el = $state(null);

	$effect(() => {
		const src = source;
		if (!el) return;
		el.innerHTML = renderMarkdown(src);
		renderMermaid(el);
	});
</script>

<div class="markdown" bind:this={el}></div>

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
		border-radius: 4px;
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
	.markdown :global(.mermaid-toggle) {
		position: absolute;
		top: 8px;
		right: 8px;
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--ink-2);
		border-radius: 5px;
		font-size: 11px;
		padding: 2px 8px;
		z-index: 2;
	}
	.markdown :global(.mermaid-rendered svg) {
		max-width: 100%;
		height: auto;
	}
	.markdown :global(.mermaid-source) {
		white-space: pre-wrap;
		font-family: var(--mono);
		font-size: 12px;
	}
	.markdown :global(table) {
		font-family: var(--font);
		font-size: 13px;
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
		font-size: 12px;
	}
</style>
