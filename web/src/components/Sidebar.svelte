<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import {
		initiatives,
		projects,
		activeProject,
		activeInitiative,
		loadIssues,
		issues,
		inboxCount,
		workspaces,
		activeWorkspace,
		switchWorkspace
	} from '$lib/store.js';
	import { paletteOpen, openComposer } from '$lib/ui.js';
	import { FileText, Box, Layers, Plus, Search, Pencil, History, Inbox, Hexagon, ChevronRight, Check, ChevronsUpDown, Settings } from '@lucide/svelte';

	// Workspace switcher — the top-level scope. Everything below it (epics,
	// issues, labels, the inbox badge) belongs to the selected workspace only.
	let wsOpen = $state(false);
	async function pickWorkspace(slug) {
		wsOpen = false;
		if (slug === $activeWorkspace?.slug) return;
		await switchWorkspace(slug);
		if ($page.url.pathname !== '/' && $page.url.pathname !== '/list') goto('/');
		onnavigate();
	}

	// Collapsible initiative groups — so a big epic list stays manageable.
	let expanded = $state(new Set());
	function toggleExpand(id) {
		const n = new Set(expanded);
		n.has(id) ? n.delete(id) : n.add(id);
		expanded = n;
	}

	// Full issue set (filter-independent) for the per-Project totals in the badge.
	let allIssues = $state([]);
	async function refreshCounts() {
		allIssues = (await api.issues()) || [];
		try {
			const r = await api.inbox();
			inboxCount.set((r?.needsReview || []).length);
		} catch {
			/* not logged in yet / offline — leave badge as-is */
		}
	}
	onMount(refreshCounts);
	// re-pull when issues change (create / move / delete via the board or SSE)
	$effect(() => {
		$issues;
		refreshCounts();
	});

	let { onnavigate = () => {} } = $props();
	let menuOpen = $state(false);
	function choose(kind) {
		menuOpen = false;
		openComposer(kind);
	}

	// Project = Raenil initiative; Epic = Raenil project.
	function editProject(i) {
		openComposer('initiative', {
			id: i.id,
			name: i.name,
			description: i.descriptionMd,
			repoUrl: i.repoUrl
		});
	}
	// Edit / delete an Epic (Raenil "project").
	function editEpic(p) {
		openComposer('project', {
			id: p.id,
			name: p.name,
			description: p.descriptionMd,
			initiativeId: p.initiativeId,
			repoUrl: p.repoUrl
		});
	}

	// Click a Project to filter every view to it (Epics + tickets show grouped in
	// the List/Board). '' = All issues.
	function pick(initiativeId) {
		activeInitiative.set(initiativeId);
		activeProject.set('');
		loadIssues();
		const p = $page.url.pathname;
		if (p !== '/' && p !== '/list') goto('/');
		onnavigate();
	}
	// Click an Epic → filter every view to that epic.
	function pickEpic(projectId) {
		activeProject.set(projectId);
		activeInitiative.set('');
		loadIssues();
		const p = $page.url.pathname;
		if (p !== '/' && p !== '/list') goto('/');
		onnavigate();
	}

	const grouped = $derived(groupProjects($initiatives, $projects, allIssues));
	function groupProjects(inis, projs, iss) {
		const byIni = new Map(inis.map((i) => [i.id, { ini: i, projects: [], count: 0 }]));
		const orphan = [];
		const projToIni = new Map(projs.map((p) => [p.id, p.initiativeId]));
		for (const p of projs) {
			if (p.initiativeId && byIni.has(p.initiativeId)) byIni.get(p.initiativeId).projects.push(p);
			else orphan.push(p);
		}
		// total issues per Project (across all its Epics)
		for (const is of iss) {
			const iniId = is.projectId ? projToIni.get(is.projectId) : null;
			if (iniId && byIni.has(iniId)) byIni.get(iniId).count++;
		}
		return { groups: [...byIni.values()].filter((g) => g.projects.length), orphan };
	}

	const nav = [
		{ label: 'Artifacts', to: '/artifacts', comp: FileText },
		{ label: 'Activities', to: '/log', comp: History }
	];
	const onIssues = $derived($page.url.pathname === '/' || $page.url.pathname === '/list');
</script>

<nav class="sidebar">
	<div class="ws">
		<button class="ws-btn" onclick={() => (wsOpen = !wsOpen)} title="Switch workspace">
			<span class="logo">{($activeWorkspace?.keyPrefix || 'R').slice(0, 1)}</span>
			<span class="ws-text">
				<span class="ws-name">{$activeWorkspace?.name || 'Raenil'}</span>
				<span class="ws-key">{$activeWorkspace?.keyPrefix || ''}</span>
			</span>
			<ChevronsUpDown size={14} strokeWidth={2} />
		</button>
		{#if wsOpen}
			<div class="menu-backdrop" role="presentation" onclick={() => (wsOpen = false)}></div>
			<div class="ws-menu">
				{#each $workspaces as w (w.id)}
					<button class="ws-item" class:on={w.id === $activeWorkspace?.id} onclick={() => pickWorkspace(w.slug)}>
						<span class="ws-item-key">{w.keyPrefix}</span>
						<span class="ws-item-name">{w.name}</span>
						{#if w.id === $activeWorkspace?.id}<Check size={14} strokeWidth={2.4} />{/if}
					</button>
				{/each}
				<a class="ws-item manage" href="/settings" onclick={() => (wsOpen = false)}>
					<Settings size={13} strokeWidth={2} />Manage workspaces
				</a>
			</div>
		{/if}
	</div>

	<button class="cmdk" onclick={() => paletteOpen.set(true)}>
		<span class="cmdk-l"><Search size={14} strokeWidth={2} /> Search…</span>
		<kbd>⌘K</kbd>
	</button>

	<div class="section">
		<a
			href="/inbox"
			class="nav-item"
			class:active={$page.url.pathname === '/inbox'}
			onclick={onnavigate}
		>
			<span class="icon"><Inbox size={16} strokeWidth={2} /></span>Inbox
			{#if $inboxCount > 0}<span class="badge">{$inboxCount}</span>{/if}
		</a>
		<button class="nav-item" class:active={onIssues && !$activeInitiative && !$activeProject} onclick={() => pick('')}>
			<span class="icon"><Layers size={16} strokeWidth={2} /></span>All Issues
		</button>
		{#each nav as n}
			{@const Icon = n.comp}
			<a
				href={n.to}
				class="nav-item"
				class:active={$page.url.pathname === n.to}
				onclick={onnavigate}
			>
				<span class="icon"><Icon size={16} strokeWidth={2} /></span>{n.label}
			</a>
		{/each}
	</div>

	<div class="section">
		<div class="section-head">
			<span class="section-title">Projects</span>
			<button class="add-btn" title="Create project or epic" onclick={() => (menuOpen = !menuOpen)}><Plus size={15} strokeWidth={2.2} /></button>
			{#if menuOpen}
				<div class="menu-backdrop" role="presentation" onclick={() => (menuOpen = false)}></div>
				<div class="add-menu">
					<button onclick={() => choose('initiative')}><Box size={14} strokeWidth={2} />New project</button>
					<button onclick={() => choose('project')}><Layers size={14} strokeWidth={2} />New epic</button>
				</div>
			{/if}
		</div>
		{#each grouped.groups as g (g.ini.id)}
			{@const isOpen = expanded.has(g.ini.id) || g.projects.some((p) => p.id === $activeProject)}
			<div class="proj-row">
				<button
					class="caret"
					class:open={isOpen}
					class:empty={g.projects.length === 0}
					onclick={() => toggleExpand(g.ini.id)}
					aria-label="Expand epics"
				>
					<ChevronRight size={17} strokeWidth={2.5} />
				</button>
				<button class="nav-item proj" class:active={$activeInitiative === g.ini.id} onclick={() => pick(g.ini.id)}>
					<span class="icon"><Hexagon size={14} strokeWidth={2} /></span><span class="pname">{g.ini.name}</span>
					<span class="ini-count">{g.count}</span>
				</button>
				<button class="row-edit" title="Edit project" onclick={() => editProject(g.ini)}><Pencil size={13} strokeWidth={2} /></button>
			</div>
			{#if isOpen}
				{#each g.projects as p (p.id)}
					<div class="epic-row">
						<button class="nav-item epic-sub" class:active={$activeProject === p.id} onclick={() => pickEpic(p.id)}>
							<span class="icon epic-ic"><Box size={12} strokeWidth={2} /></span><span class="pname">{p.name}</span>
						</button>
						<button class="row-edit" title="Edit epic" onclick={() => editEpic(p)}><Pencil size={13} strokeWidth={2} /></button>
					</div>
				{/each}
			{/if}
		{/each}
	</div>
</nav>

<style>
	.sidebar {
		width: 240px;
		height: 100%;
		background: var(--bg-elev);
		border-right: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		padding: 12px 10px;
		gap: 14px;
		overflow-y: auto;
	}
	.ws {
		position: relative;
	}
	.ws-btn {
		display: flex;
		align-items: center;
		gap: 9px;
		width: 100%;
		padding: 5px 8px;
		border: none;
		background: none;
		border-radius: 8px;
		color: var(--text-dim);
		text-align: left;
	}
	.ws-btn:hover {
		background: var(--bg-hover);
	}
	.ws-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		line-height: 1.25;
	}
	.ws-name {
		font-weight: 600;
		font-size: 14px;
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-key {
		font-family: var(--mono);
		font-size: 10.5px;
		color: var(--text-faint);
		letter-spacing: 0.04em;
	}
	.ws-menu {
		position: absolute;
		top: 42px;
		left: 4px;
		right: 4px;
		z-index: 31;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 9px;
		box-shadow: var(--shadow);
		padding: 4px;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.ws-item {
		display: flex;
		align-items: center;
		gap: 8px;
		background: none;
		border: none;
		color: var(--text-dim);
		text-align: left;
		padding: 7px 8px;
		border-radius: 6px;
		font-size: 13.5px;
		width: 100%;
		text-decoration: none;
	}
	.ws-item:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.ws-item.on {
		color: var(--text);
	}
	.ws-item-key {
		font-family: var(--mono);
		font-size: 10px;
		color: var(--text-faint);
		background: var(--bg);
		border-radius: 4px;
		padding: 2px 5px;
		flex: none;
		min-width: 34px;
		text-align: center;
	}
	.ws-item-name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-item.manage {
		border-top: 1px solid var(--border);
		margin-top: 3px;
		padding-top: 8px;
		font-size: 12.5px;
		color: var(--text-faint);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 4px 8px;
	}
	.logo {
		width: 26px;
		height: 26px;
		border-radius: 8px;
		background: var(--accent-grad);
		color: white;
		display: grid;
		place-items: center;
		font-family: var(--disp);
		font-weight: 800;
		box-shadow: 0 4px 14px var(--accent-soft);
	}
	.name {
		font-weight: 600;
		font-size: 15px;
	}
	.cmdk {
		display: flex;
		justify-content: space-between;
		align-items: center;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text-dim);
		padding: 8px 11px;
		font-size: 14px;
	}
	.cmdk-l {
		display: inline-flex;
		align-items: center;
		gap: 7px;
	}
	kbd {
		font-family: var(--mono);
		font-size: 11px;
		background: var(--bg-elev2);
		padding: 1px 5px;
		border-radius: 4px;
	}
	.section {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.section-title,
	.ini {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-faint);
		padding: 6px 8px 2px;
	}
	.section-head {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding-right: 4px;
	}
	.add-btn {
		width: 20px;
		height: 20px;
		border-radius: 5px;
		border: none;
		background: none;
		color: var(--text-faint);
		font-size: 15px;
		line-height: 1;
		display: grid;
		place-items: center;
	}
	.add-btn:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.menu-backdrop {
		position: fixed;
		inset: 0;
		z-index: 30;
	}
	.add-menu {
		position: absolute;
		top: 24px;
		right: 4px;
		z-index: 31;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 8px;
		box-shadow: var(--shadow);
		padding: 4px;
		min-width: 150px;
		display: flex;
		flex-direction: column;
	}
	.add-menu button {
		display: flex;
		align-items: center;
		gap: 8px;
		background: none;
		border: none;
		color: var(--text);
		text-align: left;
		padding: 7px 9px;
		border-radius: 6px;
		font-size: 13px;
	}
	.add-menu button:hover {
		background: var(--bg-hover);
	}
	.add-menu .mi {
		color: var(--text-faint);
		font-size: 11px;
		width: 12px;
		text-align: center;
	}
	.ini {
		text-transform: none;
		font-size: 11px;
		color: var(--text-dim);
		padding-top: 8px;
	}
	.ini-head {
		display: flex;
		align-items: center;
		padding-right: 4px;
	}
	.ini-toggle {
		display: flex;
		align-items: center;
		gap: 6px;
		flex: 1;
		min-width: 0;
		background: none;
		border: none;
		text-align: left;
		color: var(--text-dim);
		font-size: 11.5px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		padding: 9px 8px 4px;
	}
	.ini-toggle:hover {
		color: var(--text);
	}
	.chev {
		font-size: 9px;
		color: var(--text-faint);
		transition: transform 0.15s ease;
		flex: none;
	}
	.chev.open {
		transform: rotate(90deg);
	}
	.ini-name {
		font-weight: 500;
		letter-spacing: 0.01em;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ini-count {
		margin-left: auto;
		min-width: 18px;
		text-align: center;
		color: var(--text-dim);
		font-size: 11px;
		font-variant-numeric: tabular-nums;
		background: var(--bg-elev2);
		border-radius: 9px;
		padding: 1px 6px;
	}
	.ini-head:hover .ini-count {
		color: var(--text);
	}
	.epic-row,
	.proj-row {
		display: flex;
		align-items: center;
	}
	.epic-row .nav-item,
	.proj-row .nav-item {
		flex: 1;
		min-width: 0;
	}
	.pname {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.caret {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 22px;
		height: 30px;
		background: none;
		border: none;
		color: var(--text-dim);
		cursor: pointer;
		flex: none;
	}
	.caret:hover {
		color: var(--text);
	}
	.caret :global(svg) {
		transition: transform 0.15s ease;
	}
	.caret.open :global(svg) {
		transform: rotate(90deg);
	}
	.caret.empty {
		visibility: hidden;
		pointer-events: none;
	}
	.epic-sub {
		padding-left: 34px;
		font-size: 13px;
		color: var(--text-faint);
	}
	.epic-sub .epic-ic {
		color: var(--accent2);
		opacity: 0.85;
	}
	.epic-sub:hover {
		color: var(--text-dim);
	}
	.epic-sub.active {
		background: var(--bg-hover);
		color: var(--text);
	}
	.row-edit {
		opacity: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		color: var(--text-faint);
		padding: 4px 6px;
		border-radius: 5px;
		flex: none;
	}
	.ini-head:hover .row-edit,
	.epic-row:hover .row-edit,
	.proj-row:hover .row-edit {
		opacity: 1;
	}
	.row-edit:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.nav-item {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 7px 8px;
		border-radius: 6px;
		font-size: 14.5px;
		color: var(--text-dim);
		background: none;
		border: none;
		text-align: left;
		width: 100%;
	}
	.nav-item:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.nav-item.active {
		background: var(--bg-hover);
		color: var(--text);
	}
	.badge {
		margin-left: auto;
		min-width: 18px;
		text-align: center;
		font-size: 11px;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: #fff;
		background: var(--accent);
		border-radius: 9px;
		padding: 1px 6px;
		line-height: 1.4;
	}
	.icon {
		width: 16px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--text-dim);
		flex: none;
	}
	.proj .icon {
		color: var(--text-faint);
	}
	.nav-item.active .icon {
		color: var(--text);
	}
</style>
