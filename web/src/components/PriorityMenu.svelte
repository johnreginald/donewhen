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

<div class="pm">
	<button class="pm-btn" onclick={() => (open = !open)}>
		<PriorityIcon priority={value} />
		<span class="pm-label">{cur.label}</span>
		<ChevronDown size={14} strokeWidth={2} class="pm-chev" />
	</button>
	{#if open}
		<div class="pm-bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="pm-menu">
			{#each PRIORITIES as p (p.value)}
				<button class="pm-item" class:on={p.value === value} onclick={() => pick(p.value)}>
					<PriorityIcon priority={p.value} />
					<span>{p.label}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.pm {
		position: relative;
	}
	.pm-btn {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		color: var(--text);
		padding: 7px 9px;
		font-size: 13.5px;
		text-align: left;
	}
	.pm-btn:hover {
		border-color: var(--border-strong);
	}
	.pm-label {
		flex: 1;
	}
	:global(.pm-chev) {
		color: var(--text-faint);
		flex: none;
	}
	.pm-bd {
		position: fixed;
		inset: 0;
		z-index: 30;
	}
	.pm-menu {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		z-index: 31;
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 8px;
		box-shadow: var(--shadow);
		padding: 4px;
		display: flex;
		flex-direction: column;
	}
	.pm-item {
		display: flex;
		align-items: center;
		gap: 9px;
		background: none;
		border: none;
		color: var(--text-dim);
		text-align: left;
		padding: 7px 9px;
		border-radius: 6px;
		font-size: 13.5px;
	}
	.pm-item:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.pm-item.on {
		color: var(--text);
	}
</style>
