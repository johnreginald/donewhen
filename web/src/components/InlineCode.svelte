<script>
	// Renders `backticked` spans as code and **double-star** spans as bold. Plain text
	// otherwise; no HTML is parsed.
	import { parseInline } from '$lib/inline.js';
	let { text = '' } = $props();
	const parts = $derived(parseInline(text));
</script>

{#each parts as p, i (i)}{#if p.kind === 'code'}<code class="ic">{p.text}</code>{:else if p.kind === 'bold'}<strong class="ib">{p.text}</strong>{:else}{p.text}{/if}{/each}

<style>
	.ic {
		font-family: var(--mono);
		font-size: 0.88em;
		background: var(--sunken);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		padding: 0 4px;
		overflow-wrap: anywhere;
	}
	.ib {
		font-weight: 600;
	}
</style>
