<script>
	// The done-when tick, drawn with two borders: no "✓" character (IBM Plex has no glyph
	// for it, so a fallback font draws it slanted) and no SVG (some browsers drop an SVG
	// inside a button). It takes the colour of the text around it.
	let { size = 12 } = $props();
</script>

<span class="tick" style:--tick={size + 'px'} aria-hidden="true"></span>

<style>
	.tick {
		display: block;
		flex: none;
		box-sizing: border-box;
		width: calc(var(--tick) * 0.36);
		height: calc(var(--tick) * 0.68);
		margin-top: calc(var(--tick) * -0.1);
		border-right: 2px solid currentColor;
		border-bottom: 2px solid currentColor;
		transform: rotate(45deg);
	}
</style>
