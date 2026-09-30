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
		<button class="lf-btn" onclick={openMenu}><ListFilter size={15} strokeWidth={2} /><span class="lf-txt">Filter</span></button>
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
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 8px;
		color: var(--ink-2);
		padding: 5px 11px;
		font-size: 13px;
	}
	.lf-btn:hover {
		color: var(--ink);
		border-color: var(--line-strong);
	}
	.lf-chip {
		color: var(--ink);
		border-color: var(--line-strong);
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
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: 9px;
		box-shadow: var(--shadow-2);
		padding: 5px;
	}
	.lf-input {
		width: 100%;
		background: var(--paper);
		border: 1px solid var(--line);
		border-radius: 6px;
		color: var(--ink);
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
		color: var(--ink-2);
		text-align: left;
		padding: 7px 8px;
		border-radius: 6px;
		font-size: 13px;
	}
	.lf-item:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.lf-empty {
		padding: 10px;
		color: var(--ink-3);
		font-size: 12.5px;
	}

	/* mobile: icon-only trigger + bottom-sheet menu (no horizontal overflow → no page shift) */
	@media (max-width: 720px) {
		.lf-txt {
			display: none;
		}
		.lf-btn,
		.lf-chip {
			padding: 7px 9px;
		}
		.lf-bd {
			background: rgba(0, 0, 0, 0.45);
			z-index: 60;
		}
		.lf-menu {
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			top: auto;
			width: auto;
			z-index: 61;
			border: none;
			border-top: 1px solid var(--line-strong);
			border-radius: 16px 16px 0 0;
			padding: 12px 12px calc(14px + env(safe-area-inset-bottom, 0px));
			box-shadow: 0 -8px 30px rgba(0, 0, 0, 0.35);
			animation: lfup 0.18s ease;
		}
		@keyframes lfup {
			from {
				transform: translateY(100%);
			}
		}
		.lf-input {
			font-size: 16px;
			padding: 11px 12px;
			margin-bottom: 8px;
		}
		.lf-list {
			max-height: 46vh;
		}
		.lf-item {
			padding: 12px 10px;
			font-size: 15.5px;
		}
	}
</style>
