<script>
	let { label } = $props();
</script>

<span class="lpill" style:--lc={label.color}>
	<span class="ldot"></span>{label.name}
</span>

<style>
	.lpill {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 3px 9px;
		border-radius: 6px;
		font-size: 12.5px;
		font-weight: 500;
		line-height: 1.35;
		color: var(--text);
		background: color-mix(in srgb, var(--lc) 15%, var(--bg-elev));
		border: 1px solid color-mix(in srgb, var(--lc) 30%, transparent);
		white-space: nowrap;
	}
	.ldot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--lc);
		flex-shrink: 0;
		box-shadow: 0 0 5px color-mix(in srgb, var(--lc) 60%, transparent);
	}
</style>
