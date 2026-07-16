<script>
	import { ChevronDown } from '@lucide/svelte';
	import StateIcon from './StateIcon.svelte';
	import { states } from '$lib/store.js';

	let { value, onchange } = $props();
	let open = $state(false);
	const cur = $derived($states.find((s) => s.id === value));
	function pick(id) {
		open = false;
		onchange?.(id);
	}
</script>

<div class="dd">
	<button class="dd-btn" onclick={() => (open = !open)}>
		{#if cur}<StateIcon category={cur.category} color={cur.color} />{/if}
		<span class="dd-label">{cur?.name ?? '—'}</span>
		<ChevronDown size={14} strokeWidth={2} class="dd-chev" />
	</button>
	{#if open}
		<div class="dd-bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="dd-menu">
			{#each $states as s (s.id)}
				<button class="dd-item" class:on={s.id === value} onclick={() => pick(s.id)}>
					<StateIcon category={s.category} color={s.color} />
					<span>{s.name}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
