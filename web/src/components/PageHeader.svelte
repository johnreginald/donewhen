<script>
	// The bar at the top of every page: Workspace › Project › <page>, where the
	// first two crumbs open their switcher menus (PP-209). Callers only supply
	// the page's own name — the workspace and project ahead of it are the same
	// on every page, so PageHeader owns them once instead of every route
	// re-deriving them.
	import { page } from '$app/stores';
	import { navOpen, connectionLost, quickCapture } from '$lib/ui.js';
	import { activeWorkspace, activeInitiative, initiatives, activeProject, loadIssues } from '$lib/store.js';
	import WorkspaceMenu from './WorkspaceMenu.svelte';
	import { Menu, ChevronRight, ChevronDown, Check, Plus } from '@lucide/svelte';

	// crumb: the page's own name ("Board", "Inbox", …), or — for a page that
	// needs more than one trailing segment (the issue detail page's epic ›
	// key) — an array of strings / { label, upper } objects. upper defaults
	// true (small caps, like a section label); pass false for an actual name
	// or key that should read in its own case.
	// Pages still pass the legacy `crumbs` array ([{label, href?, upper?}], first
	// entry a link back to the section). Workspace and Project now lead the
	// path, so a leading link crumb is dropped; /tasks is labelled "Tasks".
	let { crumb = '', crumbs = null, children } = $props();
	const tail = $derived.by(() => {
		if (crumbs) {
			const list = crumbs.filter((c, i) => !(i === 0 && c.href && crumbs.length > 1));
			return list.map((c) => (c.label === 'List' && $page.url.pathname === '/tasks' ? { ...c, label: 'Tasks' } : c));
		}
		if (crumb === '') return [];
		return Array.isArray(crumb) ? crumb.map((c) => (typeof c === 'string' ? { label: c } : c)) : [{ label: crumb }];
	});

	let wsOpen = $state(false);
	let projOpen = $state(false);

	const projLabel = $derived(
		$activeInitiative ? ($initiatives.find((i) => i.id === $activeInitiative)?.name ?? 'Project') : 'All projects'
	);

	function pickAllProjects() {
		activeInitiative.set('');
		activeProject.set('');
		loadIssues();
		projOpen = false;
	}
	function pickInitiative(id) {
		activeInitiative.set(id);
		activeProject.set('');
		loadIssues();
		projOpen = false;
	}
</script>

<header class="ph">
	<div class="phrow">
		<button class="menu" onclick={() => navOpen.set(true)} aria-label="Open navigation">
			<Menu size={18} strokeWidth={2} />
		</button>
		<nav class="crumbs" aria-label="Breadcrumb">
			<div class="crumbwrap">
				<button
					class="crumbbtn"
					onclick={() => (wsOpen = !wsOpen)}
					aria-haspopup="true"
					aria-expanded={wsOpen}
				>
					{$activeWorkspace?.name || 'Raenil'}<ChevronDown size={10} strokeWidth={2.6} class="cchev" />
				</button>
				{#if wsOpen}
					<div class="dd-bd" role="presentation" onclick={() => (wsOpen = false)}></div>
					<div class="ddmenu"><WorkspaceMenu onpick={() => (wsOpen = false)} /></div>
				{/if}
			</div>
			<ChevronRight size={12} strokeWidth={2} class="sep" />
			<div class="crumbwrap">
				<button
					class="crumbbtn"
					onclick={() => (projOpen = !projOpen)}
					aria-haspopup="true"
					aria-expanded={projOpen}
				>
					{projLabel}<ChevronDown size={10} strokeWidth={2.6} class="cchev" />
				</button>
				{#if projOpen}
					<div class="dd-bd" role="presentation" onclick={() => (projOpen = false)}></div>
					<div class="ddmenu">
						<div class="ddsectitle">Projects in {$activeWorkspace?.name || 'this workspace'}</div>
						<button class="ddi" class:on={!$activeInitiative} onclick={pickAllProjects}>
							<span class="nm">All projects</span>
							{#if !$activeInitiative}<Check size={14} strokeWidth={2.4} class="ck" />{/if}
						</button>
						{#each $initiatives as i (i.id)}
							<button class="ddi" class:on={$activeInitiative === i.id} onclick={() => pickInitiative(i.id)}>
								<span class="nm">{i.name}</span>
								{#if $activeInitiative === i.id}<Check size={14} strokeWidth={2.4} class="ck" />{/if}
							</button>
						{/each}
					</div>
				{/if}
			</div>
			{#each tail as c, i (i)}
				<ChevronRight size={12} strokeWidth={2} class="sep" />
				<span class="crumb" class:here={i === tail.length - 1} class:upper={c.upper !== false}>{c.label}</span>
			{/each}
		</nav>
		<div class="right">
			{@render children?.()}
			<button class="btn primary new-issue" onclick={() => quickCapture.set(true)} aria-label="New issue">
				<Plus size={14} strokeWidth={2.4} class="plus-ic" />
				<span class="ni-label">New issue</span><kbd class="kbd">C</kbd>
			</button>
		</div>
	</div>
	{#if $connectionLost}
		<div class="connban" role="status">
			<span class="cbdot"></span>Live updates paused — reconnecting…
		</div>
	{/if}
</header>

<style>
	.ph {
		display: flex;
		flex-direction: column;
		background: var(--paper);
		flex-shrink: 0;
	}
	.phrow {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 48px;
		padding: 8px 20px;
		padding-top: calc(8px + env(safe-area-inset-top, 0px));
		border-bottom: 1px solid var(--line);
	}
	.menu {
		display: none;
		background: none;
		border: none;
		color: var(--ink-2);
		padding: 4px;
		margin-left: -6px;
		border-radius: 6px;
	}
	.crumbs {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
		color: var(--ink-3);
	}
	.crumbs :global(.sep) {
		flex-shrink: 0;
		color: var(--ink-3);
	}
	.crumbwrap {
		position: relative;
	}
	.crumbbtn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: none;
		border: none;
		font-size: 12px;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--ink-3);
		padding: 3px 4px;
		margin: -3px -4px;
		border-radius: 5px;
		font-family: inherit;
	}
	.crumbbtn:hover {
		color: var(--ink);
		background: var(--hover);
	}
	.crumbbtn :global(.cchev) {
		opacity: 0.7;
	}
	.crumb {
		font-size: 12px;
		letter-spacing: 0.06em;
		color: var(--ink-2);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.crumb.upper {
		text-transform: uppercase;
	}
	.crumb:not(.upper) {
		font-size: 13.5px;
		letter-spacing: 0;
	}
	.crumb.here {
		color: var(--ink);
		font-weight: 500;
	}
	.dd-bd {
		position: fixed;
		inset: 0;
		z-index: 19;
	}
	.ddmenu {
		position: absolute;
		top: 28px;
		left: -4px;
		z-index: 20;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		padding: 5px;
		min-width: 230px;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.ddsectitle {
		font-family: var(--mono);
		font-size: 9.5px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ink-3);
		padding: 6px 8px 3px;
	}
	.ddi {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 7px 8px;
		border-radius: var(--r-sm);
		font-size: 13px;
		color: var(--ink-2);
		background: none;
		border: none;
		text-align: left;
		width: 100%;
		box-sizing: border-box;
		font-family: inherit;
		text-transform: none;
		letter-spacing: normal;
	}
	.ddi:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.ddi.on {
		color: var(--ink);
	}
	.ddi .nm {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ddi :global(.ck) {
		color: var(--accent);
		flex: none;
	}
	.right {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.new-issue {
		flex: none;
		white-space: nowrap;
	}
	.new-issue .kbd {
		font-family: var(--mono);
		font-size: 10px;
		padding: 0 4px;
		border: 1px solid oklch(1 0 0 / 0.35);
		border-radius: 4px;
		opacity: 0.9;
	}
	:global(.plus-ic) {
		display: none;
		flex: none;
	}
	.connban {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 8px 20px;
		background: var(--sunken);
		border-bottom: 1px solid var(--line);
		color: var(--ink-2);
		font-size: 12.5px;
		flex: none;
	}
	.cbdot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		border: 1.5px solid var(--ink-3);
		flex: none;
	}
	@media (max-width: 720px) {
		.menu {
			display: inline-flex;
		}
		.phrow {
			padding-left: 14px;
			padding-right: 14px;
		}
		.connban {
			padding-left: 14px;
			padding-right: 14px;
		}
		/* The crumb row scrolls on its own instead of wrapping mid-word — the
		   page itself still never scrolls horizontally. */
		.crumbs {
			flex: 1 1 auto;
			min-width: 0;
			overflow-x: auto;
			flex-wrap: nowrap;
			scrollbar-width: none;
		}
		.crumbs::-webkit-scrollbar {
			display: none;
		}
		.crumbbtn,
		.crumb {
			white-space: nowrap;
			flex: none;
		}
		/* "New issue" collapses to its icon so the crumb row keeps room to breathe. */
		.new-issue {
			width: 32px;
			height: 32px;
			padding: 0;
			justify-content: center;
			border-radius: 50%;
		}
		.new-issue .ni-label,
		.new-issue .kbd {
			display: none;
		}
		.new-issue :global(.plus-ic) {
			display: inline-flex;
		}
		/* Workspace / project switchers become bottom sheets on phones. */
		.ddmenu {
			position: fixed;
			top: auto;
			bottom: 0;
			left: 0;
			right: 0;
			border-radius: var(--r-lg) var(--r-lg) 0 0;
			max-height: 70vh;
			overflow-y: auto;
			min-width: 0;
			padding: 10px 10px calc(14px + env(safe-area-inset-bottom, 0px));
		}
	}
</style>
