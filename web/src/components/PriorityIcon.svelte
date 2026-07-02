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
	const bars = $derived(heights[priority] ?? heights[0]);
</script>

<span class="prio" class:urgent={priority === 1} title="priority {priority}">
	{#each bars as h, i}
		<span
			class="bar"
			style:height="{h}px"
			style:opacity={priority === 0 ? 0.35 : priority !== 1 && i >= priority - 1 ? 0.3 : 1}
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
		background: var(--text-dim);
		border-radius: 1px;
	}
	.urgent .bar {
		background: #f87171;
	}
</style>
