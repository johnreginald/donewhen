<script>
	import { dndzone } from 'svelte-dnd-action';
	import { states, issues } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';
	import IssueCard from './IssueCard.svelte';

	let cols = $state([]);
	let dragging = false;

	// Rebuild columns from live data, except while a drag is in flight.
	$effect(() => {
		const st = $states;
		const is = $issues;
		if (dragging) return;
		cols = st.map((s) => ({
			id: s.id,
			name: s.name,
			color: s.color,
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

<div class="board">
	{#each cols as col, i (col.id)}
		<div class="column">
			<div class="col-head">
				<span class="dot" style:background={col.color}></span>
				<span class="col-name">{col.name}</span>
				<span class="count">{col.items.length}</span>
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

<style>
	.board {
		display: flex;
		gap: 12px;
		height: 100%;
		overflow-x: auto;
		padding: 12px;
	}
	.column {
		flex: 0 0 280px;
		display: flex;
		flex-direction: column;
		background: var(--bg);
		min-height: 0;
	}
	.col-head {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 4px 6px 10px;
		font-size: 13px;
		font-weight: 500;
		position: sticky;
		top: 0;
	}
	.count {
		color: var(--text-faint);
		font-size: 12px;
	}
	.col-body {
		display: flex;
		flex-direction: column;
		gap: 8px;
		overflow-y: auto;
		flex: 1;
		min-height: 40px;
		padding: 2px;
	}
	.card-wrap {
		outline: none;
	}
	@media (max-width: 640px) {
		.column {
			flex-basis: 84vw;
		}
	}
</style>
