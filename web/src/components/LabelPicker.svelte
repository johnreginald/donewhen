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
						class="input lp-input"
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
	/* The remove mark shows only on the label being pointed at. */
	.lp-chip :global(.lp-x) {
		color: var(--ink-3);
		opacity: 0;
		margin-left: -2px;
	}
	.lp-chip:hover :global(.lp-x),
	.lp-chip:focus-visible :global(.lp-x) {
		opacity: 1;
		color: var(--danger);
	}
	.lp-add {
		position: relative;
	}
	.lp-addbtn {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		background: none;
		border: 1px dashed var(--line);
		border-radius: var(--r-sm);
		color: var(--ink-3);
		padding: 2px 8px;
		font-size: var(--t-sm);
	}
	.lp-addbtn:hover {
		color: var(--ink);
		border-color: var(--ink-3);
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
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
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
		border-bottom: 1px solid var(--line);
	}
	:global(.lp-tagic) {
		color: var(--ink-3);
		flex: none;
	}
	.lp-input {
		flex: 1;
		min-width: 0;
		padding: 4px 8px;
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
		color: var(--ink-2);
		text-align: left;
		padding: 7px 8px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
	}
	.lp-item:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.lp-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
	}
	.lp-empty {
		padding: 10px 8px;
		color: var(--ink-3);
		font-size: var(--t-sm);
	}
</style>
