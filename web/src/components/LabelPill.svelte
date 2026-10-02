<script>
	let { label } = $props();
</script>

<span class="tag" style:--lc={label.color}>
	<span class="tdot"></span>{label.name}
</span>

<style>
	.tag {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 22px;
		padding: 0 8px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
		font-weight: 400;
		line-height: 1;
		color: var(--ink-2);
		background: var(--sunken);
		border: 1px solid var(--line);
		white-space: nowrap;
	}
	.tdot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--lc);
		flex-shrink: 0;
	}
</style>
