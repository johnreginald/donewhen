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
	.markdown :global(h1),
	.markdown :global(h2),
	.markdown :global(h3) {
		margin: 0.8em 0 0.4em;
		line-height: 1.3;
	}
	.markdown :global(h1) {
		font-size: 1.4em;
	}
	.markdown :global(h2) {
		font-size: 1.2em;
	}
	.markdown :global(p) {
		margin: 0.5em 0;
		line-height: 1.6;
	}
	.markdown :global(ul),
	.markdown :global(ol) {
		padding-left: 1.4em;
		line-height: 1.6;
	}
	.markdown :global(code) {
		background: var(--surface);
		padding: 1px 5px;
		border-radius: 4px;
		font-family: var(--mono);
		font-size: 0.9em;
	}
	.markdown :global(pre) {
		background: var(--surface);
		padding: 12px;
		border-radius: var(--r);
		overflow-x: auto;
	}
	.markdown :global(pre code) {
		background: none;
		padding: 0;
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
		border-collapse: collapse;
		margin: 0.6em 0;
	}
	.markdown :global(th),
	.markdown :global(td) {
		border: 1px solid var(--line);
		padding: 5px 10px;
	}
</style>
