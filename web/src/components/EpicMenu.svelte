<script>
	import { ChevronDown, Box } from '@lucide/svelte';

	let { value = '', options = [], onchange } = $props();
	let open = $state(false);
	const cur = $derived(options.find((o) => o.id === value));
	function pick(id) {
		open = false;
		onchange?.(id);
	}
</script>

<div class="dd">
	<button class="dd-btn" onclick={() => (open = !open)}>
		<Box size={14} strokeWidth={2} class="dd-ic" />
		<span class="dd-label">{cur?.name ?? '— None —'}</span>
		<ChevronDown size={14} strokeWidth={2} class="dd-chev" />
	</button>
	{#if open}
		<div class="dd-bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="dd-menu">
			<button class="dd-item" class:on={!value} onclick={() => pick('')}>
				<span class="dd-none">— None —</span>
			</button>
			{#each options as o (o.id)}
				<button class="dd-item" class:on={o.id === value} onclick={() => pick(o.id)}>
					<Box size={14} strokeWidth={2} />
					<span>{o.name}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	:global(.dd-ic) {
		color: var(--text-faint);
		flex: none;
	}
	.dd-none {
		margin-left: 23px;
	}
</style>
