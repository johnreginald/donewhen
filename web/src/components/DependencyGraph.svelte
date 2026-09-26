<script>
	// Which ticket waits on which, as a graph that reads left to right: a
	// ticket sits one column after the last of its blockers, so the order
	// the work can happen in is the order of the columns. Hovering a ticket
	// lights its whole chain — what it waits on and what waits on it.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { visibleIssues, states, blockLinks, activeJobs, agents, priorityAgents, taskAgentId } from '$lib/store.js';
	import { openIssue } from '$lib/ui.js';
	import StateIcon from './StateIcon.svelte';
	import { LoaderCircle, Lock, GitFork } from '@lucide/svelte';

	let allIssues = $state([]); // for blockers outside the current view
	let showUnlinked = $state(false);
	let hover = $state('');
	onMount(async () => (allIssues = (await api.issues().catch(() => [])) || []));

	const W = 232, H = 74, COLGAP = 84, ROWGAP = 14, PAD = 20;

	const graph = $derived.by(() => {
		const inView = new Map($visibleIssues.map((i) => [i.id, i]));
		const byId = new Map(allIssues.map((i) => [i.id, i]));
		for (const [id, i] of inView) byId.set(id, i);
		// Edges touching the view; a blocker outside it is drawn faded.
		const edges = $blockLinks.filter((l) => inView.has(l.issueId) || inView.has(l.blockerId));
		const ids = new Set();
		for (const e of edges) {
			ids.add(e.issueId);
			ids.add(e.blockerId);
		}
		if (showUnlinked) for (const id of inView.keys()) ids.add(id);
		const nodes = [...ids].map((id) => byId.get(id)).filter(Boolean);

		// Column: the longest chain of blockers before it.
		const preds = new Map(nodes.map((n) => [n.id, []]));
		const succs = new Map(nodes.map((n) => [n.id, []]));
		for (const e of edges) {
			if (!preds.has(e.issueId) || !preds.has(e.blockerId)) continue;
			preds.get(e.issueId).push(e.blockerId);
			succs.get(e.blockerId).push(e.issueId);
		}
		const level = new Map();
		const depth = (id, seen = new Set()) => {
			if (level.has(id)) return level.get(id);
			if (seen.has(id)) return 0; // a cycle cannot be saved, but never loop
			seen.add(id);
			const d = Math.max(-1, ...preds.get(id).map((p) => depth(p, seen))) + 1;
			level.set(id, d);
			return d;
		};
		nodes.forEach((n) => depth(n.id));
		const cols = [];
		for (const n of nodes) (cols[level.get(n.id)] ||= []).push(n);

		// Order each column by where its blockers sit, so edges cross less.
		const rowOf = new Map();
		cols.forEach((col, c) => {
			if (c === 0) col.sort((a, b) => a.number - b.number);
			else {
				const bary = (n) => {
					const ps = preds.get(n.id).filter((p) => rowOf.has(p));
					return ps.length ? ps.reduce((s, p) => s + rowOf.get(p), 0) / ps.length : 1e9;
				};
				col.sort((a, b) => bary(a) - bary(b) || a.number - b.number);
			}
			col.forEach((n, r) => rowOf.set(n.id, r));
		});

		const pos = new Map();
		cols.forEach((col, c) => col.forEach((n, r) => pos.set(n.id, { x: PAD + c * (W + COLGAP), y: PAD + r * (H + ROWGAP) })));
		const width = PAD * 2 + Math.max(1, cols.length) * (W + COLGAP) - COLGAP;
		const height = PAD * 2 + Math.max(1, ...cols.map((c) => c.length)) * (H + ROWGAP) - ROWGAP;
		return { nodes, edges: edges.filter((e) => pos.has(e.issueId) && pos.has(e.blockerId)), pos, preds, succs, inView, width, height, cols: cols.length };
	});

	// The chain through the hovered ticket, both ways.
	const lit = $derived.by(() => {
		if (!hover) return null;
		const s = new Set([hover]);
		const walk = (id, next) => {
			for (const n of next.get(id) || []) if (!s.has(n)) (s.add(n), walk(n, next));
		};
		walk(hover, graph.preds);
		walk(hover, graph.succs);
		return s;
	});

	const stOf = (n) => $states.find((s) => s.id === n.stateId);
	const jobOf = (n) => $activeJobs.find((j) => j.issueId === n.id);
	const agentOf = (n) => $agents.find((a) => a.id === (jobOf(n)?.agentId || taskAgentId(n, $priorityAgents)));
	const openCount = (n) => $blockLinks.filter((l) => l.issueId === n.id && !l.done).length;

	function path(e) {
		const a = graph.pos.get(e.blockerId), b = graph.pos.get(e.issueId);
		const x1 = a.x + W, y1 = a.y + H / 2, x2 = b.x, y2 = b.y + H / 2;
		const mx = (x1 + x2) / 2;
		return `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2 - 6},${y2}`;
	}
</script>

<div class="dg">
	<div class="bar">
		<span class="legend"><span class="ln open"></span>waits on (still open)</span>
		<span class="legend"><span class="ln ok"></span>cleared (In Review or Done)</span>
		<span class="spacer"></span>
		<label class="tog"><input type="checkbox" bind:checked={showUnlinked} /> Show tickets with no links</label>
	</div>

	{#if !graph.nodes.length}
		<div class="empty">
			<GitFork size={22} strokeWidth={1.6} />
			<p>No ticket in this view is linked to another. Add a blocker on a task's page, or pick an epic whose tickets depend on each other.</p>
		</div>
	{:else}
		<div class="scroll">
			<div class="canvas" style:width="{graph.width}px" style:height="{graph.height}px">
				<svg width={graph.width} height={graph.height} aria-hidden="true">
					<defs>
						<marker id="dg-arrow-open" viewBox="0 0 8 8" refX="6" refY="4" markerWidth="7" markerHeight="7" orient="auto">
							<path d="M0,0 L8,4 L0,8 z" fill="#fbbf24" />
						</marker>
						<marker id="dg-arrow-ok" viewBox="0 0 8 8" refX="6" refY="4" markerWidth="7" markerHeight="7" orient="auto">
							<path d="M0,0 L8,4 L0,8 z" fill="var(--text-faint)" />
						</marker>
					</defs>
					{#each graph.edges as e (e.blockerId + e.issueId)}
						<path
							d={path(e)}
							class="edge"
							class:open={!e.done}
							class:dim={lit && !(lit.has(e.issueId) && lit.has(e.blockerId))}
							marker-end="url(#dg-arrow-{e.done ? 'ok' : 'open'})"
						/>
					{/each}
				</svg>
				{#each graph.nodes as n (n.id)}
					{@const p = graph.pos.get(n.id)}
					{@const st = stOf(n)}
					{@const job = jobOf(n)}
					{@const a = agentOf(n)}
					{@const waits = openCount(n)}
					<button
						class="node"
						class:outside={!graph.inView.has(n.id)}
						class:running={job?.status === 'claimed'}
						class:dim={lit && !lit.has(n.id)}
						class:done={st?.category === 'completed' || st?.category === 'canceled'}
						style:left="{p.x}px"
						style:top="{p.y}px"
						style:width="{W}px"
						style:height="{H}px"
						onclick={() => openIssue(n.key)}
						onmouseenter={() => (hover = n.id)}
						onmouseleave={() => (hover = '')}
						onfocus={() => (hover = n.id)}
						onblur={() => (hover = '')}
						title="{n.key} · {n.title} — {st?.name || ''}"
					>
						<span class="top">
							<StateIcon category={st?.category} color={st?.color} />
							<span class="key">{n.key}</span>
							<span class="stn">{st?.name || ''}</span>
							{#if waits}<span class="lock"><Lock size={10} strokeWidth={2.4} />{waits}</span>{/if}
						</span>
						<span class="tt">{n.title}</span>
						<span class="ft">
							{#if job}
								<span class="doing" class:q={job.status === 'queued'}><LoaderCircle size={11} strokeWidth={2.4} />{job.status === 'queued' ? 'Queued' : 'Working'}</span>
							{/if}
							{#if a}<span class="ag">{a.name}</span>{/if}
						</span>
					</button>
				{/each}
			</div>
		</div>
	{/if}
</div>

<style>
	.dg {
		height: 100%;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 8px 20px;
		font-size: 12px;
		color: var(--text-faint);
		border-bottom: 1px solid var(--border);
	}
	.legend {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.ln {
		width: 18px;
		height: 0;
		border-top: 2px solid var(--text-faint);
	}
	.ln.open {
		border-top-color: #fbbf24;
	}
	.spacer {
		flex: 1;
	}
	.tog {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		cursor: pointer;
	}
	.scroll {
		flex: 1;
		overflow: auto;
		min-height: 0;
	}
	.canvas {
		position: relative;
	}
	svg {
		position: absolute;
		inset: 0;
		overflow: visible;
	}
	.edge {
		fill: none;
		stroke: var(--text-faint);
		stroke-width: 1.5;
		opacity: 0.55;
		transition: opacity 0.12s;
	}
	.edge.open {
		stroke: #fbbf24;
		opacity: 0.85;
	}
	.edge.dim {
		opacity: 0.08;
	}
	.node {
		position: absolute;
		display: flex;
		flex-direction: column;
		gap: 4px;
		text-align: left;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 9px;
		padding: 8px 10px;
		color: var(--text);
		transition: opacity 0.12s, border-color 0.12s;
		overflow: hidden;
	}
	.node:hover,
	.node:focus-visible {
		border-color: var(--border-strong);
		outline: none;
	}
	.node.running {
		border-color: color-mix(in srgb, var(--st-progress) 55%, var(--border));
	}
	.node.outside {
		border-style: dashed;
		opacity: 0.7;
	}
	.node.done .tt {
		color: var(--text-faint);
	}
	.node.dim {
		opacity: 0.2;
	}
	.top {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 11.5px;
		min-width: 0;
	}
	.key {
		font-family: var(--mono);
		color: var(--text-dim);
	}
	.stn {
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
	}
	.lock {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		color: #fbbf24;
		font-size: 11px;
	}
	.tt {
		font-size: 12.5px;
		line-height: 1.3;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ft {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 11px;
		color: var(--text-faint);
		min-width: 0;
	}
	.doing {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		color: var(--st-progress);
	}
	.doing :global(svg) {
		animation: dg-spin 1.2s linear infinite;
	}
	.doing.q {
		color: var(--text-faint);
	}
	.doing.q :global(svg) {
		animation: none;
	}
	@keyframes dg-spin {
		to {
			transform: rotate(360deg);
		}
	}
	.ag {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.empty {
		margin: 60px auto;
		max-width: 420px;
		text-align: center;
		color: var(--text-faint);
		font-size: 13px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
	}
</style>
