<script>
	import { ChevronDown } from '@lucide/svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import { PRIORITIES } from '$lib/store.js';

	let { value = 0, onchange } = $props();
	let open = $state(false);
	const cur = $derived(PRIORITIES.find((p) => p.value === value) || PRIORITIES[0]);
	function pick(v) {
		open = false;
		onchange?.(v);
	}
</script>

<div class="dd">
	<button class="dd-btn" onclick={() => (open = !open)}>
		<PriorityIcon priority={value} />
		<span class="dd-label">{cur.label}</span>
		<ChevronDown size={14} strokeWidth={2} class="dd-chev" />
	</button>
	{#if open}
		<div class="dd-bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="dd-menu">
			{#each PRIORITIES as p (p.value)}
				<button class="dd-item" class:on={p.value === value} onclick={() => pick(p.value)}>
					<PriorityIcon priority={p.value} />
					<span>{p.label}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
