<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { initiatives, projects, activeProject, loadIssues } from '$lib/store.js';
	import { paletteOpen } from '$lib/ui.js';

	let { onnavigate = () => {} } = $props();

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
		<div class="section-title">Projects</div>
		<button class="nav-item proj" class:active={$activeProject === ''} onclick={() => pick('')}>
			<span class="icon">◇</span>All issues
		</button>
		{#each grouped.groups as g (g.ini.id)}
			<div class="ini">{g.ini.name}</div>
			{#each g.projects as p (p.id)}
				<button class="nav-item proj" class:active={$activeProject === p.id} onclick={() => pick(p.id)}>
					<span class="icon">▸</span>{p.name}
				</button>
			{/each}
		{/each}
		{#each grouped.orphan as p (p.id)}
			<button class="nav-item proj" class:active={$activeProject === p.id} onclick={() => pick(p.id)}>
				<span class="icon">▸</span>{p.name}
			</button>
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
		padding: 7px 10px;
		font-size: 13px;
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
	.ini {
		text-transform: none;
		font-size: 11px;
		color: var(--text-dim);
		padding-top: 8px;
	}
	.nav-item {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 7px 8px;
		border-radius: 6px;
		font-size: 13.5px;
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
