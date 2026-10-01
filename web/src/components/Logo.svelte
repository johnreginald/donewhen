<script module>
	// Mask ids are document-global, so each instance needs its own.
	let seq = 0;
</script>

<script>
	// The DoneWhen mark: a solid D with a check cut out of it. The check keeps going
	// past the edge of the D, and the colours swap as it crosses. Same geometry as
	// static/icon.svg, coloured from the theme tokens.
	let { size = 24 } = $props();
	const id = `dw${seq++}`;
	const D = 'M120 112 H258 A144 144 0 0 1 258 400 H120 Z';
	const CHECK = 'M188 262 L236 312 L398 126';
</script>

<svg class="logo" width={size} height={size} viewBox="0 0 512 512" aria-hidden="true">
	<defs>
		<mask id="{id}-in">
			<rect width="512" height="512" fill="#fff" />
			<path d={CHECK} fill="none" stroke="#000" stroke-width="46" stroke-linecap="round" stroke-linejoin="round" />
		</mask>
		<mask id="{id}-out">
			<rect width="512" height="512" fill="#fff" />
			<path d={D} fill="#000" />
		</mask>
	</defs>
	<rect width="512" height="512" rx="112" fill="var(--accent)" />
	<g transform="translate(-5 0)">
		<path d={D} fill="var(--accent-ink)" mask="url(#{id}-in)" />
		<path
			d={CHECK}
			fill="none"
			stroke="var(--accent-ink)"
			stroke-width="46"
			stroke-linecap="round"
			stroke-linejoin="round"
			mask="url(#{id}-out)"
		/>
	</g>
</svg>

<style>
	.logo {
		flex: none;
		display: block;
	}
</style>
