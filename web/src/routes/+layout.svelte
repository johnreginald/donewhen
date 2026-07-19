<script>
	import '@fontsource-variable/inter';
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import Sidebar from '$components/Sidebar.svelte';
	import CommandPalette from '$components/CommandPalette.svelte';
	import Composer from '$components/Composer.svelte';
	import { api } from '$lib/api.js';
	import { connectSSE } from '$lib/sse.js';
	import { loadMeta, loadIssues, applyEvent, me } from '$lib/store.js';
	import { paletteOpen, toast, showToast, flashIssue, composer, openComposer } from '$lib/ui.js';
	import { registerServiceWorker } from '$lib/push.js';
	import { Columns3, List, Plus, Settings, LogOut, Menu } from '@lucide/svelte';
	import LabelFilter from '$components/LabelFilter.svelte';

	let { children } = $props();
	let ready = $state(false);
	let mobileNav = $state(false);
	let userOpen = $state(false);
	let disconnect;

	const initials = $derived(($me?.email || '?').slice(0, 2).toUpperCase());

	const isLogin = $derived($page.url.pathname === '/login');
	const showView = $derived(['/', '/list'].includes($page.url.pathname));

	onMount(() => {
		registerServiceWorker();
		window.addEventListener('keydown', globalKeys);
		boot();
		return () => {
			window.removeEventListener('keydown', globalKeys);
			disconnect && disconnect();
		};
	});

	async function boot() {
		try {
			const status = await api.authStatus();
			if (!status.authenticated) {
				goto('/login');
				return;
			}
			me.set(status.user);
			await loadMeta();
			await loadIssues();
			disconnect = connectSSE(handleEvent);
			ready = true;
		} catch {
			goto('/login');
		}
	}

	function handleEvent(ev) {
		applyEvent(ev);
		if (ev.issue) flashIssue(ev.issue.id);
		if (ev.type === 'issue.state_changed' && ev.issue && ev.to) {
			const who = ev.actor === 'ai' ? 'AI' : 'you';
			showToast(`${ev.issue.key} → ${ev.to.name} (by ${who})`);
		}
	}

	function isTyping(e) {
		const t = e.target;
		return t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable);
	}

	function globalKeys(e) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			paletteOpen.update((v) => !v);
			return;
		}
		if (e.key === 'c' && !isTyping(e) && !$paletteOpen && !$composer) {
			e.preventDefault();
			openComposer('issue');
		}
	}

	async function logout() {
		await api.logout();
		goto('/login');
	}
</script>

{#if isLogin}
	{@render children()}
{:else if ready}
	<div class="shell">
		<div class="nav-col" class:open={mobileNav}>
			<Sidebar onnavigate={() => (mobileNav = false)} />
		</div>
		{#if mobileNav}
			<div class="nav-backdrop" role="presentation" onclick={() => (mobileNav = false)}></div>
		{/if}
		<main>
			<header class="topbar">
				<button class="hamburger btn ghost" onclick={() => (mobileNav = !mobileNav)}><Menu size={18} /></button>
				{#if showView}
					<div class="vtoggle">
						<a href="/" class="vt" class:on={$page.url.pathname === '/'}><Columns3 size={15} strokeWidth={2} />Board</a>
						<a href="/list" class="vt" class:on={$page.url.pathname === '/list'}><List size={15} strokeWidth={2} />List</a>
					</div>
					<LabelFilter />
				{/if}
				<div class="spacer"></div>
				<button class="btn primary np" onclick={() => openComposer('issue')}><Plus size={16} strokeWidth={2.4} /><span class="np-label">New issue</span></button>
				<div class="usermenu">
					<button class="avatar" title={$me?.email} onclick={() => (userOpen = !userOpen)}>{initials}</button>
					{#if userOpen}
						<div class="umbd" role="presentation" onclick={() => (userOpen = false)}></div>
						<div class="umenu">
							<div class="umhead">{$me?.email}</div>
							<a href="/settings" class="umitem" onclick={() => (userOpen = false)}>
								<Settings size={15} strokeWidth={2} />Settings
							</a>
							<button class="umitem danger" onclick={logout}><LogOut size={15} strokeWidth={2} />Log out</button>
						</div>
					{/if}
				</div>
			</header>
			<div class="content">
				{@render children()}
			</div>
		</main>
	</div>
	<CommandPalette />
	<Composer />
{:else}
	<div class="booting">Loading Raenil…</div>
{/if}

{#if $toast}
	<div class="toast" class:error={$toast.kind === 'error'}>{$toast.message}</div>
{/if}

<style>
	.shell {
		display: flex;
		height: 100%;
	}
	.nav-col {
		flex-shrink: 0;
	}
	main {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.topbar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 14px;
		/* PWA / notch: fill the safe area, keep controls below the status bar */
		padding-top: calc(8px + env(safe-area-inset-top, 0px));
		padding-left: calc(14px + env(safe-area-inset-left, 0px));
		padding-right: calc(14px + env(safe-area-inset-right, 0px));
		border-bottom: 1px solid var(--border);
		background: var(--bg);
	}
	.spacer {
		flex: 1;
	}
	.vtoggle {
		display: flex;
		gap: 2px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2px;
	}
	.vt {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 4px 11px;
		border-radius: 6px;
		font-size: 13px;
		color: var(--text-dim);
	}
	.vt:hover {
		color: var(--text);
	}
	.vt.on {
		background: var(--bg-hover);
		color: var(--text);
	}
	.usermenu {
		position: relative;
	}
	.avatar {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		border: none;
		background: var(--accent-grad);
		color: #fff;
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.02em;
		display: grid;
		place-items: center;
	}
	.avatar:hover {
		filter: brightness(1.1);
	}
	.umbd {
		position: fixed;
		inset: 0;
		z-index: 40;
	}
	.umenu {
		position: absolute;
		top: 36px;
		right: 0;
		z-index: 41;
		min-width: 200px;
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 10px;
		box-shadow: var(--shadow);
		padding: 5px;
		display: flex;
		flex-direction: column;
	}
	.umhead {
		font-size: 12px;
		color: var(--text-faint);
		padding: 7px 9px 6px;
		border-bottom: 1px solid var(--border);
		margin-bottom: 4px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.umitem {
		display: flex;
		align-items: center;
		gap: 9px;
		background: none;
		border: none;
		color: var(--text);
		text-align: left;
		padding: 8px 9px;
		border-radius: 6px;
		font-size: 13.5px;
	}
	.umitem:hover {
		background: var(--bg-hover);
	}
	.umitem.danger {
		color: #f87171;
	}
	.umi {
		width: 15px;
		text-align: center;
		color: var(--text-faint);
	}
	.umitem.danger .umi {
		color: #f87171;
	}
	.hamburger {
		display: none;
	}
	.content {
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}
	.booting {
		display: grid;
		place-items: center;
		height: 100%;
		color: var(--text-dim);
	}
	.nav-backdrop {
		display: none;
	}
	.toast {
		position: fixed;
		bottom: 20px;
		left: 50%;
		transform: translateX(-50%);
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		color: var(--text);
		padding: 10px 16px;
		border-radius: var(--radius);
		box-shadow: var(--shadow);
		z-index: 80;
		font-size: 13px;
	}
	.toast.error {
		border-color: #f87171;
		color: #fca5a5;
	}
	@media (max-width: 720px) {
		.hamburger {
			display: inline-flex;
		}
		.topbar {
			gap: 6px;
		}
		.np-label {
			display: none;
		}
		.np {
			padding-left: 10px;
			padding-right: 10px;
		}
		.nav-col {
			position: fixed;
			left: 0;
			top: 0;
			height: 100%;
			z-index: 50;
			transform: translateX(-100%);
			transition: transform 0.18s ease;
		}
		.nav-col.open {
			transform: translateX(0);
		}
		.nav-backdrop {
			display: block;
			position: fixed;
			inset: 0;
			background: rgba(0, 0, 0, 0.5);
			z-index: 45;
		}
	}
</style>
