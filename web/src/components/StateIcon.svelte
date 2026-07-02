<script>
	// Linear-style workflow-state glyph, colored by the state.
	// category: triage | backlog | unstarted | started | completed | canceled
	let { category = 'backlog', color = '#94a3b8', size = 14 } = $props();
	const r = 6;
	const c = 8; // center in a 16 viewBox
</script>

<svg width={size} height={size} viewBox="0 0 16 16" fill="none" class="sicon">
	{#if category === 'completed'}
		<circle cx={c} cy={c} r={r} fill={color} />
		<path d="M5 8.2l2 2 4-4.4" stroke="#0b0c10" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" fill="none" />
	{:else if category === 'canceled'}
		<circle cx={c} cy={c} r={r} fill={color} />
		<path d="M5.7 5.7l4.6 4.6M10.3 5.7l-4.6 4.6" stroke="#0b0c10" stroke-width="1.5" stroke-linecap="round" />
	{:else if category === 'started'}
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" />
		<circle cx={c} cy={c} r="3.2" fill={color} />
	{:else if category === 'triage'}
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" />
		<circle cx={c} cy={c} r="1.6" fill={color} />
	{:else if category === 'backlog'}
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" stroke-dasharray="2.4 2.1" />
	{:else}
		<!-- unstarted: hollow ring -->
		<circle cx={c} cy={c} r={r} stroke={color} stroke-width="1.6" />
	{/if}
</svg>

<style>
	.sicon {
		flex: none;
		display: block;
	}
</style>
