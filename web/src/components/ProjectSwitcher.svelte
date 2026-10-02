<script>
	import { initiatives, projects, activeInitiative, activeProject, loadIssues } from '$lib/store.js';
	import { ChevronDown, ChevronRight, Check, Layers, Box, Hexagon } from '@lucide/svelte';

	let open = $state(false);
	let ex = $state(new Set()); // expanded initiatives
	function toggle(id) {
		const n = new Set(ex);
		n.has(id) ? n.delete(id) : n.add(id);
		ex = n;
	}

	const label = $derived(
		$activeProject
			? ($projects.find((p) => p.id === $activeProject)?.name ?? 'Epic')
			: $activeInitiative
				? ($initiatives.find((i) => i.id === $activeInitiative)?.name ?? 'Project')
				: 'All Issues'
	);

	const epicsOf = (iniId) => $projects.filter((p) => p.initiativeId === iniId);

	function pickAll() {
		activeInitiative.set('');
		activeProject.set('');
		loadIssues();
		open = false;
	}
	function pickInitiative(id) {
		activeInitiative.set(id);
		activeProject.set('');
		loadIssues();
		open = false;
	}
	function pickEpic(id) {
		activeProject.set(id);
		activeInitiative.set('');
		loadIssues();
		open = false;
	}
</script>

<button class="psw" onclick={() => (open = true)} aria-label="Filter by project or epic">
	{#if $activeProject}<Box size={14} strokeWidth={2} />{:else}<Layers size={15} strokeWidth={2} />{/if}
	<span class="psw-name">{label}</span>
	<ChevronDown size={13} strokeWidth={2.4} />
</button>

{#if open}
	<div class="sheet-bd" role="presentation" onclick={() => (open = false)}></div>
	<div class="sheet" role="dialog" aria-label="Filter">
		<div class="grip"></div>
		<div class="sheet-h">Show</div>
		<div class="scroll">
			<button class="item" class:on={!$activeInitiative && !$activeProject} onclick={pickAll}>
				<Layers size={17} strokeWidth={2} /><span class="nm">All Issues</span>
				{#if !$activeInitiative && !$activeProject}<Check size={16} strokeWidth={2.5} />{/if}
			</button>
			{#each $initiatives as i (i.id)}
				{@const eps = epicsOf(i.id)}
				{@const isOpen = ex.has(i.id) || eps.some((p) => p.id === $activeProject)}
				<div class="row">
					<button class="cx" class:open={isOpen} class:empty={eps.length === 0} onclick={() => toggle(i.id)} aria-label="Expand epics">
						<ChevronRight size={16} strokeWidth={2.5} />
					</button>
					<button class="item ini" class:on={$activeInitiative === i.id} onclick={() => pickInitiative(i.id)}>
						<Hexagon size={16} strokeWidth={2} /><span class="nm">{i.name}</span>
						{#if $activeInitiative === i.id}<Check size={16} strokeWidth={2.5} />{/if}
					</button>
				</div>
				{#if isOpen}
					{#each eps as p (p.id)}
						<button class="item epic" class:on={$activeProject === p.id} onclick={() => pickEpic(p.id)}>
							<Box size={15} strokeWidth={2} /><span class="nm">{p.name}</span>
							{#if $activeProject === p.id}<Check size={15} strokeWidth={2.5} />{/if}
						</button>
					{/each}
				{/if}
			{/each}
		</div>
	</div>
{/if}

<style>
	.psw {
		display: none;
		align-items: center;
		gap: 6px;
		background: none;
		border: none;
		color: var(--ink);
		font-size: var(--t-md);
		font-weight: 600;
		padding: 4px 2px;
		max-width: 40vw;
	}
	.psw-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.psw :global(svg:first-child) {
		color: var(--ink-2);
		flex: none;
	}
	.psw :global(svg:last-child) {
		color: var(--ink-3);
		flex: none;
	}

	.sheet-bd {
		position: fixed;
		inset: 0;
		z-index: 60;
		background: oklch(0 0 0 / 0.45);
	}
	.sheet {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 61;
		background: var(--surface);
		border-top: 1px solid var(--line-strong);
		border-radius: var(--r-lg) var(--r-lg) 0 0;
		padding: 8px 10px calc(14px + env(safe-area-inset-bottom, 0px));
		box-shadow: var(--shadow-2);
		animation: slideup 0.18s ease;
	}
	@keyframes slideup {
		from {
			transform: translateY(100%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.sheet {
			animation: none;
		}
	}
	.grip {
		width: 36px;
		height: 4px;
		border-radius: var(--r-sm);
		background: var(--line-strong);
		margin: 4px auto 8px;
	}
	.sheet-h {
		font-size: var(--t-xs);
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-3);
		padding: 4px 12px 6px;
	}
	.scroll {
		max-height: 62vh;
		overflow-y: auto;
	}
	.item {
		display: flex;
		align-items: center;
		gap: 12px;
		width: 100%;
		background: none;
		border: none;
		color: var(--ink-2);
		text-align: left;
		padding: 12px;
		border-radius: var(--r-lg);
		font-size: var(--t-md);
	}
	.item.epic {
		padding-left: 26px;
		font-size: var(--t-base);
	}
	.item.epic :global(svg:first-child) {
		color: var(--accent);
	}
	.item .nm {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.item :global(svg) {
		flex: none;
	}
	.item:active {
		background: var(--hover);
	}
	.item.on {
		color: var(--ink);
	}
	.item.on :global(svg:first-child) {
		color: var(--accent);
	}
	.item.epic.on :global(svg:first-child) {
		color: var(--accent);
	}
	.row {
		display: flex;
		align-items: center;
	}
	.row .item.ini {
		flex: 1;
	}
	.cx {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 30px;
		height: 44px;
		background: none;
		border: none;
		color: var(--ink-3);
		flex: none;
	}
	.cx :global(svg) {
		transition: transform 0.15s ease;
	}
	.cx.open :global(svg) {
		transform: rotate(90deg);
	}
	.cx.empty {
		visibility: hidden;
		pointer-events: none;
	}

	@media (max-width: 720px) {
		.psw {
			display: inline-flex;
		}
	}
</style>
