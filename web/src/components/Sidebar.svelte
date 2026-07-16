<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { initiatives, projects, activeProject, loadIssues } from '$lib/store.js';
	import { paletteOpen, openComposer } from '$lib/ui.js';

	let { onnavigate = () => {} } = $props();
	let menuOpen = $state(false);
	function choose(kind) {
		menuOpen = false;
		openComposer(kind);
	}

	// Expanded Project (initiative) groups, persisted. Default: all collapsed —
	// the sidebar shows only Project names; click one to reveal its Epics.
	function loadExpanded() {
		try {
			return new Set(JSON.parse(localStorage.getItem('raenil.expanded') || '[]'));
		} catch {
			return new Set();
		}
	}
	let expanded = $state(loadExpanded());
	function toggle(id) {
		const n = new Set(expanded);
		n.has(id) ? n.delete(id) : n.add(id);
		expanded = n;
		try {
			localStorage.setItem('raenil.expanded', JSON.stringify([...n]));
		} catch {
			/* ignore */
		}
	}
	// Project = Raenil initiative; Epic = Raenil project.
	function editProject(i) {
		openComposer('initiative', { id: i.id, name: i.name, description: i.descriptionMd });
	}
	function editEpic(p) {
		openComposer('project', {
			id: p.id,
			name: p.name,
			description: p.descriptionMd,
			initiativeId: p.initiativeId
		});
	}

	function pick(id) {
		activeProject.set(id);
		loadIssues();
		if ($page.url.pathname !== '/') goto('/');
		onnavigate();
	}

	const grouped = $derived(groupProjects($initiatives, $projects));
	function groupProjects(inis, projs) {
		const byIni = new Map(inis.map((i) => [i.id, { ini: i, projects: [] }]));
		const orphan = [];
		for (const p of projs) {
			if (p.initiativeId && byIni.has(p.initiativeId)) byIni.get(p.initiativeId).projects.push(p);
			else orphan.push(p);
		}
		return { groups: [...byIni.values()].filter((g) => g.projects.length), orphan };
	}

	const nav = [
		{ label: 'Board', to: '/', icon: '▦' },
		{ label: 'List', to: '/list', icon: '☰' },
		{ label: 'Documents', to: '/docs', icon: '📄' },
		{ label: 'Settings', to: '/settings', icon: '⚙' }
	];
</script>

<nav class="sidebar">
	<div class="brand">
		<span class="logo">R</span>
		<span class="name">Raenil</span>
	</div>

	<button class="cmdk" onclick={() => paletteOpen.set(true)}>
		<span>Search…</span>
		<kbd>⌘K</kbd>
	</button>

	<div class="section">
		{#each nav as n}
			<a
				href={n.to}
				class="nav-item"
				class:active={$page.url.pathname === n.to}
				onclick={onnavigate}
			>
				<span class="icon">{n.icon}</span>{n.label}
			</a>
		{/each}
	</div>

	<div class="section">
		<div class="section-head">
			<span class="section-title">Projects</span>
			<button class="add-btn" title="Create project or epic" onclick={() => (menuOpen = !menuOpen)}>+</button>
			{#if menuOpen}
				<div class="menu-backdrop" role="presentation" onclick={() => (menuOpen = false)}></div>
				<div class="add-menu">
					<button onclick={() => choose('initiative')}><span class="mi">◇</span>New project</button>
					<button onclick={() => choose('project')}><span class="mi">▸</span>New epic</button>
				</div>
			{/if}
		</div>
		<button class="nav-item proj" class:active={$activeProject === ''} onclick={() => pick('')}>
			<span class="icon">◇</span>All issues
		</button>
		{#each grouped.groups as g (g.ini.id)}
			<div class="ini-head">
				<button class="ini-toggle" onclick={() => toggle(g.ini.id)}>
					<span class="chev" class:open={expanded.has(g.ini.id)}>▸</span>
					<span class="ini-name">{g.ini.name}</span>
					<span class="ini-count">{g.projects.length}</span>
				</button>
				<button class="row-edit" title="Edit project" onclick={() => editProject(g.ini)}>✎</button>
			</div>
			{#if expanded.has(g.ini.id)}
				{#each g.projects as p (p.id)}
					<div class="epic-row">
						<button class="nav-item proj" class:active={$activeProject === p.id} onclick={() => pick(p.id)}>
							<span class="icon">▸</span>{p.name}
						</button>
						<button class="row-edit" title="Edit epic" onclick={() => editEpic(p)}>✎</button>
					</div>
				{/each}
			{/if}
		{/each}
		{#each grouped.orphan as p (p.id)}
			<div class="epic-row">
				<button class="nav-item proj" class:active={$activeProject === p.id} onclick={() => pick(p.id)}>
					<span class="icon">▸</span>{p.name}
				</button>
				<button class="row-edit" title="Edit epic" onclick={() => editEpic(p)}>✎</button>
			</div>
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
	.epic-row {
		display: flex;
		align-items: center;
	}
	.epic-row .nav-item {
		flex: 1;
		min-width: 0;
	}
	.row-edit {
		opacity: 0;
		background: none;
		border: none;
		color: var(--text-faint);
		font-size: 12px;
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
	.icon {
		width: 16px;
		text-align: center;
		opacity: 0.8;
	}
	.proj .icon {
		font-size: 10px;
	}
</style>
