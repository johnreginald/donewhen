<script>
	// Which ticket waits on which, as a map to move around: tickets read left
	// to right, each one column after the last of its blockers, so the columns
	// are the steps the work can happen in. Drag to move, scroll or the
	// buttons to zoom. Hover a ticket to see its chain — what it waits on in
	// amber, what it unblocks in blue — click to pin it and see the detail,
	// double-click to open it.
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api.js';
	import { visibleIssues, states, blockLinks, activeJobs, agents, projects, priorityAgents, taskAgentId } from '$lib/store.js';
	import { openIssue } from '$lib/ui.js';
	import StateIcon from './StateIcon.svelte';
	import { LoaderCircle, Lock, GitFork, Plus, Minus, Maximize, X, ArrowRight, Box } from '@lucide/svelte';

	let allIssues = $state([]); // for blockers outside the current view
	let showUnlinked = $state(false);
	let hover = $state('');
	let pinned = $state('');
	onMount(async () => (allIssues = (await api.issues().catch(() => [])) || []));

	const W = 240, H = 96, COLGAP = 96, ROWGAP = 16, PAD = 28, HEAD = 34;

	const graph = $derived.by(() => {
		const inView = new Map($visibleIssues.map((i) => [i.id, i]));
		const byId = new Map(allIssues.map((i) => [i.id, i]));
		for (const [id, i] of inView) byId.set(id, i);
		const edges = $blockLinks.filter((l) => inView.has(l.issueId) || inView.has(l.blockerId));
		const ids = new Set();
		for (const e of edges) (ids.add(e.issueId), ids.add(e.blockerId));
		if (showUnlinked) for (const id of inView.keys()) ids.add(id);
		const nodes = [...ids].map((id) => byId.get(id)).filter(Boolean);

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
			if (seen.has(id)) return 0;
			seen.add(id);
			const d = Math.max(-1, ...preds.get(id).map((p) => depth(p, seen))) + 1;
			level.set(id, d);
			return d;
		};
		nodes.forEach((n) => depth(n.id));
		const cols = [];
		for (const n of nodes) (cols[level.get(n.id)] ||= []).push(n);
		for (let i = 0; i < cols.length; i++) cols[i] ||= [];

		// Order each column by where its blockers sit, so edges cross less.
		const rowOf = new Map();
		cols.forEach((col, c) => {
			if (c === 0) col.sort((a, b) => (a.projectId || '').localeCompare(b.projectId || '') || a.number - b.number);
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
		cols.forEach((col, c) => col.forEach((n, r) => pos.set(n.id, { x: PAD + c * (W + COLGAP), y: PAD + HEAD + r * (H + ROWGAP) })));
		const width = PAD * 2 + Math.max(1, cols.length) * (W + COLGAP) - COLGAP;
		const height = PAD * 2 + HEAD + Math.max(1, ...cols.map((c) => c.length)) * (H + ROWGAP) - ROWGAP;
		const epics = new Set(nodes.map((n) => n.projectId).filter(Boolean));
		return {
			nodes, cols, pos, preds, succs, inView, width, height, manyEpics: epics.size > 1,
			edges: edges.filter((e) => pos.has(e.issueId) && pos.has(e.blockerId))
		};
	});

	// ── the chain through the focused ticket ────────────────────────────
	const focus = $derived(pinned || hover);
	const chain = $derived.by(() => {
		if (!focus || !graph.pos.has(focus)) return null;
		const up = new Set(), down = new Set();
		const walk = (id, next, into) => {
			for (const n of next.get(id) || []) if (!into.has(n)) (into.add(n), walk(n, next, into));
		};
		walk(focus, graph.preds, up);
		walk(focus, graph.succs, down);
		return { up, down };
	});
	const inChain = (id) => !chain || id === focus || chain.up.has(id) || chain.down.has(id);
	function edgeRole(e) {
		if (!chain) return '';
		if ((e.issueId === focus || chain.up.has(e.issueId)) && (chain.up.has(e.blockerId))) return 'up';
		if ((e.blockerId === focus || chain.down.has(e.blockerId)) && chain.down.has(e.issueId)) return 'down';
		return 'dim';
	}

	// ── pan and zoom ────────────────────────────────────────────────────
	let view = $state(null);
	let tx = $state(0), ty = $state(0), zoom = $state(1);
	let drag = null; // { x, y, tx, ty, moved }
	let dragging = $state(false);
	const clampZoom = (z) => Math.min(2, Math.max(0.25, z));

	function onDown(e) {
		if (e.button !== 0) return;
		drag = { x: e.clientX, y: e.clientY, tx, ty, moved: false };
	}
	function onMove(e) {
		if (!drag) return;
		const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
		if (!drag.moved && Math.hypot(dx, dy) < 4) return;
		drag.moved = true;
		dragging = true;
		tx = drag.tx + dx;
		ty = drag.ty + dy;
	}
	function onUp() {
		// A drag that moved is not a click on the ticket it started on.
		if (drag?.moved) setTimeout(() => (dragging = false), 0);
		else dragging = false;
		setTimeout(() => (drag = null), 0);
	}
	function zoomAt(z, cx, cy) {
		const nz = clampZoom(z);
		// Keep the point under the cursor where it is.
		tx = cx - ((cx - tx) * nz) / zoom;
		ty = cy - ((cy - ty) * nz) / zoom;
		zoom = nz;
	}
	function onWheel(e) {
		const r = view.getBoundingClientRect();
		if (e.ctrlKey || e.metaKey) {
			e.preventDefault();
			zoomAt(zoom * Math.exp(-e.deltaY * 0.01), e.clientX - r.left, e.clientY - r.top);
		} else {
			e.preventDefault();
			tx -= e.deltaX;
			ty -= e.deltaY;
		}
	}
	function step(f) {
		const r = view.getBoundingClientRect();
		zoomAt(zoom * f, r.width / 2, r.height / 2);
	}
	function fit() {
		if (!view) return;
		const r = view.getBoundingClientRect();
		const z = clampZoom(Math.min(1, (r.width - 24) / graph.width, (r.height - 24) / graph.height));
		zoom = z;
		tx = (r.width - graph.width * z) / 2;
		ty = Math.max(12, (r.height - graph.height * z) / 2);
	}
	// Scrolling pans and ⌘/ctrl-scroll zooms the map instead of the page:
	// the listener must be able to cancel, so it is not passive.
	$effect(() => {
		if (!view) return;
		view.addEventListener('wheel', onWheel, { passive: false });
		return () => view.removeEventListener('wheel', onWheel);
	});

	// Fit once when the graph first has something to show.
	let fitted = false;
	$effect(() => {
		if (!fitted && graph.nodes.length && view) {
			fitted = true;
			tick().then(fit);
		}
	});

	function onKey(e) {
		if (e.key === 'Escape') pinned = '';
		else if (e.key === '+' || e.key === '=') step(1.2);
		else if (e.key === '-') step(1 / 1.2);
		else if (e.key === '0') fit();
	}

	// ── what a ticket shows ─────────────────────────────────────────────
	const stOf = (n) => $states.find((s) => s.id === n.stateId);
	const jobOf = (n) => $activeJobs.find((j) => j.issueId === n.id);
	const agentOf = (n) => $agents.find((a) => a.id === (jobOf(n)?.agentId || taskAgentId(n, $priorityAgents)));
	const epicOf = (n) => $projects.find((p) => p.id === n.projectId);
	const openCount = (n) => $blockLinks.filter((l) => l.issueId === n.id && !l.done).length;
	const nodeById = (id) => graph.nodes.find((n) => n.id === id);
	const colTitle = (c) => (c === 0 ? 'Can start now' : `Step ${c + 1}`);

	function path(e) {
		const a = graph.pos.get(e.blockerId), b = graph.pos.get(e.issueId);
		const x1 = a.x + W, y1 = a.y + H / 2, x2 = b.x, y2 = b.y + H / 2;
		const mx = x1 + Math.max(40, (x2 - x1) / 2);
		return `M${x1},${y1} C${mx},${y1} ${x2 - Math.max(40, (x2 - x1) / 2)},${y2} ${x2 - 7},${y2}`;
	}
	function select(n) {
		if (dragging) return;
		pinned = pinned === n.id ? '' : n.id;
	}
	const pinnedNode = $derived(pinned ? nodeById(pinned) : null);
</script>

<svelte:window onmouseup={onUp} onmousemove={onMove} />

<div class="dg">
	<div class="bar">
		<span class="legend"><span class="ln open"></span>waits on (open)</span>
		<span class="legend"><span class="ln ok"></span>cleared (In Review / Done)</span>
		<span class="legend hint">Drag to move · ⌘ + scroll to zoom · click a ticket to follow its chain</span>
		<span class="spacer"></span>
		<label class="tog"><input type="checkbox" bind:checked={showUnlinked} /> Show unlinked</label>
	</div>

	{#if !graph.nodes.length}
		<div class="empty">
			<GitFork size={22} strokeWidth={1.6} />
			<p>No ticket in this view is linked to another. Add a blocker on a task's page, or pick an epic whose tickets depend on each other.</p>
		</div>
	{:else}
		<!-- The map is a pan/zoom surface: drag and wheel move it, the keys zoom. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
		<div
			class="view"
			class:grabbing={dragging}
			bind:this={view}
			onmousedown={onDown}
			onkeydown={onKey}
			tabindex="0"
			role="application"
			aria-label="Ticket dependency map"
		>
			<div class="canvas" style:width="{graph.width}px" style:height="{graph.height}px" style:transform="translate({tx}px, {ty}px) scale({zoom})">
				<!-- step headers -->
				{#each graph.cols as col, c (c)}
					<div class="colhead" style:left="{PAD + c * (W + COLGAP)}px" style:top="{PAD}px" style:width="{W}px">
						<span class="ct" class:now={c === 0}>{colTitle(c)}</span><span class="cn">{col.length}</span>
					</div>
				{/each}

				<svg width={graph.width} height={graph.height} aria-hidden="true">
					<defs>
						{#each [['open', '#fbbf24'], ['ok', '#6b7280'], ['up', '#fbbf24'], ['down', '#60a5fa']] as [k, c] (k)}
							<marker id="dg-{k}" viewBox="0 0 8 8" refX="6" refY="4" markerWidth="7" markerHeight="7" orient="auto">
								<path d="M0,0 L8,4 L0,8 z" fill={c} />
							</marker>
						{/each}
					</defs>
					{#each graph.edges as e (e.blockerId + e.issueId)}
						{@const role = edgeRole(e)}
						<path
							d={path(e)}
							class="edge {role}"
							class:open={!e.done}
							marker-end="url(#dg-{role === 'up' || role === 'down' ? role : e.done ? 'ok' : 'open'})"
						/>
					{/each}
				</svg>

				{#each graph.nodes as n (n.id)}
					{@const p = graph.pos.get(n.id)}
					{@const st = stOf(n)}
					{@const job = jobOf(n)}
					{@const a = agentOf(n)}
					{@const waits = openCount(n)}
					{@const ep = epicOf(n)}
					<button
						class="node"
						class:outside={!graph.inView.has(n.id)}
						class:running={job?.status === 'claimed'}
						class:dim={!inChain(n.id)}
						class:focus={n.id === focus}
						class:up={chain?.up.has(n.id)}
						class:down={chain?.down.has(n.id)}
						class:done={st?.category === 'completed' || st?.category === 'canceled'}
						style:left="{p.x}px"
						style:top="{p.y}px"
						style:width="{W}px"
						style:height="{H}px"
						style:--st={st?.color || 'var(--border-strong)'}
						onclick={() => select(n)}
						ondblclick={() => openIssue(n.key)}
						onmouseenter={() => (hover = n.id)}
						onmouseleave={() => (hover = '')}
						title="{n.key} · {n.title}"
					>
						<span class="top">
							<StateIcon category={st?.category} color={st?.color} />
							<span class="key">{n.key}</span>
							<span class="stn">{st?.name || ''}</span>
							{#if waits}<span class="lock" title="{waits} blocker{waits === 1 ? '' : 's'} still open"><Lock size={10} strokeWidth={2.4} />{waits}</span>{/if}
						</span>
						<span class="tt">{n.title}</span>
						<span class="ft">
							{#if job}
								<span class="doing" class:q={job.status === 'queued'}><LoaderCircle size={11} strokeWidth={2.4} />{job.status === 'queued' ? 'Queued' : 'Working'}</span>
							{/if}
							{#if a}<span class="ag">{a.name}</span>{/if}
							{#if graph.manyEpics && ep}<span class="ep"><Box size={10} strokeWidth={2.2} />{ep.name}</span>{/if}
						</span>
					</button>
				{/each}
			</div>

			<div class="zoom" role="group" aria-label="Zoom">
				<button onclick={() => step(1.2)} aria-label="Zoom in" title="Zoom in (+)"><Plus size={14} /></button>
				<span class="zv">{Math.round(zoom * 100)}%</span>
				<button onclick={() => step(1 / 1.2)} aria-label="Zoom out" title="Zoom out (−)"><Minus size={14} /></button>
				<button onclick={fit} aria-label="Fit to screen" title="Fit (0)"><Maximize size={13} /></button>
			</div>

			{#if pinnedNode}
				{@const n = pinnedNode}
				{@const st = stOf(n)}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div class="panel" onmousedown={(e) => e.stopPropagation()} role="dialog" tabindex="-1" aria-label="{n.key} detail">
					<div class="ph">
						<StateIcon category={st?.category} color={st?.color} />
						<span class="key">{n.key}</span>
						<span class="stn">{st?.name || ''}</span>
						<button class="x" onclick={() => (pinned = '')} aria-label="Close"><X size={14} /></button>
					</div>
					<div class="pt">{n.title}</div>
					<div class="sect">
						<div class="sh up">Waits on <span>{graph.preds.get(n.id).length}</span></div>
						{#each graph.preds.get(n.id) as id (id)}
							{@const b = nodeById(id)}
							{#if b}
								<button class="rel" onclick={() => (pinned = b.id)}>
									<StateIcon category={stOf(b)?.category} color={stOf(b)?.color} /><span class="key">{b.key}</span><span class="rt">{b.title}</span>
								</button>
							{/if}
						{:else}<div class="none">Nothing — it can start now.</div>{/each}
					</div>
					<div class="sect">
						<div class="sh down">Unblocks <span>{graph.succs.get(n.id).length}</span></div>
						{#each graph.succs.get(n.id) as id (id)}
							{@const d = nodeById(id)}
							{#if d}
								<button class="rel" onclick={() => (pinned = d.id)}>
									<StateIcon category={stOf(d)?.category} color={stOf(d)?.color} /><span class="key">{d.key}</span><span class="rt">{d.title}</span>
								</button>
							{/if}
						{:else}<div class="none">Nothing waits on it.</div>{/each}
					</div>
					{#if chain}
						<div class="chainsum">{chain.up.size} upstream · {chain.down.size} downstream in total</div>
					{/if}
					<button class="btn primary sm open" onclick={() => openIssue(n.key)}>Open task <ArrowRight size={13} /></button>
				</div>
			{/if}
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
		flex-wrap: wrap;
	}
	.legend {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.legend.hint {
		color: var(--text-faint);
		opacity: 0.8;
	}
	.ln {
		width: 18px;
		height: 0;
		border-top: 2px solid #6b7280;
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
	.view {
		position: relative;
		flex: 1;
		min-height: 0;
		overflow: hidden;
		cursor: grab;
		outline: none;
		background-image: radial-gradient(circle, color-mix(in srgb, var(--text-faint) 22%, transparent) 1px, transparent 1px);
		background-size: 22px 22px;
		user-select: none;
	}
	.view.grabbing {
		cursor: grabbing;
	}
	.canvas {
		position: absolute;
		left: 0;
		top: 0;
		transform-origin: 0 0;
	}
	svg {
		position: absolute;
		inset: 0;
		overflow: visible;
		pointer-events: none;
	}
	.colhead {
		position: absolute;
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 11.5px;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--text-faint);
		border-bottom: 1px dashed var(--border);
		padding-bottom: 6px;
	}
	.ct.now {
		color: #4ade80;
	}
	.cn {
		font-weight: 500;
		font-variant-numeric: tabular-nums;
	}
	.edge {
		fill: none;
		stroke: #6b7280;
		stroke-width: 1.6;
		opacity: 0.6;
		transition: opacity 0.12s, stroke 0.12s;
	}
	.edge.open {
		stroke: #fbbf24;
		opacity: 0.8;
	}
	.edge.up {
		stroke: #fbbf24;
		stroke-width: 2.4;
		opacity: 1;
	}
	.edge.down {
		stroke: #60a5fa;
		stroke-width: 2.4;
		opacity: 1;
	}
	.edge.dim {
		opacity: 0.07;
	}
	.node {
		position: absolute;
		display: flex;
		flex-direction: column;
		gap: 5px;
		text-align: left;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-left: 3px solid var(--st);
		border-radius: 9px;
		padding: 9px 11px;
		color: var(--text);
		transition: opacity 0.12s, border-color 0.12s, box-shadow 0.12s;
		overflow: hidden;
		cursor: pointer;
	}
	.view.grabbing .node {
		cursor: grabbing;
	}
	.node:hover {
		border-top-color: var(--border-strong);
		border-right-color: var(--border-strong);
		border-bottom-color: var(--border-strong);
	}
	.node.focus {
		box-shadow: 0 0 0 2px var(--accent2);
	}
	.node.up {
		box-shadow: 0 0 0 1px color-mix(in srgb, #fbbf24 60%, transparent);
	}
	.node.down {
		box-shadow: 0 0 0 1px color-mix(in srgb, #60a5fa 60%, transparent);
	}
	.node.running {
		background: color-mix(in srgb, var(--st-progress) 7%, var(--bg-elev));
	}
	.node.outside {
		border-style: dashed;
		border-left-style: solid;
		opacity: 0.75;
	}
	.node.done .tt {
		color: var(--text-faint);
	}
	.node.dim {
		opacity: 0.18;
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
		font-size: 11.5px;
		flex: none;
	}
	.stn {
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
		font-size: 11.5px;
	}
	.lock {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		color: #fbbf24;
		font-size: 11px;
	}
	.tt {
		font-size: 13px;
		line-height: 1.3;
		font-weight: 500;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.ft {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 11px;
		color: var(--text-faint);
		min-width: 0;
		margin-top: auto;
	}
	.doing {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		color: var(--st-progress);
		flex: none;
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
	.ag,
	.ep {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ep {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		margin-left: auto;
		color: var(--accent2);
	}
	.zoom {
		position: absolute;
		right: 14px;
		bottom: 14px;
		display: flex;
		align-items: center;
		gap: 2px;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 9px;
		padding: 3px;
		box-shadow: var(--shadow);
		cursor: default;
	}
	.zoom button {
		display: grid;
		place-items: center;
		width: 28px;
		height: 26px;
		background: none;
		border: none;
		border-radius: 6px;
		color: var(--text-dim);
	}
	.zoom button:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.zv {
		min-width: 42px;
		text-align: center;
		font-size: 11.5px;
		color: var(--text-faint);
		font-variant-numeric: tabular-nums;
	}
	.panel {
		position: absolute;
		top: 12px;
		right: 12px;
		width: 300px;
		max-height: calc(100% - 80px);
		overflow-y: auto;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 12px;
		box-shadow: var(--shadow);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		cursor: default;
		user-select: text;
	}
	.ph {
		display: flex;
		align-items: center;
		gap: 7px;
		font-size: 12px;
	}
	.x {
		margin-left: auto;
		display: inline-flex;
		background: none;
		border: none;
		color: var(--text-faint);
		padding: 2px;
		border-radius: 5px;
	}
	.x:hover {
		color: var(--text);
		background: var(--bg-hover);
	}
	.pt {
		font-size: 14px;
		font-weight: 500;
		line-height: 1.4;
	}
	.sect {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.sh {
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		margin-bottom: 3px;
	}
	.sh span {
		font-weight: 500;
		color: var(--text-faint);
		margin-left: 4px;
	}
	.sh.up {
		color: #fbbf24;
	}
	.sh.down {
		color: #60a5fa;
	}
	.rel {
		display: flex;
		align-items: center;
		gap: 6px;
		background: none;
		border: none;
		text-align: left;
		padding: 4px 6px;
		border-radius: 6px;
		color: var(--text);
		font-size: 12.5px;
		min-width: 0;
	}
	.rel:hover {
		background: var(--bg-hover);
	}
	.rt {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.none {
		font-size: 12px;
		color: var(--text-faint);
		padding: 2px 6px;
	}
	.chainsum {
		font-size: 11.5px;
		color: var(--text-faint);
	}
	.open {
		align-self: flex-start;
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
	@media (prefers-reduced-motion: reduce) {
		.doing :global(svg) {
			animation: none;
		}
	}
</style>
