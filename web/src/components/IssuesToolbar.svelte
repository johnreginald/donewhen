<script>
	// The controls above every issue view: search, scope, the State / Priority /
	// Label filters, Save view, and the Board / List / By epic / Links switch.
	// The filters live in the page URL, so a copied link opens the same list,
	// and "Save view" keeps one under Views in the sidebar (PP-205).
	import { onMount, untrack } from 'svelte';
	import { get } from 'svelte/store';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { autohide } from '$lib/autohide.js';
	import {
		issueQuery, projects, initiatives, activeProject, activeInitiative, activeFilters, savedViews,
		states, labels, loadIssues, PRIORITIES, filterQuery, applyFilterQuery
	} from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { hasFilterParams, parseFilters, serializeFilters, emptyFilters, toggle } from '$lib/filters.js';
	import { showToast } from '$lib/ui.js';
	import ProjectSwitcher from './ProjectSwitcher.svelte';
	import MultiFilter from './MultiFilter.svelte';
	import { Search, List, Layers, Columns3, GitFork, ListChecks, Bookmark, X } from '@lucide/svelte';

	// Board · List · By epic · Tasks · Links. List is state-grouped rows,
	// By epic is the Project → Epic → Issue tree, Tasks is grouped by day.
	const views = [
		{ href: '/board', label: 'Board', icon: Columns3 },
		{ href: '/list', label: 'List', icon: List },
		{ href: '/by-epic', label: 'By epic', icon: Layers },
		{ href: '/tasks', label: 'Tasks', icon: ListChecks },
		{ href: '/links', label: 'Links', icon: GitFork }
	];

	const path = $derived($page.url.pathname);
	// The query the filters amount to now; reading the stores keeps it current.
	const query = $derived.by(() => {
		$activeFilters;
		$activeProject;
		$issueQuery;
		return filterQuery();
	});
	const withQuery = (href) => (query ? `${href}?${query}` : href);

	// A URL that names filters sets them (a shared link, a saved view, a reload);
	// otherwise the filters already in effect stay as they are.
	$effect(() => {
		const search = $page.url.search;
		if (hasFilterParams(search)) untrack(() => applyFilterQuery(search));
	});
	// And the URL follows the filters, replacing the entry so Back is not flooded.
	let urlTimer;
	$effect(() => {
		const want = query;
		const search = untrack(() => $page.url.search);
		const have = hasFilterParams(search) ? serializeFilters(parseFilters(search)) : '';
		if (have === want) return;
		clearTimeout(urlTimer);
		urlTimer = setTimeout(() => {
			// Re-read: another navigation may have landed in the meantime.
			if (filterQuery() === want) goto(withQuery(untrack(() => $page.url.pathname)), { replaceState: true, keepFocus: true, noScroll: true });
		}, 150);
	});
	onMount(() => () => clearTimeout(urlTimer));

	const stateOptions = $derived($states.map((s) => ({ value: s.name, name: s.name, color: s.color })));
	const priorityOptions = PRIORITIES.map((p) => ({ value: p.value, name: p.label }));
	const labelOptions = $derived($labels.map((l) => ({ value: l.id, name: l.name, color: l.color })));

	function setFilter(key, values) {
		activeFilters.update((f) => ({ ...f, [key]: values }));
	}

	// Chips for everything in effect, each removable on its own.
	const chips = $derived([
		...$activeFilters.states.map((v) => ({ id: 's' + v, text: v, clear: () => setFilter('states', toggle($activeFilters.states, v)) })),
		...$activeFilters.priorities.map((v) => ({
			id: 'p' + v,
			text: PRIORITIES.find((p) => p.value === v)?.label ?? String(v),
			clear: () => setFilter('priorities', toggle($activeFilters.priorities, v))
		})),
		...$activeFilters.labels.map((v) => ({
			id: 'l' + v,
			text: $labels.find((l) => l.id === v)?.name ?? 'Label',
			clear: () => setFilter('labels', toggle($activeFilters.labels, v))
		})),
		...($activeProject
			? [{ id: 'epic', text: $projects.find((p) => p.id === $activeProject)?.name ?? 'Epic', clear: clearEpic }]
			: $activeInitiative
				? [{ id: 'ini', text: $initiatives.find((i) => i.id === $activeInitiative)?.name ?? 'Project', clear: clearEpic }]
				: [])
	]);
	function clearEpic() {
		activeProject.set('');
		activeInitiative.set('');
		loadIssues();
	}
	function clearAll() {
		activeFilters.set(emptyFilters());
		issueQuery.set('');
		if (get(activeProject) || get(activeInitiative)) clearEpic();
	}

	// Save view: name the filters in effect and keep them in the sidebar.
	let saving = $state(false);
	let viewName = $state('');
	let viewError = $state('');
	let nameEl = $state(null);
	const canSave = $derived(!!query);
	function openSave() {
		saving = true;
		viewName = '';
		viewError = '';
		queueMicrotask(() => nameEl?.focus());
	}
	async function saveView() {
		const name = viewName.trim();
		if (!name || name.length > 60) {
			viewError = name ? 'Keep the name under 60 characters.' : 'Give the view a name.';
			return;
		}
		try {
			const v = await api.post('/views', { name, query: filterQuery(), layout: path === '/board' ? 'board' : 'list' });
			savedViews.update((l) => [...l, v]);
			saving = false;
			showToast(`View "${v.name}" saved`);
		} catch (e) {
			viewError = e.message || 'Could not save the view';
		}
	}
	function saveKey(e) {
		if (e.key === 'Enter') {
			e.preventDefault();
			saveView();
		} else if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			saving = false;
		}
	}
</script>

<div class="bar" use:autohide>
	<label class="search">
		<Search size={14} strokeWidth={2} />
		<input bind:value={$issueQuery} placeholder="Search tasks…" />
	</label>
	<ProjectSwitcher />
	<MultiFilter label="State" options={stateOptions} selected={$activeFilters.states} onchange={(v) => setFilter('states', v)} />
	<MultiFilter label="Priority" options={priorityOptions} selected={$activeFilters.priorities} onchange={(v) => setFilter('priorities', v)} />
	<MultiFilter label="Label" options={labelOptions} selected={$activeFilters.labels} onchange={(v) => setFilter('labels', v)} searchable />
	<div class="spacer"></div>
	{#if canSave}
		<div class="dd">
			<button class="btn sd" onclick={() => (saving ? (saving = false) : openSave())} aria-haspopup="true" aria-expanded={saving}>
				<Bookmark size={13} strokeWidth={2.2} />Save view
			</button>
			{#if saving}
				<div class="dd-bd" role="presentation" onclick={() => (saving = false)}></div>
				<div class="dd-menu save">
					<input
						bind:this={nameEl}
						bind:value={viewName}
						oninput={() => (viewError = '')}
						onkeydown={saveKey}
						class="input"
						placeholder="View name"
						aria-label="View name"
						maxlength="60"
					/>
					{#if viewError}<div class="verr" role="alert">{viewError}</div>{/if}
					<button class="btn primary" onclick={saveView}>Save</button>
				</div>
			{/if}
		</div>
	{/if}
	<div class="vtoggle" role="tablist">
		{#each views as v (v.label)}
			{@const Icon = v.icon}
			<a href={withQuery(v.href)} class="vt" class:on={path === v.href} aria-label={v.label}>
				<Icon size={14} strokeWidth={2} /><span class="vt-txt">{v.label}</span>
			</a>
		{/each}
	</div>
</div>

{#if chips.length}
	<div class="chiprow">
		{#each chips as c (c.id)}
			<button class="fchip" onclick={c.clear} title="Clear this filter">{c.text}<span class="x"><X size={10} strokeWidth={2.6} /></span></button>
		{/each}
		{#if chips.length > 1}<button class="clearall" onclick={clearAll}>Clear all</button>{/if}
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
		border: 1px solid var(--line-strong);
		border-radius: var(--r);
		padding: 5px 10px;
		color: var(--ink-3);
		width: min(260px, 100%);
		transition:
			border-color 0.12s,
			box-shadow 0.12s;
	}
	/* the wrapper is the field: it shares the .input border and focus ring */
	.search:focus-within {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--focus);
	}
	.search input {
		background: none;
		border: none;
		outline: none;
		font-size: var(--t-base);
		width: 100%;
		color: var(--ink);
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
		border-radius: var(--r);
		font: 500 var(--t-sm)/1 var(--font);
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
		border-radius: var(--r);
		box-shadow: var(--shadow-2);
		padding: 5px;
		display: flex;
		flex-direction: column;
		max-height: 300px;
		overflow-y: auto;
	}
	.dd-menu.save {
		width: 240px;
		gap: 8px;
		padding: 8px;
	}
	.verr {
		font-size: var(--t-sm);
		color: var(--danger);
	}
	.clearall {
		background: none;
		border: none;
		color: var(--ink-3);
		font-size: var(--t-sm);
		padding: 0 6px;
	}
	.clearall:hover {
		color: var(--ink);
	}
	.vtoggle {
		display: flex;
		gap: 2px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 2px;
	}
	.vt {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 4px 10px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
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
		font-size: var(--t-sm);
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
