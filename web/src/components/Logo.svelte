<script>
	// The DoneWhen mark: a ring that completes with a check that breaks out of it.
	// Same geometry as static/icon.svg, coloured from the theme tokens.
	let { size = 24 } = $props();
</script>

<svg class="logo" width={size} height={size} viewBox="0 0 512 512" aria-hidden="true">
	<rect width="512" height="512" rx="112" fill="var(--accent)" />
	<g fill="none" stroke="var(--accent-ink)" stroke-width="40" stroke-linecap="round" stroke-linejoin="round">
		<path d="M379.6 232.9 A128 128 0 1 1 278.2 139.9" />
		<path d="M190 268 L244 322 L366 176" />
	</g>
</svg>

<style>
	.logo {
		flex: none;
		display: block;
	}
</style>
