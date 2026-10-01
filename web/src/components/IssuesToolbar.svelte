<script>
	// The controls above every issue view — search, scope, labels and
	// the Board / List / By epic / Links switch — laid out like Paperclip's Tasks bar.
	// The priority filter, saved views and the active-filter chip row are
	// Board-only additions (boardVisibleIssues); the other views keep reading
	// visibleIssues, untouched.
	import { page } from '$app/stores';
	import {
		issueQuery, projects, initiatives, activeProject, activeInitiative, activeLabel, labels,
		loadIssues, PRIORITIES, activePriority, activeSavedView, SAVED_VIEWS, clearBoardFilters
	} from '$lib/store.js';
	import ProjectSwitcher from './ProjectSwitcher.svelte';
	import LabelFilter from './LabelFilter.svelte';
	import { Search, List, Layers, Columns3, GitFork, ListChecks, Star, ChevronDown, X } from '@lucide/svelte';

	// Board · List · By epic · Tasks · Links. List is state-grouped rows,
	// By epic is the Project → Epic → Issue tree, Tasks is grouped by day.
	const views = [
		{ href: '/board', label: 'Board', icon: Columns3 },
		{ href: '/list', label: 'List', icon: List },
		{ href: '/by-epic', label: 'By epic', icon: Layers },
		{ href: '/tasks', label: 'Tasks', icon: ListChecks },
		{ href: '/links', label: 'Links', icon: GitFork }
	];

	const onBoard = $derived($page.url.pathname === '/board');

	let priOpen = $state(false);
	let viewsOpen = $state(false);

	function pickPriority(v) {
		activePriority.set(v);
		priOpen = false;
	}
	function pickSavedView(id) {
		activeSavedView.set(id);
		viewsOpen = false;
	}

	const priLabel = $derived(
		$activePriority === '' ? 'Priority' : (PRIORITIES.find((p) => p.value === Number($activePriority))?.label ?? 'Priority')
	);
	const viewLabel = $derived(SAVED_VIEWS.find((v) => v.id === $activeSavedView)?.label ?? '');

	// Active-filter chips — epic/project scope, label, priority, saved view.
	// Each independently removable. Only on the Board, and only once something
	// is active (filter chip only appears when it means something).
	const epicChip = $derived(
		$activeProject
			? $projects.find((p) => p.id === $activeProject)?.name
			: $activeInitiative
				? $initiatives.find((i) => i.id === $activeInitiative)?.name
				: null
	);
	const labelChip = $derived($labels.find((l) => l.id === $activeLabel)?.name ?? null);
	const hasChips = $derived(onBoard && (epicChip || labelChip || $activePriority !== '' || $activeSavedView));

	function clearEpic() {
		activeProject.set('');
		activeInitiative.set('');
		loadIssues();
	}
	function clearLabel() {
		activeLabel.set('');
		loadIssues();
	}
</script>

<div class="bar">
	<label class="search">
		<Search size={14} strokeWidth={2} />
		<input bind:value={$issueQuery} placeholder="Search tasks…" />
	</label>
	<ProjectSwitcher />
	<LabelFilter />
	{#if onBoard}
		<div class="dd">
			<button class="btn sd" class:on={$activePriority !== ''} onclick={() => (priOpen = !priOpen)}>
				{priLabel}<ChevronDown size={13} strokeWidth={2.4} class="car" />
			</button>
			{#if priOpen}
				<div class="dd-bd" role="presentation" onclick={() => (priOpen = false)}></div>
				<div class="dd-menu">
					<button class="dd-item" class:on={$activePriority === ''} onclick={() => pickPriority('')}>All priorities</button>
					{#each PRIORITIES as p (p.value)}
						<button class="dd-item" class:on={$activePriority === String(p.value)} onclick={() => pickPriority(String(p.value))}>{p.label}</button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
	<div class="spacer"></div>
	{#if onBoard}
		<div class="dd">
			<button class="btn sd" class:on={!!$activeSavedView} onclick={() => (viewsOpen = !viewsOpen)}>
				<Star size={13} strokeWidth={2.2} />{viewLabel || 'Saved views'}<ChevronDown size={13} strokeWidth={2.4} class="car" />
			</button>
			{#if viewsOpen}
				<div class="dd-bd" role="presentation" onclick={() => (viewsOpen = false)}></div>
				<div class="dd-menu">
					<div class="dd-head">Saved views</div>
					<button class="dd-item" class:on={!$activeSavedView} onclick={() => pickSavedView('')}>All issues</button>
					{#each SAVED_VIEWS as v (v.id)}
						<button class="dd-item" class:on={$activeSavedView === v.id} onclick={() => pickSavedView(v.id)}>{v.label}</button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
	<div class="vtoggle" role="tablist">
		{#each views as v (v.label)}
			{@const Icon = v.icon}
			<a href={v.href} class="vt" class:on={$page.url.pathname === v.href} aria-label={v.label}>
				<Icon size={14} strokeWidth={2} /><span class="vt-txt">{v.label}</span>
			</a>
		{/each}
	</div>
</div>

{#if hasChips}
	<div class="chiprow">
		{#if epicChip}
			<button class="fchip" onclick={clearEpic} title="Clear epic filter">{epicChip}<span class="x"><X size={10} strokeWidth={2.6} /></span></button>
		{/if}
		{#if labelChip}
			<button class="fchip" onclick={clearLabel} title="Clear label filter">{labelChip}<span class="x"><X size={10} strokeWidth={2.6} /></span></button>
		{/if}
		{#if $activePriority !== ''}
			<button class="fchip" onclick={() => pickPriority('')} title="Clear priority filter">{priLabel}<span class="x"><X size={10} strokeWidth={2.6} /></span></button>
		{/if}
		{#if $activeSavedView}
			<button class="fchip" onclick={() => pickSavedView('')} title="Clear saved view">{viewLabel}<span class="x"><X size={10} strokeWidth={2.6} /></span></button>
		{/if}
	</div>
{/if}

<style>
	.bar {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 20px;
		flex-wrap: wrap;
		flex-shrink: 0;
	}
	.search {
		display: flex;
		align-items: center;
		gap: 7px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 7px;
		padding: 5px 10px;
		color: var(--ink-3);
		width: min(260px, 100%);
	}
	.search:focus-within {
		border-color: var(--line-strong);
	}
	.search input {
		background: none;
		border: none;
		outline: none;
		font-size: 13px;
		width: 100%;
	}
	.search input::placeholder {
		color: var(--ink-3);
	}
	.spacer {
		flex: 1;
	}
	.btn {
		height: 30px;
		padding: 0 11px;
		border-radius: 8px;
		font: 500 13px/1 var(--font);
		display: inline-flex;
		align-items: center;
		gap: 6px;
		white-space: nowrap;
	}
	.btn.sd {
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--ink-2);
	}
	.btn.sd:hover {
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.btn.sd.on {
		border-color: var(--accent);
		color: var(--accent);
	}
	.btn :global(.car) {
		color: var(--ink-3);
		margin-left: 1px;
	}
	.dd {
		position: relative;
	}
	.dd-bd {
		position: fixed;
		inset: 0;
		z-index: 40;
	}
	.dd-menu {
		position: absolute;
		top: calc(100% + 5px);
		left: 0;
		z-index: 41;
		width: 200px;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: 9px;
		box-shadow: var(--shadow-2);
		padding: 5px;
		display: flex;
		flex-direction: column;
		max-height: 300px;
		overflow-y: auto;
	}
	.dd-head {
		font-size: 11px;
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 6px 8px 7px;
		font-family: var(--mono);
	}
	.dd-item {
		display: flex;
		align-items: center;
		background: none;
		border: none;
		color: var(--ink-2);
		text-align: left;
		padding: 7px 9px;
		border-radius: 6px;
		font-size: 13.5px;
	}
	.dd-item:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.dd-item.on {
		background: var(--accent-soft);
		color: var(--accent);
		font-weight: 500;
	}
	.vtoggle {
		display: flex;
		gap: 2px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 2px;
	}
	.vt {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 12.5px;
		color: var(--ink-2);
	}
	.vt:hover {
		color: var(--ink);
	}
	.vt.on {
		background: var(--hover);
		color: var(--ink);
	}
	.chiprow {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		padding: 0 20px 10px;
	}
	.fchip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 26px;
		padding: 0 6px 0 10px;
		border-radius: 999px;
		background: var(--accent-soft);
		color: var(--accent);
		font-size: 12.5px;
		font-weight: 500;
		border: none;
	}
	.fchip .x {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.fchip .x:hover {
		background: color-mix(in srgb, var(--accent) 18%, transparent);
	}
	@media (max-width: 720px) {
		.bar {
			padding: 8px 14px;
		}
		.vt-txt {
			display: none;
		}
		.search {
			flex: 1 1 100%;
			height: 40px;
		}
		.chiprow {
			padding: 0 14px 8px;
		}
	}
</style>
