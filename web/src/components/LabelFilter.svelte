<script>
	import { ListFilter, X } from '@lucide/svelte';
	import { labels, activeLabel, loadIssues } from '$lib/store.js';

	let open = $state(false);
	let query = $state('');
	let inputEl = $state(null);
	const cur = $derived($labels.find((l) => l.id === $activeLabel));
	const filtered = $derived(
		$labels.filter((l) => l.name.toLowerCase().includes(query.trim().toLowerCase()))
	);
	function pick(id) {
		activeLabel.set(id);
		loadIssues();
		open = false;
		query = '';
	}
	function clear() {
		activeLabel.set('');
		loadIssues();
	}
	function openMenu() {
		open = true;
		query = '';
		queueMicrotask(() => inputEl?.focus());
	}
</script>

<div class="lf">
	{#if cur}
		<button class="lf-chip" onclick={clear} title="Clear label filter">
			<span class="lf-dot" style:background={cur.color}></span>{cur.name}
			<X size={13} strokeWidth={2.4} />
		</button>
	{:else}
		<button class="lf-btn" onclick={openMenu}><ListFilter size={15} strokeWidth={2} />Filter</button>
	{/if}
	{#if open}
		<div class="lf-bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="lf-menu">
			<input bind:this={inputEl} class="lf-input" placeholder="Filter by label…" bind:value={query} />
			<div class="lf-list">
				{#each filtered as l (l.id)}
					<button class="lf-item" onclick={() => pick(l.id)}>
						<span class="lf-dot" style:background={l.color}></span>{l.name}
					</button>
				{:else}
					<div class="lf-empty">No labels</div>
				{/each}
			</div>
		</div>
	{/if}
</div>

<style>
	.lf {
		position: relative;
	}
	.lf-btn,
	.lf-chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		color: var(--text-dim);
		padding: 5px 11px;
		font-size: 13px;
	}
	.lf-btn:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.lf-chip {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.lf-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
	}
	.lf-bd {
		position: fixed;
		inset: 0;
		z-index: 40;
	}
	.lf-menu {
		position: absolute;
		top: calc(100% + 5px);
		left: 0;
		z-index: 41;
		width: 220px;
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 9px;
		box-shadow: var(--shadow);
		padding: 5px;
	}
	.lf-input {
		width: 100%;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		color: var(--text);
		padding: 6px 9px;
		font-size: 13px;
		outline: none;
		margin-bottom: 4px;
	}
	.lf-list {
		display: flex;
		flex-direction: column;
		max-height: 240px;
		overflow-y: auto;
	}
	.lf-item {
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
	.lf-item:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.lf-empty {
		padding: 10px;
		color: var(--text-faint);
		font-size: 12.5px;
	}
</style>
