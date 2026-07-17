<script>
	import { X, Plus, Tag } from '@lucide/svelte';
	import { labels as allLabels } from '$lib/store.js';
	import LabelPill from './LabelPill.svelte';

	let { selected = [], onchange } = $props(); // selected = array of label ids
	let open = $state(false);
	let query = $state('');
	let inputEl = $state(null);

	const chosen = $derived(selected.map((id) => $allLabels.find((l) => l.id === id)).filter(Boolean));
	const filtered = $derived(
		$allLabels.filter(
			(l) => !selected.includes(l.id) && l.name.toLowerCase().includes(query.trim().toLowerCase())
		)
	);

	function add(id) {
		onchange?.([...selected, id]);
		query = '';
		queueMicrotask(() => inputEl?.focus());
	}
	function remove(id) {
		onchange?.(selected.filter((x) => x !== id));
	}
	function openMenu() {
		open = true;
		query = '';
		queueMicrotask(() => inputEl?.focus());
	}
	function onKey(e) {
		if (e.key === 'Enter' && filtered.length) {
			e.preventDefault();
			add(filtered[0].id);
		} else if (e.key === 'Escape') {
			open = false;
		}
	}
</script>

<div class="lp">
	{#each chosen as l (l.id)}
		<button class="lp-chip" onclick={() => remove(l.id)} title="Remove">
			<LabelPill label={l} /><X size={12} strokeWidth={2.4} class="lp-x" />
		</button>
	{/each}
	<div class="lp-add">
		<button class="lp-addbtn" onclick={openMenu}><Plus size={13} strokeWidth={2.4} />label</button>
		{#if open}
			<div class="lp-bd" role="presentation" onclick={() => (open = false)}></div>
			<div class="lp-menu">
				<div class="lp-inputrow">
					<Tag size={13} strokeWidth={2} class="lp-tagic" />
					<input
						bind:this={inputEl}
						bind:value={query}
						onkeydown={onKey}
						class="lp-input"
						placeholder="Filter labels…"
					/>
				</div>
				<div class="lp-list">
					{#each filtered as l (l.id)}
						<button class="lp-item" onclick={() => add(l.id)}>
							<span class="lp-dot" style:background={l.color}></span>{l.name}
						</button>
					{:else}
						<div class="lp-empty">No matching labels</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.lp {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
	}
	.lp-chip {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		background: none;
		border: none;
		padding: 0;
	}
	:global(.lp-x) {
		color: var(--text-faint);
	}
	.lp-chip:hover :global(.lp-x) {
		color: #f87171;
	}
	.lp-add {
		position: relative;
	}
	.lp-addbtn {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		background: var(--bg-elev2);
		border: 1px dashed var(--border-strong);
		border-radius: 6px;
		color: var(--text-dim);
		padding: 3px 9px;
		font-size: 12.5px;
	}
	.lp-addbtn:hover {
		color: var(--text);
		border-color: var(--text-faint);
	}
	.lp-bd {
		position: fixed;
		inset: 0;
		z-index: 30;
	}
	.lp-menu {
		position: absolute;
		top: calc(100% + 5px);
		left: 0;
		z-index: 31;
		width: 240px;
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 9px;
		box-shadow: var(--shadow);
		padding: 5px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.lp-inputrow {
		display: flex;
		align-items: center;
		gap: 7px;
		padding: 5px 8px;
		border-bottom: 1px solid var(--border);
	}
	:global(.lp-tagic) {
		color: var(--text-faint);
		flex: none;
	}
	.lp-input {
		flex: 1;
		background: transparent;
		border: none;
		outline: none;
		color: var(--text);
		font-size: 13px;
	}
	.lp-list {
		display: flex;
		flex-direction: column;
		max-height: 220px;
		overflow-y: auto;
	}
	.lp-item {
		display: flex;
		align-items: center;
		gap: 8px;
		background: none;
		border: none;
		color: var(--text-dim);
		text-align: left;
		padding: 7px 8px;
		border-radius: 6px;
		font-size: 13px;
	}
	.lp-item:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.lp-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
	}
	.lp-empty {
		padding: 10px 8px;
		color: var(--text-faint);
		font-size: 12.5px;
	}
</style>
