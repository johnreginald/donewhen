<script>
	import { dndzone } from 'svelte-dnd-action';
	import { states, issues } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { showToast, openComposer } from '$lib/ui.js';
	import IssueCard from './IssueCard.svelte';
	import StateIcon from './StateIcon.svelte';

	let cols = $state([]);
	let dragging = false;
	let mobileCol = $state(1);
	let mobileInit = false;

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
		const is = $issues;
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
	});

	function consider(i, e) {
		dragging = true;
		cols[i].items = e.detail.items;
	}

	async function finalize(i, e) {
		const col = cols[i];
		col.items = e.detail.items;
		dragging = false;
		try {
			for (let idx = 0; idx < col.items.length; idx++) {
				const it = col.items[idx];
				if (it.stateId !== col.id || it.position !== idx) {
					const moved = it.stateId !== col.id;
					it.stateId = col.id;
					it.position = idx;
					await api.updateIssue(it.id, { stateId: col.id, position: idx });
					if (moved) showToast(`${it.key} → ${col.name}`);
				}
			}
		} catch (err) {
			showToast('Move failed: ' + err.message, 'error');
		}
	}
</script>

<!-- Desktop: horizontal drag-and-drop columns -->
<div class="board desktop">
	{#each cols as col, i (col.id)}
		<div class="column" style:--col-color={col.color}>
			<div class="col-head">
				<StateIcon category={col.category} color={col.color} />
				<span class="col-name">{col.name}</span>
				<span class="count">{col.items.length}</span>
				<span class="spacer"></span>
				<button class="ch-btn" title="Options">⋯</button>
				<button class="ch-btn" title="New issue in {col.name}" onclick={() => openComposer('issue', { stateId: col.id })}>+</button>
			</div>
			<div
				class="col-body"
				use:dndzone={{ items: col.items, flipDurationMs: 150, dropTargetStyle: {} }}
				onconsider={(e) => consider(i, e)}
				onfinalize={(e) => finalize(i, e)}
			>
				{#each col.items as issue (issue.id)}
					<div class="card-wrap">
						<IssueCard {issue} />
					</div>
				{/each}
			</div>
		</div>
	{/each}
</div>

<!-- Mobile: state-tab selector + a single scrolling column -->
<div class="board mobile">
	<div class="mtabs">
		{#each cols as col, i (col.id)}
			<button class="mtab" class:on={i === mobileCol} onclick={() => (mobileCol = i)}>
				<StateIcon category={col.category} color={col.color} size={13} />
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

<style>
	.board {
		display: flex;
		gap: 12px;
		height: 100%;
		overflow-x: auto;
		padding: 12px;
	}
	.column {
		flex: 0 0 300px;
		display: flex;
		flex-direction: column;
		min-height: 0;
		/* subtle per-status tinted lane, Linear-style */
		background: color-mix(in srgb, var(--col-color, var(--border)) 7%, var(--bg));
		border: 1px solid color-mix(in srgb, var(--col-color, var(--border)) 11%, var(--border));
		border-radius: 12px;
		padding: 8px 8px 4px;
	}
	.col-head {
		display: flex;
		align-items: center;
		gap: 7px;
		padding: 4px 6px 10px;
		font-size: 13px;
		font-weight: 500;
	}
	.col-name {
		color: var(--text);
	}
	.count {
		color: var(--text-faint);
		font-size: 12px;
	}
	.col-head .spacer {
		flex: 1;
	}
	.ch-btn {
		width: 20px;
		height: 20px;
		display: grid;
		place-items: center;
		background: none;
		border: none;
		color: var(--text-faint);
		border-radius: 5px;
		font-size: 14px;
		line-height: 1;
		opacity: 0;
		transition: opacity 0.1s, background 0.1s;
	}
	.column:hover .ch-btn {
		opacity: 1;
	}
	.ch-btn:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.col-body {
		display: flex;
		flex-direction: column;
		gap: 8px;
		overflow-y: auto;
		flex: 1;
		min-height: 40px;
		padding: 2px;
		/* overlay-thin scrollbar, revealed on column hover — no layout shift
		   because the 8px gutter is always reserved, only the thumb fades in */
		scrollbar-color: transparent transparent;
		transition: scrollbar-color 0.2s ease;
	}
	.col-body::-webkit-scrollbar-thumb {
		background: transparent;
		border-radius: 8px;
		border: 2px solid transparent;
		background-clip: padding-box;
		transition: background 0.2s ease;
	}
	.column:hover .col-body,
	.col-body:hover,
	.col-body:focus-within {
		scrollbar-color: var(--border-strong) transparent;
	}
	.column:hover .col-body::-webkit-scrollbar-thumb,
	.col-body:hover::-webkit-scrollbar-thumb,
	.col-body:focus-within::-webkit-scrollbar-thumb {
		background: var(--border-strong);
		background-clip: padding-box;
	}
	.col-body::-webkit-scrollbar-thumb:hover {
		background: var(--text-faint);
		background-clip: padding-box;
	}
	.card-wrap {
		outline: none;
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
		border-bottom: 1px solid var(--border);
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
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 20px;
		padding: 6px 12px;
		color: var(--text-dim);
		font-size: 13px;
		font-weight: 500;
	}
	.mtab.on {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--border-strong);
	}
	.mcount {
		color: var(--text-faint);
		font-size: 12px;
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
		.board.desktop {
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
