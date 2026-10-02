<script>
	// List — every issue, grouped by workflow state (default), epic or
	// priority. Dense 36px rows, sticky group headers, j/k keyboard nav,
	// checkbox multi-select with a bulk bar that reuses the plain issue
	// PATCH (state / labels / priority) — no new endpoints.
	import {
		visibleIssues,
		states,
		projects,
		labels as labelStore,
		blockLinks,
		openBlockersByIssue,
		loadIssues,
		PRIORITIES
	} from '$lib/store.js';
	import PageHeader from '$components/PageHeader.svelte';
	import IssuesToolbar from '$components/IssuesToolbar.svelte';
	import { autohide } from '$lib/autohide.js';
	import StateIcon from '$components/StateIcon.svelte';
	import PriorityIcon from '$components/PriorityIcon.svelte';
	import LabelPill from '$components/LabelPill.svelte';
	import { openIssue, showToast, askBlockedReason } from '$lib/ui.js';
	import { needsReason } from '$lib/blocked.js';
	import { rel } from '$lib/format.js';
	import { api } from '$lib/api.js';

	const stOf = (id) => $states.find((s) => s.id === id);
	const epicOf = (id) => $projects.find((p) => p.id === id);
	const openBlockers = $derived(openBlockersByIssue($blockLinks));

	const LS_KEY = 'donewhen.list.collapsedStates';
	// First visit: only the "live" categories start open, same as the design.
	const DEFAULT_OPEN = new Set(['Ready', 'In Progress', 'In Review']);
	let collapsed = $state(null); // null = not computed yet (waiting on $states)
	$effect(() => {
		if (collapsed !== null || !$states.length) return;
		try {
			const raw = localStorage.getItem(LS_KEY);
			if (raw !== null) {
				collapsed = new Set(JSON.parse(raw));
				return;
			}
		} catch {
			/* fall through to default */
		}
		collapsed = new Set($states.filter((s) => !DEFAULT_OPEN.has(s.name)).map((s) => s.id));
	});
	function toggleGroup(id) {
		const n = new Set(collapsed);
		n.has(id) ? n.delete(id) : n.add(id);
		collapsed = n;
		try {
			localStorage.setItem(LS_KEY, JSON.stringify([...n]));
		} catch {
			/* storage blocked: not remembered */
		}
	}

	// ── group / sort ──────────────────────────────────────────────────
	let groupBy = $state('state'); // state | epic | priority
	let sortBy = $state('updated'); // updated | priority | created
	let menuOpen = $state(false);

	function sortRows(list) {
		const copy = [...list];
		if (sortBy === 'priority') copy.sort((a, b) => (a.priority || 99) - (b.priority || 99) || a.number - b.number);
		else if (sortBy === 'created') copy.sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt));
		else copy.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
		return copy;
	}

	const groups = $derived(build($visibleIssues, $states, $projects, groupBy, sortBy));
	function build(list, sts, projs, gb, sb) {
		if (gb === 'epic') {
			const byEpic = new Map();
			for (const i of list) {
				const k = i.projectId || '__none__';
				if (!byEpic.has(k)) byEpic.set(k, []);
				byEpic.get(k).push(i);
			}
			const out = [...byEpic.entries()].map(([id, rows]) => ({
				id,
				name: id === '__none__' ? 'No epic' : projs.find((p) => p.id === id)?.name || 'No epic',
				rows: sortRows(rows)
			}));
			out.sort((a, b) => (a.name === 'No epic' ? 1 : 0) - (b.name === 'No epic' ? 1 : 0) || a.name.localeCompare(b.name));
			return out;
		}
		if (gb === 'priority') {
			const order = [1, 2, 3, 4, 0];
			const byPri = new Map(order.map((p) => [p, []]));
			for (const i of list) byPri.get(i.priority ?? 0).push(i);
			return order.map((p) => ({
				id: 'p' + p,
				name: PRIORITIES.find((x) => x.value === p)?.label ?? 'No priority',
				pvalue: p,
				rows: sortRows(byPri.get(p))
			}));
		}
		// default: state, in workflow order
		const byState = new Map(sts.map((s) => [s.id, []]));
		for (const i of list) {
			if (!byState.has(i.stateId)) byState.set(i.stateId, []);
			byState.get(i.stateId).push(i);
		}
		return sts
			.slice()
			.sort((a, b) => a.position - b.position)
			.map((s) => ({ id: s.id, name: s.name, category: s.category, color: s.color, rows: sortRows(byState.get(s.id) || []) }));
	}
	const totalRows = $derived(groups.reduce((n, g) => n + g.rows.length, 0));

	// j / k / Enter come from the layout's shortcut handler: it moves real focus
	// across the rows, and Enter on a focused row is just its click.

	// ── selection + bulk bar (existing PATCH /api/issues/:id only) ─────
	let selected = $state(new Set());
	function toggleSel(id) {
		const n = new Set(selected);
		n.has(id) ? n.delete(id) : n.add(id);
		selected = n;
	}
	function clearSel() {
		selected = new Set();
	}
	let bulkMenu = $state('');
	async function runBulk(label, fn) {
		bulkMenu = '';
		const n = selected.size;
		try {
			await Promise.all([...selected].map(fn));
			showToast(`${label} on ${n} issue${n === 1 ? '' : 's'}`, 'info');
			clearSel();
			await loadIssues();
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function bulkSetState(stateId) {
		// One reason covers the whole selection when it is moving into Blocked.
		const entering = [...selected].filter((id) => needsReason($states, $visibleIssues.find((i) => i.id === id)?.stateId, stateId));
		let blockedReason;
		if (entering.length) {
			blockedReason = await askBlockedReason(entering.length === 1 ? $visibleIssues.find((i) => i.id === entering[0])?.key : `${entering.length} issues`);
			if (blockedReason === null) return;
		}
		runBulk('Moved', (id) => api.updateIssue(id, entering.includes(id) ? { stateId, blockedReason } : { stateId }));
	}
	const bulkSetPriority = (priority) => runBulk('Priority set', (id) => api.updateIssue(id, { priority }));
	const bulkAddLabel = (label) =>
		runBulk('Label added', (id) => {
			const issue = $visibleIssues.find((i) => i.id === id);
			if (!issue || issue.labels.some((l) => l.id === label.id)) return Promise.resolve();
			return api.updateIssue(id, { labelIds: [...issue.labels.map((l) => l.id), label.id] });
		});

	const loading = $derived($states.length === 0);
</script>

<div class="page">
	<PageHeader crumbs={[{ label: 'Tasks', href: '/board' }, { label: 'List' }]} />
	<IssuesToolbar />
	<div class="subbar" use:autohide>
		<span class="spacer"></span>
		<div class="dd">
			<button class="gsbtn" onclick={() => (menuOpen = !menuOpen)} aria-haspopup="true" aria-expanded={menuOpen}>
				⇅ Sort &amp; group <span class="chev">▾</span>
			</button>
			{#if menuOpen}
				<div class="dd-bd" role="presentation" onclick={() => (menuOpen = false)}></div>
				<div class="gsmenu" role="menu">
					<div class="gsh">Group by</div>
					{#each [['state', 'State'], ['epic', 'Epic'], ['priority', 'Priority']] as [v, l] (v)}
						<button class="gsi" class:sel={groupBy === v} onclick={() => ((groupBy = v), (menuOpen = false))}>
							{l}<span class="sp"></span>{#if groupBy === v}✓{/if}
						</button>
					{/each}
					<div class="gsh">Sort by</div>
					{#each [['priority', 'Priority'], ['updated', 'Updated'], ['created', 'Created']] as [v, l] (v)}
						<button class="gsi" class:sel={sortBy === v} onclick={() => ((sortBy = v), (menuOpen = false))}>
							{l}<span class="sp"></span>{#if sortBy === v}✓{/if}
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="colhd" aria-hidden="true">
			<span></span><span>Pri</span><span>Key</span><span></span><span>Title</span><span>Labels</span><span>Epic</span><span>Blocked</span><span>Updated</span>
		</div>
		<div class="lbody">
			{#each Array(10) as _, i (i)}
				<div class="skrow">
					<span class="skel" style="width:14px;height:14px;border-radius:4px"></span>
					<span class="skel" style="width:16px;height:11px"></span>
					<span class="skel" style="width:44px;height:11px"></span>
					<span class="skel" style="width:13px;height:13px;border-radius:50%"></span>
					<span class="skel" style="width:{40 + ((i * 17) % 45)}%;height:12px"></span>
					<span class="skel" style="width:60px;height:16px;border-radius:999px"></span>
				</div>
			{/each}
		</div>
	{:else if !totalRows}
		<div class="empty">
			<div class="ic">—</div>
			<p class="etitle">No issues match</p>
			<p class="esub">Try clearing filters, or press ⌘K to create one.</p>
		</div>
	{:else}
		<div class="colhd" aria-hidden="true">
			<span></span><span>Pri</span><span>Key</span><span></span><span>Title</span><span>Labels</span><span>Epic</span><span>Blocked</span><span>Updated</span>
		</div>
		<div class="lbody">
			{#each groups as g (g.id)}
				<button class="lgrp" onclick={() => toggleGroup(g.id)} aria-expanded={!collapsed?.has(g.id)}>
					<span class="chv" class:open={!collapsed?.has(g.id)}>›</span>
					{#if groupBy === 'state'}
						<StateIcon category={g.category} color={g.color} size={13} />
					{:else if groupBy === 'priority'}
						<PriorityIcon priority={g.pvalue} />
					{/if}
					<span class="gname">{g.name}</span>
					<span class="n">{g.rows.length}</span>
				</button>
				{#if !collapsed?.has(g.id)}
					{#each g.rows as r (r.id)}
						{@const st = stOf(r.stateId)}
						{@const ep = epicOf(r.projectId)}
						{@const waits = (openBlockers[r.id] || []).length}
						<button
							class="lrow"
							data-issue-key={r.key}
							class:selr={selected.has(r.id)}
							onclick={() => openIssue(r.key)}
						>
							<span
								class="cbx"
								class:on={selected.has(r.id)}
								role="checkbox"
								aria-checked={selected.has(r.id)}
								aria-label="Select {r.key}"
								tabindex="0"
								onclick={(e) => {
									e.stopPropagation();
									toggleSel(r.id);
								}}
								onkeydown={(e) => {
									if (e.key === ' ' || e.key === 'Enter') {
										e.preventDefault();
										e.stopPropagation();
										toggleSel(r.id);
									}
								}}
							></span>
							<PriorityIcon priority={r.priority} />
							<span class="k">{r.key}</span>
							<StateIcon category={st?.category} color={st?.color} size={13} />
							<span class="t">{r.title}</span>
							<span class="lbls">{#each r.labels as l (l.id)}<LabelPill label={l} />{/each}</span>
							<span class="ep">{ep?.name || ''}</span>
							<span class="blkwrap">{#if waits}<span class="blk" title="{waits} blocker{waits === 1 ? '' : 's'} still open">⛌ {waits}</span>{/if}</span>
							<span class="d">{rel(r.updatedAt)}</span>
						</button>
					{/each}
				{/if}
			{/each}
		</div>
	{/if}

	{#if selected.size}
		<div class="bulkbar">
			<b>{selected.size} selected</b>
			<span class="bd"></span>
			<div class="dd">
				<button class="bbtn" onclick={() => (bulkMenu = bulkMenu === 'state' ? '' : 'state')}>Move state</button>
				{#if bulkMenu === 'state'}
					<div class="dd-bd" role="presentation" onclick={() => (bulkMenu = '')}></div>
					<div class="bmenu" role="menu">
						{#each $states as s (s.id)}
							<button onclick={() => bulkSetState(s.id)}><StateIcon category={s.category} color={s.color} size={12} />{s.name}</button>
						{/each}
					</div>
				{/if}
			</div>
			<div class="dd">
				<button class="bbtn" onclick={() => (bulkMenu = bulkMenu === 'label' ? '' : 'label')}>Add label</button>
				{#if bulkMenu === 'label'}
					<div class="dd-bd" role="presentation" onclick={() => (bulkMenu = '')}></div>
					<div class="bmenu" role="menu">
						{#each $labelStore as l (l.id)}
							<button onclick={() => bulkAddLabel(l)}><span class="ldot" style:background={l.color}></span>{l.name}</button>
						{:else}
							<div class="bnone">No labels yet.</div>
						{/each}
					</div>
				{/if}
			</div>
			<div class="dd">
				<button class="bbtn" onclick={() => (bulkMenu = bulkMenu === 'priority' ? '' : 'priority')}>Set priority</button>
				{#if bulkMenu === 'priority'}
					<div class="dd-bd" role="presentation" onclick={() => (bulkMenu = '')}></div>
					<div class="bmenu" role="menu">
						{#each PRIORITIES as p (p.value)}
							<button onclick={() => bulkSetPriority(p.value)}>{p.label}</button>
						{/each}
					</div>
				{/if}
			</div>
			<span class="bd"></span>
			<button class="x" onclick={clearSel} aria-label="Clear selection">✕</button>
		</div>
	{/if}
</div>

<style>
	.page {
		height: 100%;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.subbar {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 0 20px 9px;
		flex: none;
		position: relative;
	}
	.spacer {
		flex: 1;
	}
	.gsbtn {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 27px;
		padding: 0 10px;
		border-radius: var(--r-sm);
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--ink);
		font-size: var(--t-sm);
	}
	.gsbtn:hover {
		border-color: var(--line-strong);
	}
	.chev {
		color: var(--ink-3);
		font-size: var(--t-xs);
	}
	.gsmenu {
		position: absolute;
		top: calc(100% + 2px);
		right: 20px;
		z-index: 32;
		width: 200px;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		padding: 6px;
	}
	.gsh {
		font: 600 var(--t-xs) var(--mono);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 7px 8px 4px;
	}
	.gsi {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		height: 27px;
		padding: 0 8px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
		color: var(--ink);
		background: none;
		border: none;
		text-align: left;
	}
	.gsi:hover {
		background: var(--hover);
	}
	.gsi.sel {
		background: var(--accent-soft);
		color: var(--accent);
		font-weight: 500;
	}
	.gsi .sp {
		flex: 1;
	}

	/* column header + rows */
	.colhd {
		display: grid;
		grid-template-columns: 20px 26px 58px 15px minmax(0, 1fr) 148px 126px 76px 60px;
		gap: 10px;
		align-items: center;
		height: 25px;
		padding: 0 20px;
		font: 600 var(--t-xs) var(--mono);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		border-bottom: 1px solid var(--line);
		border-top: 1px solid var(--line);
		flex: none;
	}
	.lbody {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		position: relative;
	}
	.lgrp {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		height: 32px;
		padding: 0 20px;
		background: var(--sunken);
		border: none;
		border-bottom: 1px solid var(--line);
		font-size: var(--t-sm);
		font-weight: 500;
		color: var(--ink);
		text-align: left;
		position: sticky;
		top: 0;
		z-index: 2;
	}
	.chv {
		display: inline-block;
		font-size: var(--t-xs);
		color: var(--ink-3);
		transition: transform var(--dur) var(--ease);
	}
	.chv.open {
		transform: rotate(90deg);
	}
	.gname {
		font-weight: 500;
	}
	.n {
		color: var(--ink-3);
		font-weight: 400;
		font-family: var(--mono);
		font-size: var(--t-xs);
	}
	.lrow {
		display: grid;
		grid-template-columns: 20px 26px 58px 15px minmax(0, 1fr) 148px 126px 76px 60px;
		gap: 10px;
		align-items: center;
		width: 100%;
		height: 36px;
		padding: 0 20px;
		border: none;
		border-bottom: 1px solid var(--line);
		background: none;
		color: var(--ink);
		font-size: var(--t-sm);
		text-align: left;
	}
	.lrow:hover {
		background: var(--hover);
	}
	.lrow.selr {
		background: var(--accent-soft);
	}
	.lrow:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.k {
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.t {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.ep {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--t-xs);
		color: var(--ink-2);
	}
	.d {
		font-size: var(--t-xs);
		color: var(--ink-3);
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.lbls {
		display: flex;
		gap: 4px;
		overflow: hidden;
	}
	.blkwrap {
		display: flex;
		align-items: center;
	}
	.blk {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		height: 19px;
		padding: 0 6px;
		border-radius: var(--r-sm);
		background: var(--danger-soft);
		color: var(--danger);
		font: 500 var(--t-xs) var(--mono);
	}
	.cbx {
		width: 14px;
		height: 14px;
		border-radius: var(--r-sm);
		border: 1.5px solid var(--line-strong);
		background: var(--surface);
		display: inline-block;
	}
	.cbx:hover {
		border-color: var(--ink-3);
	}
	.cbx.on {
		background: var(--accent);
		border-color: var(--accent);
	}

	/* loading skeleton */
	.skel {
		background: linear-gradient(90deg, var(--sunken) 25%, var(--hover) 37%, var(--sunken) 63%);
		background-size: 400% 100%;
		animation: skshim 1.6s ease infinite;
		border-radius: var(--r-sm);
		display: inline-block;
	}
	@keyframes skshim {
		0% {
			background-position: 100% 0;
		}
		100% {
			background-position: 0 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.skel {
			animation: none;
		}
	}
	.skrow {
		display: grid;
		grid-template-columns: 22px 56px 16px minmax(0, 1fr) 84px 46px;
		gap: 10px;
		align-items: center;
		height: 36px;
		padding: 0 20px;
		border-bottom: 1px solid var(--line);
	}

	/* empty */
	.empty {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 9px;
		padding: 24px;
		text-align: center;
	}
	.ic {
		width: 38px;
		height: 38px;
		border-radius: 50%;
		border: 1.5px dashed var(--line-strong);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--ink-3);
		font-size: var(--t-base);
	}
	.etitle {
		margin: 0;
		font-size: var(--t-base);
		font-weight: 600;
		color: var(--ink);
	}
	.esub {
		margin: 0;
		font-size: var(--t-sm);
		color: var(--ink-3);
		max-width: 260px;
		line-height: 1.45;
	}

	/* bulk bar */
	.bulkbar {
		position: sticky;
		bottom: 16px;
		left: 50%;
		transform: translateX(0);
		margin: 0 auto 0;
		width: fit-content;
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--ink);
		color: var(--paper);
		border-radius: var(--r-lg);
		padding: 7px 8px 7px 14px;
		box-shadow: var(--shadow-2);
		font-size: var(--t-sm);
		z-index: 6;
	}
	.bulkbar b {
		font-weight: 600;
		white-space: nowrap;
	}
	.bulkbar .bd {
		width: 1px;
		height: 16px;
		background: oklch(1 0 0 / 0.18);
		flex: none;
	}
	.bulkbar .dd {
		position: relative;
	}
	.bulkbar .bbtn {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 26px;
		padding: 0 10px;
		border-radius: var(--r);
		background: oklch(1 0 0 / 0.08);
		color: var(--paper);
		font-size: var(--t-sm);
		border: none;
		white-space: nowrap;
	}
	.bulkbar .bbtn:hover {
		background: oklch(1 0 0 / 0.16);
	}
	.bulkbar .x {
		background: none;
		border: none;
		color: var(--paper);
		padding: 4px 6px;
		border-radius: var(--r-sm);
	}
	.bulkbar .x:hover {
		background: oklch(1 0 0 / 0.16);
	}
	.bmenu {
		position: absolute;
		bottom: calc(100% + 6px);
		left: 0;
		z-index: 31;
		min-width: 170px;
		max-height: 260px;
		overflow-y: auto;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r);
		box-shadow: var(--shadow-2);
		padding: 4px;
		display: flex;
		flex-direction: column;
	}
	.bmenu button {
		display: flex;
		align-items: center;
		gap: 8px;
		background: none;
		border: none;
		text-align: left;
		padding: 7px 9px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
		color: var(--ink);
	}
	.bmenu button:hover {
		background: var(--hover);
	}
	.bnone {
		padding: 7px 9px;
		font-size: var(--t-sm);
		color: var(--ink-3);
	}
	.ldot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		flex: none;
	}

	/* mobile: two-line row — glyph + title + priority, then key, epic, updated */
	@media (max-width: 720px) {
		.colhd {
			display: none;
		}
		.lgrp {
			height: 30px;
			padding: 0 16px;
		}
		.lrow {
			grid-template-columns: 16px 14px auto minmax(0, 1fr) auto;
			grid-template-areas:
				'cbx st t t pri'
				'cbx . k ep d';
			column-gap: 9px;
			row-gap: 3px;
			height: auto;
			min-height: 52px;
			padding: 9px 16px;
		}
		.lrow > :global(.cbx) {
			grid-area: cbx;
		}
		.lrow > :global(.sicon) {
			grid-area: st;
		}
		.lrow > :global(.pri3),
		.lrow > :global(.urg) {
			grid-area: pri;
		}
		.t {
			grid-area: t;
		}
		.k {
			grid-area: k;
			white-space: nowrap;
		}
		.ep {
			grid-area: ep;
		}
		.d {
			grid-area: d;
		}
		.lbls,
		.blkwrap {
			display: none;
		}
		.subbar {
			padding: 0 16px 9px;
		}
	}
</style>
