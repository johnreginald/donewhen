<script>
	// A filter button with a checklist: pick any number of values (OR within the
	// filter). Used for State, Priority and Label in the filter bar.
	import { ChevronDown, Check } from '@lucide/svelte';
	import { toggle } from '$lib/filters.js';

	// options: [{ value, name, color? }]; selected: values; onchange(nextValues)
	let { label, options = [], selected = [], onchange, searchable = false } = $props();

	let open = $state(false);
	let query = $state('');
	let inputEl = $state(null);
	let menuEl = $state(null);
	let btnEl = $state(null);

	const shown = $derived(
		searchable ? options.filter((o) => o.name.toLowerCase().includes(query.trim().toLowerCase())) : options
	);
	const summary = $derived(
		selected.length === 0
			? label
			: selected.length === 1
				? (options.find((o) => o.value === selected[0])?.name ?? label)
				: `${label} · ${selected.length}`
	);

	function openMenu() {
		open = true;
		query = '';
		queueMicrotask(() => (inputEl || menuEl?.querySelector('button'))?.focus());
	}
	function close() {
		open = false;
		btnEl?.focus();
	}
	function onKey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			close();
		} else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			const items = [...menuEl.querySelectorAll('.mf-item')];
			const at = items.indexOf(document.activeElement);
			const next = e.key === 'ArrowDown' ? at + 1 : at - 1;
			items[(next + items.length) % items.length]?.focus();
		}
	}
</script>

<div class="mf">
	<button
		bind:this={btnEl}
		class="mf-btn"
		class:on={selected.length > 0}
		aria-haspopup="true"
		aria-expanded={open}
		onclick={() => (open ? close() : openMenu())}
	>
		{summary}<ChevronDown size={13} strokeWidth={2.4} class="mf-car" />
	</button>
	{#if open}
		<div class="mf-bd" role="presentation" onclick={close}></div>
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div class="mf-menu" bind:this={menuEl} onkeydown={onKey} role="group" aria-label={label}>
			{#if searchable}
				<input bind:this={inputEl} class="input mf-input" placeholder="Filter {label.toLowerCase()}…" bind:value={query} />
			{/if}
			<div class="mf-list">
				{#each shown as o (o.value)}
					{@const on = selected.includes(o.value)}
					<button class="mf-item" class:on aria-pressed={on} onclick={() => onchange?.(toggle(selected, o.value))}>
						<span class="mf-check">{#if on}<Check size={13} strokeWidth={2.6} />{/if}</span>
						{#if o.color}<span class="mf-dot" style:background={o.color}></span>{/if}
						{o.name}
					</button>
				{:else}
					<div class="mf-empty">Nothing to pick</div>
				{/each}
			</div>
			{#if selected.length}
				<button class="mf-clear" onclick={() => onchange?.([])}>Clear {label.toLowerCase()}</button>
			{/if}
		</div>
	{/if}
</div>

<style>
	.mf {
		position: relative;
	}
	.mf-btn {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 30px;
		padding: 0 11px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		color: var(--ink-2);
		font: 500 var(--t-sm)/1 var(--font);
		white-space: nowrap;
	}
	.mf-btn:hover {
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.mf-btn.on {
		border-color: var(--accent);
		color: var(--accent);
	}
	.mf-btn :global(.mf-car) {
		color: var(--ink-3);
	}
	.mf-bd {
		position: fixed;
		inset: 0;
		z-index: 40;
	}
	.mf-menu {
		position: absolute;
		top: calc(100% + 5px);
		left: 0;
		z-index: 41;
		width: 220px;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r);
		box-shadow: var(--shadow-2);
		padding: 5px;
	}
	.mf-input {
		padding: 6px 9px;
		margin-bottom: 4px;
	}
	.mf-list {
		display: flex;
		flex-direction: column;
		max-height: 260px;
		overflow-y: auto;
	}
	.mf-item {
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
	.mf-item:hover,
	.mf-item:focus-visible {
		background: var(--hover);
		color: var(--ink);
	}
	.mf-item.on {
		color: var(--ink);
		font-weight: 500;
	}
	.mf-check {
		width: 14px;
		height: 14px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--accent);
		flex: none;
	}
	.mf-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
	}
	.mf-empty {
		padding: 10px;
		color: var(--ink-3);
		font-size: var(--t-sm);
	}
	.mf-clear {
		width: 100%;
		margin-top: 4px;
		padding: 7px 8px;
		background: none;
		border: none;
		border-top: 1px solid var(--line);
		color: var(--ink-3);
		font-size: var(--t-sm);
		text-align: left;
	}
	.mf-clear:hover {
		color: var(--ink);
	}

	/* mobile: bottom-sheet menu so the page never shifts sideways */
	@media (max-width: 720px) {
		.mf-bd {
			background: oklch(0 0 0 / 0.45);
			z-index: 60;
		}
		.mf-menu {
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			top: auto;
			width: auto;
			z-index: 61;
			border: none;
			border-top: 1px solid var(--line-strong);
			border-radius: var(--r-lg) var(--r-lg) 0 0;
			padding: 12px 12px calc(14px + env(safe-area-inset-bottom, 0px));
		}
		.mf-list {
			max-height: 46vh;
		}
		.mf-item {
			padding: 12px 10px;
			font-size: var(--t-md);
		}
	}
</style>
