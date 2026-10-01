<script>
	// Workflow-state glyph, colored by the state. The backend only models 6
	// categories (triage/backlog/unstarted/started/completed/canceled), but
	// the design has 9 distinct marks — Aligning/Ready share "unstarted" and
	// render identically (a plain ring, per the design system), while
	// In Progress/Blocked/In Review share "started" and are told apart by
	// name. `name` is optional — callers that only pass category/color (every
	// other screen) still get a sensible default for "started".
	// category: triage | backlog | unstarted | started | completed | canceled
	let { category = 'backlog', color = 'var(--st-backlog)', name = '', size = 14 } = $props();
	const c = 8; // center in a 16 viewBox
	const r = 6; // ring radius

	function slug(s) {
		return (s || '').toLowerCase().trim();
	}

	// started-category sub-variant: in progress (half fill) | blocked (bar) | in review (three-quarter fill)
	const startedKind = $derived.by(() => {
		const n = slug(name);
		if (n === 'blocked') return 'blocked';
		if (n === 'in review' || n === 'review') return 'review';
		return 'progress';
	});

	// A pie-wedge path for a fraction of the circle, starting at 12 o'clock,
	// sweeping clockwise — used for the half/three-quarter "fill" glyphs.
	function pieFill(fraction, radius) {
		const theta = fraction * 2 * Math.PI;
		const sx = c;
		const sy = c - radius;
		const ex = c + radius * Math.sin(theta);
		const ey = c - radius * Math.cos(theta);
		const large = fraction > 0.5 ? 1 : 0;
		return `M${c},${c} L${sx},${sy} A${radius},${radius} 0 ${large} 1 ${ex},${ey} Z`;
	}
</script>

<svg width={size} height={size} viewBox="0 0 16 16" fill="none" class="sicon">
	{#if category === 'completed'}
		<circle cx={c} cy={c} r={r} fill={color} />
		<path d="M5 8.2l2 2 4-4.4" stroke="var(--surface)" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" fill="none" />
	{:else if category === 'canceled'}
		<circle cx={c} cy={c} r={r} fill={color} />
		<path d="M5.7 5.7l4.6 4.6M10.3 5.7l-4.6 4.6" stroke="var(--surface)" stroke-width="1.5" stroke-linecap="round" />
	{:else if category === 'started'}
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" />
		{#if startedKind === 'blocked'}
			<line x1={c - 3} y1={c} x2={c + 3} y2={c} stroke={color} stroke-width="1.8" stroke-linecap="round" />
		{:else if startedKind === 'review'}
			<path d={pieFill(0.75, 4.1)} fill={color} />
		{:else}
			<path d={pieFill(0.5, 4.1)} fill={color} />
		{/if}
	{:else if category === 'triage'}
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" />
		<circle cx={c} cy={c} r="1.6" fill={color} />
	{:else if category === 'backlog'}
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" stroke-dasharray="2.4 2.1" />
	{:else}
		<!-- unstarted: hollow ring — Aligning and Ready both render this -->
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" />
	{/if}
</svg>

<style>
	.sicon {
		flex: none;
		display: block;
	}
</style>
