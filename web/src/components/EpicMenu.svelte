<script>
	import { ChevronDown, Box } from '@lucide/svelte';

	// A single-choice menu over { id, name } options. Named for its first use;
	// icon and none let it pick other things too.
	let { value = '', options = [], onchange, icon = Box, none = '— None —' } = $props();
	const Icon = $derived(icon);
	let open = $state(false);
	const cur = $derived(options.find((o) => o.id === value));
	function pick(id) {
		open = false;
		onchange?.(id);
	}
</script>

<div class="dd">
	<button class="dd-btn" onclick={() => (open = !open)}>
		<Icon size={14} strokeWidth={2} class="dd-ic" />
		<span class="dd-label">{cur?.name ?? none}</span>
		<ChevronDown size={14} strokeWidth={2} class="dd-chev" />
	</button>
	{#if open}
		<div class="dd-bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="dd-menu">
			<button class="dd-item" class:on={!value} onclick={() => pick('')}>
				<span class="dd-none">{none}</span>
			</button>
			{#each options as o (o.id)}
				<button class="dd-item" class:on={o.id === value} onclick={() => pick(o.id)}>
					<Icon size={14} strokeWidth={2} />
					<span>{o.name}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	:global(.dd-ic) {
		color: var(--ink-3);
		flex: none;
	}
	.dd-none {
		margin-left: 23px;
	}
</style>
