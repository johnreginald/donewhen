<script>
	// Renders `backticked` spans as code. Plain text otherwise; no HTML is parsed.
	let { text = '' } = $props();
	const parts = $derived(String(text ?? '').split(/(`[^`]+`)/g).filter(Boolean));
</script>

{#each parts as p, i (i)}{#if p.length > 2 && p.startsWith('`') && p.endsWith('`')}<code class="ic">{p.slice(1, -1)}</code>{:else}{p}{/if}{/each}

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
</style>
