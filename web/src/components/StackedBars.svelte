<script>
	// A small stacked bar chart over days, drawn to the dataviz mark specs:
	// thin bars anchored to the baseline, a 4px rounded top on the top segment
	// only, a 2px surface gap between stacked segments and between bars, a
	// recessive baseline, a legend whenever there is more than one series, a
	// hover tooltip on every column (hit target the full column, not the mark),
	// and a table for screen readers.
	//
	// series: [{ key, label, color }] — bottom to top, in a fixed order.
	// days:   [{ date: 'YYYY-MM-DD', [key]: number, ... }]
	// format: how a value reads in the tooltip.
	let { title, subtitle = 'Last 14 days', series, days, format = (v) => String(v), max = null } = $props();

	let width = $state(0);
	const H = 96; // plot height
	const GAP = 2; // surface gap between bars and between segments
	const R = 4; // rounded data end

	let hover = $state(-1);

	const top = $derived(
		max ?? Math.max(1, ...days.map((d) => series.reduce((sum, s) => sum + (d[s.key] || 0), 0)))
	);
	const barW = $derived(days.length ? Math.max(2, width / days.length - GAP) : 0);
	const empty = $derived(days.every((d) => series.every((s) => !d[s.key])));

	// Segments for one day, bottom-up; the last non-empty one carries the round top.
	function segments(d) {
		let y = H;
		const out = [];
		for (const s of series) {
			const v = d[s.key] || 0;
			if (!v) continue;
			const h = Math.max(1, (v / top) * H);
			out.push({ key: s.key, color: s.color, y: y - h, h });
			y -= h + GAP;
		}
		if (out.length) out[out.length - 1].top = true;
		return out;
	}

	// A rect with only its top corners rounded, so the bar stays flat on the baseline.
	function topRounded(x, y, w, h) {
		const r = Math.min(R, w / 2, h);
		return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`;
	}

	const short = (iso) => {
		const [, m, d] = iso.split('-').map(Number);
		return `${m}/${d}`;
	};
	const long = (iso) =>
		new Date(iso + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
</script>

<figure class="chart">
	<figcaption>
		<span class="t">{title}</span>
		<span class="s">{subtitle}</span>
	</figcaption>

	{#if series.length > 1}
		<ul class="legend">
			{#each series as s (s.key)}
				<li><span class="sw" style:background={s.color}></span>{s.label}</li>
			{/each}
		</ul>
	{/if}

	<div class="plot" bind:clientWidth={width}>
		{#if width}
			<svg {width} height={H + 1} role="img" aria-label="{title}, {subtitle}">
				{#each days as d, i (d.date)}
					{@const x = i * (barW + GAP)}
					{#each segments(d) as seg (seg.key)}
						{#if seg.top}
							<path d={topRounded(x, seg.y, barW, seg.h)} fill={seg.color} opacity={hover >= 0 && hover !== i ? 0.45 : 1} />
						{:else}
							<rect {x} y={seg.y} width={barW} height={seg.h} fill={seg.color} opacity={hover >= 0 && hover !== i ? 0.45 : 1} />
						{/if}
					{/each}
					<!-- hit target: the whole column -->
					<rect
						class="hit"
						{x}
						y="0"
						width={barW + GAP}
						height={H}
						role="presentation"
						onmouseenter={() => (hover = i)}
						onmouseleave={() => (hover = -1)}
					/>
				{/each}
				<line x1="0" x2={width} y1={H + 0.5} y2={H + 0.5} class="base" />
			</svg>
			{#if hover >= 0}
				{@const d = days[hover]}
				<div class="tip" style:left="{Math.min(Math.max(hover * (barW + GAP) + barW / 2, 70), width - 70)}px">
					<div class="tip-d">{long(d.date)}</div>
					{#each [...series].reverse() as s (s.key)}
						<div class="tip-r">
							<span class="sw" style:background={s.color}></span>
							<span class="tl">{s.label}</span>
							<span class="tv">{format(d[s.key] || 0, d)}</span>
						</div>
					{/each}
				</div>
			{/if}
			{#if empty}
				<div class="none">Nothing yet</div>
			{/if}
		{/if}
	</div>
	{#if days.length}
		<div class="axis">
			<span>{short(days[0].date)}</span>
			<span>{short(days[Math.floor(days.length / 2)].date)}</span>
			<span>{short(days[days.length - 1].date)}</span>
		</div>
	{/if}

	<table class="sr">
		<caption>{title}</caption>
		<thead><tr><th>Date</th>{#each series as s (s.key)}<th>{s.label}</th>{/each}</tr></thead>
		<tbody>
			{#each days as d (d.date)}
				<tr><td>{d.date}</td>{#each series as s (s.key)}<td>{format(d[s.key] || 0, d)}</td>{/each}</tr>
			{/each}
		</tbody>
	</table>
</figure>

<style>
	.chart {
		margin: 0;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 14px 16px 12px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
		position: relative;
	}
	figcaption {
		display: flex;
		flex-direction: column;
		line-height: 1.3;
	}
	.t {
		font-size: 13px;
		color: var(--text);
	}
	.s {
		font-size: 11.5px;
		color: var(--text-faint);
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 12px;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: 11.5px;
		color: var(--text-dim);
	}
	.legend li {
		display: inline-flex;
		align-items: center;
		gap: 5px;
	}
	.sw {
		width: 8px;
		height: 8px;
		border-radius: 2px;
		flex-shrink: 0;
	}
	.plot {
		position: relative;
		height: 97px;
	}
	svg {
		display: block;
		overflow: visible;
	}
	.hit {
		fill: transparent;
		cursor: default;
	}
	.base {
		stroke: var(--border-strong);
		stroke-width: 1;
	}
	.none {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		color: var(--text-faint);
		font-size: 12px;
		pointer-events: none;
	}
	.tip {
		position: absolute;
		bottom: calc(100% + 6px);
		transform: translateX(-50%);
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 8px;
		box-shadow: var(--shadow);
		padding: 7px 9px;
		font-size: 12px;
		min-width: 140px;
		pointer-events: none;
		z-index: 5;
	}
	.tip-d {
		color: var(--text-dim);
		margin-bottom: 4px;
	}
	.tip-r {
		display: flex;
		align-items: center;
		gap: 6px;
		color: var(--text-dim);
	}
	.tl {
		flex: 1;
	}
	.tv {
		color: var(--text);
		font-variant-numeric: tabular-nums;
	}
	.axis {
		display: flex;
		justify-content: space-between;
		font-size: 11px;
		color: var(--text-faint);
		font-variant-numeric: tabular-nums;
	}
	.sr {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
