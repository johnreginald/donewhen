<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import {
		projects,
		activeProject,
		activeInitiative,
		loadIssues,
		issues,
		inboxCount,
		activeWorkspace,
		me
	} from '$lib/store.js';
	import { paletteOpen, openComposer, askArchive } from '$lib/ui.js';
	import WorkspaceMenu from './WorkspaceMenu.svelte';
	import {
		FileText, Box, Plus, Archive, Search, Pencil, History, Inbox, Columns3, List, Layers, GitFork,
		ChevronsUpDown, Settings, LogOut, ChevronRight, ChevronLeft, OctagonX
	} from '@lucide/svelte';

	// The broad app sections (design's sidebar nav).
	const WORKSPACE_NAV = [
		{ href: '/list', label: 'List', icon: List },
		{ href: '/by-epic', label: 'By epic', icon: Layers },
		{ href: '/blocked', label: 'Blocked', icon: OctagonX },
		{ href: '/links', label: 'Links', icon: GitFork },
		{ href: '/artifacts', label: 'Artifacts', icon: FileText },
		{ href: '/log', label: 'Log', icon: History },
		{ href: '/settings', label: 'Settings', icon: Settings }
	];

	let { onnavigate = () => {} } = $props();

	// Sidebar / rail — collapsed to a 64px icon strip, remembered per device.
	// The rail is a desktop affordance; below 720px this is a slide-in drawer
	// instead (see +layout.svelte), which always shows expanded.
	let collapsed = $state(false);
	let isMobile = $state(false);
	onMount(() => {
		try {
			collapsed = localStorage.getItem('raenil.sidebarCollapsed') === '1';
		} catch {
			/* storage blocked: stay expanded */
		}
		const mq = window.matchMedia('(max-width: 720px)');
		isMobile = mq.matches;
		const onChange = (e) => (isMobile = e.matches);
		mq.addEventListener('change', onChange);
		return () => mq.removeEventListener('change', onChange);
	});
	function toggleCollapsed() {
		collapsed = !collapsed;
		try {
			localStorage.setItem('raenil.sidebarCollapsed', collapsed ? '1' : '0');
		} catch {
			/* not remembered */
		}
	}

	// Workspace switcher — the top-level scope. Everything below it (epics,
	// issues, labels, the inbox badge) belongs to the selected workspace only.
	let wsOpen = $state(false);
	let wsBtn = $state(null);
	let wsPos = $state({ top: 0, left: 0 });
	// The sidebar clips overflow, so the menu is fixed-positioned under the
	// button instead of absolutely inside the sidebar.
	function toggleWs() {
		if (!wsOpen && wsBtn) {
			const r = wsBtn.getBoundingClientRect();
			wsPos = { top: r.bottom + 4, left: r.left };
		}
		wsOpen = !wsOpen;
	}

	// Full issue set (filter-independent) for the per-epic totals in the badge.
	let allIssues = $state([]);
	async function refreshCounts() {
		allIssues = (await api.issues()) || [];
		try {
			const r = await api.inbox();
			inboxCount.set((r?.needsReview || []).length + (r?.waiting || []).length);
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

	// Click an Epic → filter every view to that epic.
	function pickEpic(projectId) {
		activeProject.set(projectId);
		activeInitiative.set('');
		loadIssues();
		if (!onIssues) goto('/board');
		onnavigate();
	}

	// The workspace (top left) already names the project, so the sidebar
	// lists its epics directly, each with how many tasks it holds.
	const epics = $derived(
		$projects.map((p) => ({ ...p, count: allIssues.filter((is) => is.projectId === p.id).length }))
	);

	// Epics fold away; the choice is kept per browser.
	let epicsOpen = $state(true);
	onMount(() => {
		try {
			epicsOpen = localStorage.getItem('raenil.epicsOpen') !== '0';
		} catch {
			/* storage blocked: stay open */
		}
	});
	function toggleEpics() {
		epicsOpen = !epicsOpen;
		try {
			localStorage.setItem('raenil.epicsOpen', epicsOpen ? '1' : '0');
		} catch {
			/* storage blocked: not remembered */
		}
	}

	const ISSUE_VIEWS = ['/tasks', '/list', '/by-epic', '/board', '/links'];
	const onIssues = $derived(ISSUE_VIEWS.includes($page.url.pathname));

	let userOpen = $state(false);
	async function logout() {
		await api.logout();
		goto('/login');
	}
</script>

<nav class="sidebar" class:collapsed={collapsed && !isMobile}>
	<button
		class="railtoggle"
		onclick={toggleCollapsed}
		title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
		aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
	>
		{#if collapsed}<ChevronRight size={12} strokeWidth={2.4} />{:else}<ChevronLeft size={12} strokeWidth={2.4} />{/if}
	</button>

	<div class="ws">
		<button class="ws-btn" bind:this={wsBtn} onclick={toggleWs} title="Switch workspace">
			<span class="logo">{($activeWorkspace?.keyPrefix || 'R').slice(0, 1)}</span>
			<span class="ws-text">
				<span class="ws-name">{$activeWorkspace?.name || 'Raenil'}</span>
				<span class="ws-key">{$activeWorkspace?.keyPrefix || ''}</span>
			</span>
			<ChevronsUpDown size={14} strokeWidth={2} />
		</button>
		{#if wsOpen}
			<div class="menu-backdrop" role="presentation" onclick={() => (wsOpen = false)}></div>
			<div class="ws-menu" style:top="{wsPos.top}px" style:left="{wsPos.left}px">
				<WorkspaceMenu onpick={() => (wsOpen = false)} />
			</div>
		{/if}
	</div>

	<div class="section">
		<button class="nav-item" onclick={() => paletteOpen.set(true)}>
			<span class="icon"><Search size={16} strokeWidth={2} /></span><span class="lbl">Search</span><kbd class="kbd">⌘K</kbd>
		</button>
		<a href="/inbox" class="nav-item" class:active={$page.url.pathname === '/inbox'} onclick={onnavigate}>
			<span class="icon" style="position:relative">
				<Inbox size={16} strokeWidth={2} />
				{#if collapsed && $inboxCount > 0}<span class="dotbadge"></span>{/if}
			</span><span class="lbl">Inbox</span>
			{#if !collapsed && $inboxCount > 0}<span class="badge">{$inboxCount}</span>{/if}
		</a>
	</div>

	<div class="section">
		<div class="section-head"><span class="section-title">Workspace</span></div>
		<a
			href="/board"
			class="nav-item"
			class:active={$page.url.pathname === '/board'}
			onclick={() => (activeInitiative.set(''), activeProject.set(''), loadIssues(), onnavigate())}
		>
			<span class="icon"><Columns3 size={16} strokeWidth={2} /></span><span class="lbl">Board</span>
		</a>
		{#each WORKSPACE_NAV as n (n.label)}
			{@const Icon = n.icon}
			<a
				href={n.href}
				class="nav-item"
				class:active={n.href === '/artifacts' ? $page.url.pathname.startsWith('/artifacts') : $page.url.pathname === n.href}
				onclick={onnavigate}
			>
				<span class="icon"><Icon size={16} strokeWidth={2} /></span><span class="lbl">{n.label}</span>
			</a>
		{/each}
	</div>

	<div class="section epics">
		<div class="section-head">
			<button class="section-toggle" onclick={toggleEpics} aria-expanded={epicsOpen}>
				<span class="chev" class:open={epicsOpen}><ChevronRight size={12} strokeWidth={2.4} /></span>
				<span class="section-title">Epics</span>
			</button>
			<button class="add-btn" title="New epic" onclick={() => openComposer('project')}><Plus size={15} strokeWidth={2.2} /></button>
		</div>
		{#if epicsOpen}
		{#each epics as p (p.id)}
			<div class="epic-row">
				<button class="nav-item epic-sub" class:active={$activeProject === p.id} onclick={() => pickEpic(p.id)}>
					<span class="icon epic-ic"><Box size={13} strokeWidth={2} /></span><span class="pname">{p.name}</span>
					<span class="ini-count">{p.count}</span>
				</button>
				<button class="row-edit" title="Edit epic" onclick={() => editEpic(p)}><Pencil size={13} strokeWidth={2} /></button>
				<button class="row-edit row-archive" title="Archive epic" aria-label="Archive epic" onclick={() => askArchive(p)}><Archive size={13} strokeWidth={2} /></button>
			</div>
		{/each}
		{/if}
	</div>

	<div class="foot">
		<button class="user" onclick={() => (userOpen = !userOpen)}>
			<span class="avatar">{($me?.email || '?').slice(0, 2).toUpperCase()}</span>
			<span class="uname">{$me?.email || ''}</span>
		</button>
		{#if userOpen}
			<div class="menu-backdrop" role="presentation" onclick={() => (userOpen = false)}></div>
			<div class="user-menu">
				<a href="/settings" class="ws-item" onclick={() => ((userOpen = false), onnavigate())}>
					<Settings size={14} strokeWidth={2} />Settings
				</a>
				<button class="ws-item danger" onclick={logout}><LogOut size={14} strokeWidth={2} />Log out</button>
			</div>
		{/if}
	</div>
</nav>

<style>
	.section-toggle {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: none;
		border: none;
		padding: 0;
		color: inherit;
	}
	.section-toggle .chev {
		display: inline-flex;
		color: var(--ink-3);
		transition: transform var(--dur) var(--ease);
	}
	.section-toggle .chev.open {
		transform: rotate(90deg);
	}
	.menu-backdrop {
		position: fixed;
		inset: 0;
		z-index: 30;
	}
	.sidebar {
		width: 240px;
		height: 100%;
		background: var(--sunken);
		border-right: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		padding: 10px 8px;
		gap: 14px;
		overflow-y: auto;
		overflow-x: hidden;
		position: relative;
		transition: width var(--dur) var(--ease);
	}
	.sidebar.collapsed {
		width: 64px;
		align-items: center;
		padding: 10px 6px;
		overflow-y: visible;
	}
	/* Fixed, not absolute: the sidebar clips horizontal overflow, which cut
	   the half of the button that hangs over its edge. Desktop only (hidden
	   on phones), where the sidebar always starts at the left edge. */
	.railtoggle {
		position: fixed;
		top: 18px;
		left: calc(240px - 11px);
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		color: var(--ink-3);
		display: none;
		align-items: center;
		justify-content: center;
		z-index: 6;
		box-shadow: var(--shadow-1);
	}
	.sidebar:hover .railtoggle,
	.railtoggle:focus-visible {
		display: flex;
	}
	.sidebar.collapsed .railtoggle {
		display: flex;
		left: calc(64px - 11px);
	}
	.railtoggle:hover {
		color: var(--ink);
		background: var(--hover);
	}
	.ws {
		position: relative;
		width: 100%;
	}
	.ws-btn {
		display: flex;
		align-items: center;
		gap: 9px;
		width: 100%;
		padding: 5px 8px;
		border: none;
		background: none;
		border-radius: var(--r-sm);
		color: var(--ink-2);
		text-align: left;
	}
	.sidebar.collapsed .ws-btn {
		justify-content: center;
		padding: 5px;
	}
	.ws-btn:hover {
		background: var(--hover);
	}
	.ws-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		line-height: 1.25;
	}
	.sidebar.collapsed .ws-text,
	.sidebar.collapsed .ws-btn :global(svg:last-child) {
		display: none;
	}
	.ws-name {
		font-weight: 600;
		font-size: 13.5px;
		color: var(--ink);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ws-key {
		font-family: var(--mono);
		font-size: 10px;
		color: var(--ink-3);
		letter-spacing: 0.04em;
	}
	.ws-menu {
		position: fixed;
		/* Grow past the sidebar so long workspace names aren't cut. */
		width: max-content;
		min-width: 220px;
		max-width: min(380px, calc(100vw - 24px));
		z-index: 31;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		padding: 5px;
	}
	.ws-item {
		display: flex;
		align-items: center;
		gap: 8px;
		background: none;
		border: none;
		color: var(--ink-2);
		text-align: left;
		padding: 7px 8px;
		border-radius: var(--r-sm);
		font-size: 13.5px;
		width: 100%;
		text-decoration: none;
	}
	.ws-item:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.logo {
		width: 24px;
		height: 24px;
		border-radius: 7px;
		background: var(--ink);
		color: var(--paper);
		font: 600 11px/24px var(--mono);
		text-align: center;
		flex: none;
	}
	kbd {
		font-family: var(--mono);
		font-size: 11px;
		background: var(--surface);
		padding: 1px 5px;
		border-radius: 4px;
	}
	.section {
		display: flex;
		flex-direction: column;
		gap: 1px;
		width: 100%;
	}
	.section-title {
		font-family: var(--mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--ink-3);
		padding: 6px 8px 2px;
	}
	.section-head {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding-right: 4px;
	}
	.sidebar.collapsed .section-head,
	.sidebar.collapsed .epics {
		display: none;
	}
	.add-btn {
		width: 18px;
		height: 18px;
		border-radius: 5px;
		border: none;
		background: none;
		color: var(--ink-3);
		font-size: 13px;
		line-height: 1;
		display: grid;
		place-items: center;
	}
	.add-btn:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.epic-row {
		display: flex;
		align-items: center;
	}
	.epic-row .nav-item {
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
	.epic-sub {
		font-size: 13px;
		color: var(--ink-2);
	}
	.epic-sub .epic-ic {
		color: var(--accent);
		opacity: 0.85;
	}
	.epic-sub.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow-1);
	}
	.ini-count {
		margin-left: auto;
		min-width: 18px;
		text-align: center;
		color: var(--ink-3);
		font-size: var(--t-xs);
		font-variant-numeric: tabular-nums;
	}
	.row-edit {
		opacity: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		color: var(--ink-3);
		padding: 4px 6px;
		border-radius: 5px;
		flex: none;
	}
	.epic-row:hover .row-edit {
		opacity: 1;
	}
	.row-archive {
		margin-left: -4px;
	}
	.row-edit:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.nav-item {
		display: flex;
		align-items: center;
		gap: 9px;
		height: 30px;
		padding: 0 8px;
		border-radius: var(--r-sm);
		font-size: var(--t-base);
		color: var(--ink-2);
		background: none;
		border: none;
		text-align: left;
		width: 100%;
		box-sizing: border-box;
	}
	.nav-item:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.nav-item.active {
		background: var(--surface);
		color: var(--ink);
		font-weight: 500;
		box-shadow: var(--shadow-1);
	}
	.sidebar.collapsed .nav-item {
		justify-content: center;
		padding: 0;
	}
	.sidebar.collapsed .nav-item .lbl,
	.sidebar.collapsed .nav-item .kbd,
	.sidebar.collapsed .nav-item .badge {
		display: none;
	}
	.badge {
		margin-left: auto;
		min-width: 18px;
		text-align: center;
		font-size: 10.5px;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: var(--accent-ink);
		background: var(--accent);
		border-radius: 999px;
		padding: 1px 6px;
		line-height: 1.4;
	}
	.dotbadge {
		position: absolute;
		top: -2px;
		right: -2px;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--accent);
	}
	.icon {
		width: 16px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--ink-2);
		flex: none;
	}
	.nav-item.active .icon {
		color: var(--ink);
	}
	.kbd {
		margin-left: auto;
		font-family: var(--mono);
		font-size: 10.5px;
		color: var(--ink-3);
		border: 1px solid var(--line-strong);
		border-radius: 4px;
		padding: 0 4px;
	}
	.foot {
		margin-top: auto;
		position: relative;
		padding-top: 8px;
		border-top: 1px solid var(--line);
		width: 100%;
	}
	.user {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		background: none;
		border: none;
		color: var(--ink-2);
		padding: 5px 6px;
		border-radius: var(--r-sm);
		text-align: left;
		font-size: 12px;
	}
	.sidebar.collapsed .user {
		justify-content: center;
	}
	.user:hover {
		background: var(--hover);
	}
	.avatar {
		width: 23px;
		height: 23px;
		border-radius: 50%;
		background: var(--accent-soft);
		color: var(--accent);
		font-size: 10px;
		font-weight: 600;
		display: grid;
		place-items: center;
		flex-shrink: 0;
	}
	.uname {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sidebar.collapsed .uname {
		display: none;
	}
	.user-menu {
		position: absolute;
		bottom: 42px;
		left: 0;
		right: 0;
		z-index: 31;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		padding: 4px;
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 180px;
	}
	.ws-item.danger {
		color: var(--danger);
	}
	@media (max-width: 720px) {
		.railtoggle {
			display: none !important;
		}
		/* Switcher menus become bottom sheets on phones, like the palette. */
		.ws-menu,
		.user-menu {
			position: fixed;
			top: auto;
			bottom: 0;
			left: 0;
			right: 0;
			border-radius: var(--r-lg) var(--r-lg) 0 0;
			max-height: 70vh;
			overflow-y: auto;
			padding: 10px 10px calc(14px + env(safe-area-inset-bottom, 0px));
		}
	}
</style>
