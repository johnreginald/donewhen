<script>
	// 0 none, 1 urgent, 2 high, 3 medium, 4 low
	let { priority = 0 } = $props();
	const heights = {
		0: [4, 4, 4],
		1: [10, 10, 10],
		2: [5, 8, 11],
		3: [5, 8, 8],
		4: [5, 5, 5]
	};
	const colors = {
		0: 'var(--ink-2)',
		1: 'var(--danger)', // urgent — red
		2: 'var(--ink)', // high
		3: 'var(--ink-2)', // medium
		4: 'var(--ink-3)' // low
	};
	const bars = $derived(heights[priority] ?? heights[0]);
	const col = $derived(colors[priority] ?? colors[0]);
</script>

<span class="prio" title="priority {priority}" style:--pc={col}>
	{#each bars as h, i}
		<span
			class="bar"
			style:height="{h}px"
			style:opacity={priority === 0 ? 0.4 : priority !== 1 && i >= priority - 1 ? 0.35 : 1}
		></span>
	{/each}
</span>

<style>
	.prio {
		display: inline-flex;
		align-items: flex-end;
		gap: 2px;
		height: 12px;
	}
	.bar {
		width: 3px;
		background: var(--pc, var(--ink-2));
		border-radius: 1px;
	}
</style>
