<script>
	// The done-when tick: an upright SVG check, never the "✓" character. IBM Plex has
	// no glyph for it, so the browser falls back to a font that draws it slanted.
	let { size = 12 } = $props();
</script>

<svg class="tick" width={size} height={size} viewBox="0 0 12 12" aria-hidden="true">
	<path d="M2.5 6.4 L5 8.9 L9.6 3.4" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" />
</svg>

<style>
	.tick {
		display: block;
		flex: none;
	}
</style>
