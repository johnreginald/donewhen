<script>
	import { dndzone } from 'svelte-dnd-action';
	import { states, visibleIssues, issueQuery, issues, moveIssueTo, activeWorkspace } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { gateFailure, gateSummary } from '$lib/gate.js';
	import { showToast, blockedReasonFor } from '$lib/ui.js';
	import IssueCard from './IssueCard.svelte';
	import StateIcon from './StateIcon.svelte';
	import { ChevronDown, ChevronRight, Lock } from '@lucide/svelte';

	let cols = $state([]);
	let dragging = false;
	let mobileCol = $state(1);
	let mobileInit = false;
	let collapsed = $state(new Set());
	let collapseFor = ''; // workspace id the folds were loaded for

	const loading = $derived($states.length === 0);
	// The whole-board empty state (brand-new workspace) — distinct from a
	// single empty column, which every column renders on its own below.
	const isEmptyWorkspace = $derived(!loading && $issues.length === 0 && !$issueQuery.trim());

	// Every column folds. Done and Canceled start folded; after that the
	// user's choice is kept per device and per workspace.
	const collapseKey = (wsId) => `donewhen.board.collapsed.${wsId}`;
	function canCollapse(col) {
		return true;
	}
	function foldedByDefault(col) {
		return col.category === 'completed' || col.category === 'canceled';
	}
	function readCollapsed(wsId) {
		try {
			const v = JSON.parse(localStorage.getItem(collapseKey(wsId)));
			return Array.isArray(v) ? v : null;
		} catch {
			return null;
		}
	}
	function toggleCollapse(id) {
		const n = new Set(collapsed);
		n.has(id) ? n.delete(id) : n.add(id);
		collapsed = n;
		try {
			localStorage.setItem(collapseKey(collapseFor), JSON.stringify([...n]));
		} catch {
			/* private mode: fold still works for this page */
		}
	}

	// Click-drag to pan the board horizontally (like Linear). Ignores presses on
	// cards/controls so card clicks + dnd reordering still work.
	let boardEl = $state(null);
	let pan = null;
	function panDown(e) {
		if (e.button !== 0) return;
		if (e.target.closest('.card, button, a, input, select, textarea, [role="button"]')) return;
		pan = { x: e.clientX, left: boardEl.scrollLeft };
		boardEl.setPointerCapture?.(e.pointerId);
		boardEl.classList.add('grabbing');
	}
	function panMove(e) {
		if (pan) boardEl.scrollLeft = pan.left - (e.clientX - pan.x);
	}
	function panEnd() {
		pan = null;
		boardEl?.classList.remove('grabbing');
	}

	// Default the mobile tab to the first column that has issues.
	$effect(() => {
		if (mobileInit || !cols.length) return;
		const firstNonEmpty = cols.findIndex((c) => c.items.length);
		mobileCol = firstNonEmpty >= 0 ? firstNonEmpty : Math.min(1, cols.length - 1);
		mobileInit = true;
	});

	// Rebuild columns from live data, except while a drag is in flight.
	$effect(() => {
		const st = $states;
		const is = $visibleIssues;
		if (dragging) return;
		cols = st.map((s) => ({
			id: s.id,
			name: s.name,
			color: s.color,
			category: s.category,
			items: is
				.filter((i) => i.stateId === s.id)
				.sort((a, b) => a.position - b.position)
		}));
		// Done and Canceled start collapsed (decided product rule) — only the
		// first time columns load, so a manual expand/collapse sticks.
		const wsId = $activeWorkspace?.id || '';
		if (wsId && collapseFor !== wsId && cols.length) {
			const stored = readCollapsed(wsId);
			collapsed = stored
				? new Set(stored)
				: new Set(cols.filter(foldedByDefault).map((c) => c.id));
			collapseFor = wsId;
		}
	});

	function consider(i, e) {
		dragging = true;
		cols[i].items = e.detail.items;
	}

	// One request for the one card that moved. The server ranks it between the
	// two neighbours at the drop point (`after` above, `before` below), so cards
	// hidden by a filter keep their order. Store objects are never edited in
	// place: the optimistic move is a copy, and a failure puts the original back.
	async function finalize(i, e) {
		const col = cols[i];
		const movedId = e.detail.info?.id;
		col.items = e.detail.items;
		dragging = false;
		const idx = col.items.findIndex((it) => it.id === movedId);
		if (idx < 0) return; // this zone is where the card left from, not where it landed
		const prevItem = col.items[idx - 1];
		const nextItem = col.items[idx + 1];
		const key = col.items[idx].key;
		const from = ($issues.find((x) => x.id === movedId) || {}).stateId;
		const reason = await blockedReasonFor(from, col.id, key);
		if (reason === null) {
			// Cancelled: rebuild the columns from the store so the card snaps back.
			issues.update((l) => [...l]);
			return;
		}
		try {
			const r = await moveIssueTo(movedId, col.id, prevItem, nextItem, reason);
			if (r && r.original.stateId !== col.id) showToast(`${r.original.key} → ${col.name}`);
		} catch (err) {
			const g = gateFailure(err);
			showToast(
				g ? gateSummary(g, key) + ' ' + g.open.map((o) => `${o.index}. ${o.text}`).join('; ') : 'Move failed: ' + err.message,
				'error'
			);
		}
	}
</script>

{#if loading}
	<div class="board desktop loading">
		{#each Array(9) as _, i (i)}
			<div class="column">
				<div class="col-head">
					<span class="skel" style:width="14px" style:height="14px" style:border-radius="50%"></span>
					<span class="skel" style:width="70px" style:height="11px"></span>
				</div>
				<div class="skelcard">
					<span class="skel" style:width="38%" style:height="9px"></span>
					<span class="skel" style:width="85%" style:height="13px"></span>
					<span class="skel" style:width="46%" style:height="17px" style:border-radius="10px"></span>
				</div>
			</div>
		{/each}
	</div>
{:else if isEmptyWorkspace}
	<div class="empty-board">
		<div class="eglyph"><span></span><span></span><span></span></div>
		<div class="eh">Nothing on the board yet</div>
		<div class="ep">New workspace, empty record. Create the first issue and it lands in Triage.</div>
	</div>
{:else}
	<!-- Desktop: horizontal drag-and-drop columns. The pointer handlers are a
	     click-drag-to-pan convenience (ignored over cards/controls), not a
	     control in their own right — pre-existing pattern, kept as-is. -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="board desktop"
		bind:this={boardEl}
		onpointerdown={panDown}
		onpointermove={panMove}
		onpointerup={panEnd}
		onpointercancel={panEnd}
	>
		{#each cols as col, i (col.id)}
			{#if canCollapse(col) && collapsed.has(col.id)}
				<div class="column is-collapsed">
					<button class="colhv" onclick={() => toggleCollapse(col.id)} title="Expand {col.name}">
						<StateIcon category={col.category} color={col.color} name={col.name} />
						<span class="cn">{col.items.length}</span>
						<span class="cname-v">{col.name}</span>
						<ChevronRight size={13} strokeWidth={2.2} />
					</button>
				</div>
			{:else}
				<div class="column" style:--col-color={col.color}>
					<div class="col-head">
						<StateIcon category={col.category} color={col.color} name={col.name} />
						<span class="col-name">{col.name}</span>
						<span class="count">{col.items.length}</span>
						<span class="spacer"></span>
						{#if $issueQuery.trim()}
							<span class="lockbadge" title="Drag disabled while filtered"><Lock size={11} strokeWidth={2.2} /></span>
						{/if}
						{#if canCollapse(col)}
							<button class="ch-btn" title="Collapse {col.name}" onclick={() => toggleCollapse(col.id)}>
								<ChevronDown size={14} strokeWidth={2.2} />
							</button>
						{/if}
					</div>
					<div class="col-body-wrap">
						<div
							class="col-body"
							use:dndzone={{ items: col.items, flipDurationMs: 150, dropTargetStyle: {}, dragDisabled: !!$issueQuery.trim() }}
							onconsider={(e) => consider(i, e)}
							onfinalize={(e) => finalize(i, e)}
						>
							{#each col.items as issue (issue.id)}
								<div class="card-wrap">
									<IssueCard {issue} />
								</div>
							{/each}
						</div>
						{#if !col.items.length}
							<div class="colempty">No issues</div>
						{/if}
					</div>
				</div>
			{/if}
		{/each}
	</div>

	<!-- Mobile: state-tab selector + a single scrolling column -->
	<div class="board mobile">
		<div class="mtabs">
			{#each cols as col, i (col.id)}
				<button class="mtab" class:on={i === mobileCol} onclick={() => (mobileCol = i)}>
					<StateIcon category={col.category} color={col.color} name={col.name} size={13} />
					<span>{col.name}</span>
					<span class="mcount">{col.items.length}</span>
				</button>
			{/each}
		</div>
		<div class="mlist">
			{#each cols[mobileCol]?.items ?? [] as issue (issue.id)}
				<IssueCard {issue} />
			{:else}
				<div class="mempty faint">Nothing in {cols[mobileCol]?.name ?? 'this state'}.</div>
			{/each}
		</div>
	</div>
{/if}

<style>
	.board {
		display: flex;
		gap: 10px;
		height: 100%;
		overflow-x: auto;
		padding: 12px;
	}
	.board.desktop.grabbing {
		cursor: grabbing;
		user-select: none;
	}
	.column {
		flex: 0 0 252px;
		display: flex;
		flex-direction: column;
		min-height: 0;
		background: var(--sunken);
		border-radius: var(--r-lg);
		padding: 9px 9px 4px;
		box-sizing: border-box;
	}
	.column.is-collapsed {
		flex: 0 0 44px;
		align-items: center;
		padding: 10px 0;
	}
	.colhv {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 9px;
		background: none;
		border: none;
		color: var(--ink-2);
		padding: 0;
	}
	.cname-v {
		writing-mode: vertical-rl;
		transform: rotate(180deg);
		font-size: var(--t-sm);
		font-weight: 500;
		color: var(--ink-2);
		letter-spacing: 0.01em;
	}
	.cn {
		font-size: var(--t-xs);
		color: var(--ink-3);
		font-family: var(--mono);
	}
	.col-head {
		display: flex;
		align-items: center;
		gap: 7px;
		padding: 2px 4px 8px;
		font-size: var(--t-sm);
		font-weight: 500;
	}
	.col-name {
		color: var(--ink);
		font-size: var(--t-base);
		font-weight: 500;
	}
	.count {
		color: var(--ink-3);
		font-size: var(--t-sm);
		font-weight: 400;
	}
	.col-head .spacer {
		flex: 1;
	}
	.lockbadge {
		display: flex;
		color: var(--ink-3);
		opacity: 0.8;
	}
	.ch-btn {
		width: 20px;
		height: 20px;
		display: grid;
		place-items: center;
		background: none;
		border: none;
		color: var(--ink-3);
		border-radius: var(--r-sm);
		flex: none;
	}
	.ch-btn:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.col-body-wrap {
		position: relative;
		flex: 1;
		min-height: 40px;
		display: flex;
	}
	.col-body {
		display: flex;
		flex-direction: column;
		gap: 8px;
		overflow-y: auto;
		flex: 1;
		min-height: 40px;
		padding: 2px;
		scrollbar-color: transparent transparent;
		transition: scrollbar-color 0.2s ease;
	}
	.col-body::-webkit-scrollbar-thumb {
		background: transparent;
		border-radius: var(--r);
		border: 2px solid transparent;
		background-clip: padding-box;
		transition: background 0.2s ease;
	}
	.column:hover .col-body,
	.col-body:hover,
	.col-body:focus-within {
		scrollbar-color: var(--line-strong) transparent;
	}
	.column:hover .col-body::-webkit-scrollbar-thumb,
	.col-body:hover::-webkit-scrollbar-thumb,
	.col-body:focus-within::-webkit-scrollbar-thumb {
		background: var(--line-strong);
		background-clip: padding-box;
	}
	.col-body::-webkit-scrollbar-thumb:hover {
		background: var(--ink-3);
		background-clip: padding-box;
	}
	.card-wrap {
		outline: none;
	}
	.colempty {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 26px 8px;
		text-align: center;
		color: var(--ink-3);
		font-size: var(--t-sm);
		pointer-events: none;
	}

	/* drag feedback — the ghost sits where the card will land; the actively
	   dragged element lifts with a stronger shadow, matching the design. */
	.col-body :global([data-is-dnd-shadow-item-hint]) {
		background: var(--hover) !important;
		border: 1.5px dashed var(--line-strong) !important;
		border-radius: var(--r);
		box-shadow: none !important;
		opacity: 0.7;
	}
	:global(#dnd-action-dragged-el) {
		box-shadow: var(--shadow-2) !important;
		border-radius: var(--r);
	}
	:global(#dnd-action-dragged-el .card) {
		transform: rotate(-1.5deg);
		border-color: var(--line-strong) !important;
	}

	/* loading skeleton */
	.skelcard {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 10px 11px;
		display: flex;
		flex-direction: column;
		gap: 9px;
	}
	.skel {
		display: block;
		background: var(--hover);
		border-radius: var(--r-sm);
	}
	@media (prefers-reduced-motion: no-preference) {
		.skel {
			animation: shimmer 1.7s ease-in-out infinite;
		}
	}
	@keyframes shimmer {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.55;
		}
	}

	/* whole-board empty state (brand-new workspace) */
	.empty-board {
		height: 100%;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
		color: var(--ink-2);
		text-align: center;
		padding: 40px;
	}
	.eglyph {
		display: flex;
		gap: 8px;
		margin-bottom: 4px;
	}
	.eglyph span {
		width: 20px;
		height: 20px;
		border-radius: 50%;
		border: 1.8px solid var(--line-strong);
	}
	.eh {
		font-family: var(--serif);
		font-weight: 400;
		font-size: var(--t-2xl);
		color: var(--ink);
	}
	.ep {
		max-width: 38ch;
		color: var(--ink-2);
		font-size: var(--t-base);
	}

	/* mobile board */
	.board.mobile {
		display: none;
	}
	.mtabs {
		display: flex;
		gap: 6px;
		overflow-x: auto;
		padding: 10px 12px;
		border-bottom: 1px solid var(--line);
		scrollbar-width: none;
	}
	.mtabs::-webkit-scrollbar {
		display: none;
	}
	.mtab {
		display: flex;
		align-items: center;
		gap: 6px;
		white-space: nowrap;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 20px; /* ds-ok: Board Mobile specimen tab pill */
		padding: 0 13px;
		height: 40px;
		box-sizing: border-box;
		color: var(--ink-2);
		font-size: 13.5px; /* ds-ok: Board Mobile specimen tab text */
		font-weight: 500;
	}
	.mtab.on {
		background: var(--sunken);
		color: var(--ink);
		border-color: var(--line-strong);
	}
	.mcount {
		color: var(--ink-3);
		font-size: var(--t-sm);
	}
	.mlist {
		flex: 1;
		overflow-y: auto;
		padding: 12px;
		display: flex;
		flex-direction: column;
		gap: 9px;
	}
	.mempty {
		padding: 44px 10px;
		text-align: center;
	}

	@media (max-width: 720px) {
		.board.desktop:not(.loading) {
			display: none;
		}
		.board.mobile {
			display: flex;
			flex-direction: column;
			height: 100%;
			overflow: hidden;
			padding: 0;
		}
	}
</style>
