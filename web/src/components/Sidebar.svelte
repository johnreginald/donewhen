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
		workspaces,
		activeWorkspace,
		switchWorkspace
	} from '$lib/store.js';
	import { paletteOpen, openComposer } from '$lib/ui.js';
	import { me } from '$lib/store.js';
	import {
		FileText, Box, Plus, Search, Pencil, History, Inbox, Check,
		ChevronsUpDown, Settings, CircleCheckBig, LogOut, ChevronRight } from '@lucide/svelte';

	// Workspace switcher — the top-level scope. Everything below it (epics,
	// issues, labels, the inbox badge) belongs to the selected workspace only.
	let wsOpen = $state(false);
	async function pickWorkspace(slug) {
		wsOpen = false;
		if (slug === $activeWorkspace?.slug) return;
		await switchWorkspace(slug);
		if (!onIssues) goto('/board');
		onnavigate();
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

	let { onnavigate = () => {} } = $props();
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

	const ISSUE_VIEWS = ['/tasks', '/list', '/board', '/links'];
	const onIssues = $derived(ISSUE_VIEWS.includes($page.url.pathname));

	let userOpen = $state(false);
	async function logout() {
		await api.logout();
		goto('/login');
	}
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

	<div class="section">
		<button class="nav-item" onclick={() => paletteOpen.set(true)}>
			<span class="icon"><Search size={16} strokeWidth={2} /></span>Search<kbd class="kbd">⌘K</kbd>
		</button>
		<a href="/inbox" class="nav-item" class:active={$page.url.pathname === '/inbox'} onclick={onnavigate}>
			<span class="icon"><Inbox size={16} strokeWidth={2} /></span>Inbox
			{#if $inboxCount > 0}<span class="badge">{$inboxCount}</span>{/if}
		</a>
	</div>

	<div class="section">
		<div class="section-head"><span class="section-title">Workspace</span></div>
		<a
			href="/board"
			class="nav-item"
			class:active={onIssues && !$activeInitiative && !$activeProject}
			onclick={() => (activeInitiative.set(''), activeProject.set(''), loadIssues(), onnavigate())}
		>
			<span class="icon"><CircleCheckBig size={16} strokeWidth={2} /></span>Tasks
		</a>
		<a href="/artifacts" class="nav-item" class:active={$page.url.pathname.startsWith('/artifacts')} onclick={onnavigate}>
			<span class="icon"><FileText size={16} strokeWidth={2} /></span>Artifacts
		</a>
		<a href="/log" class="nav-item" class:active={$page.url.pathname === '/log'} onclick={onnavigate}>
			<span class="icon"><History size={16} strokeWidth={2} /></span>Log
		</a>
	</div>

	<div class="section">
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
		color: var(--text-faint);
		transition: transform 0.15s;
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
		color: var(--text-dim);
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
	.epic-row:hover .row-edit {
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
	.nav-item.active .icon {
		color: var(--text);
	}
	.kbd {
		margin-left: auto;
		font-family: var(--mono);
		font-size: 10.5px;
		color: var(--text-faint);
		border: 1px solid var(--border-strong);
		border-radius: 4px;
		padding: 0 4px;
	}
	.foot {
		margin-top: auto;
		position: relative;
		padding-top: 8px;
		border-top: 1px solid var(--border);
	}
	.user {
		display: flex;
		align-items: center;
		gap: 9px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 5px 6px;
		border-radius: 8px;
		text-align: left;
		font-size: 13px;
	}
	.user:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.avatar {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		background: var(--accent-grad);
		color: #fff;
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
	.user-menu {
		position: absolute;
		bottom: 42px;
		left: 0;
		right: 0;
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
	.ws-item.danger {
		color: #f87171;
	}
</style>
