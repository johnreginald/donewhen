<script>
	// 0 none, 1 urgent, 2 high, 3 medium, 4 low.
	// Priority is neutral ink — the one exception is Urgent, which is the
	// only priority allowed to use --danger (decided product rule).
	let { priority = 0 } = $props();
	const HEIGHTS = [5, 8, 11];
	// how many of the 3 bars are "on" (filled) at each level
	const FILLED = { 0: 0, 2: 3, 3: 2, 4: 1 };
	const filled = $derived(FILLED[priority] ?? 0);
</script>

{#if priority === 1}
	<span class="urg" title="Urgent">!</span>
{:else}
	<span class="pri3" title={priority === 0 ? 'No priority' : ['', '', 'High', 'Medium', 'Low'][priority]}>
		{#each HEIGHTS as h, i}
			<i class:on={i < filled} style:height="{h}px"></i>
		{/each}
	</span>
{/if}

<style>
	.urg {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 14px;
		height: 14px;
		border-radius: 3px;
		background: var(--danger);
		color: var(--surface);
		font: 700 10px/14px var(--font);
		flex: none;
	}
	.pri3 {
		display: inline-flex;
		align-items: flex-end;
		gap: 1.5px;
		height: 11px;
	}
	.pri3 i {
		display: block;
		width: 3px;
		border-radius: 1px;
		background: var(--line-strong);
	}
	.pri3 i.on {
		background: var(--ink-2);
	}
</style>
