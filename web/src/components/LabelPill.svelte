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
		padding: 3px 9px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
		font-weight: 500;
		line-height: 1.3;
		color: var(--ink);
		background: var(--surface);
		border: 1px solid var(--line);
		white-space: nowrap;
	}
	.tdot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--lc);
		flex-shrink: 0;
	}
</style>
