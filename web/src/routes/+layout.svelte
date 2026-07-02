<script>
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import Sidebar from '$components/Sidebar.svelte';
	import IssuePanel from '$components/IssuePanel.svelte';
	import CommandPalette from '$components/CommandPalette.svelte';
	import { api } from '$lib/api.js';
	import { connectSSE } from '$lib/sse.js';
	import { loadMeta, loadIssues, applyEvent, me } from '$lib/store.js';
	import { paletteOpen, panelIssueId, toast, showToast } from '$lib/ui.js';
	import { registerServiceWorker } from '$lib/push.js';

	let { children } = $props();
	let ready = $state(false);
	let mobileNav = $state(false);
	let disconnect;

	const isLogin = $derived($page.url.pathname === '/login');

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
		if (e.key === 'Escape') {
			panelIssueId.set(null);
			return;
		}
		if (e.key === 'c' && !isTyping(e) && !$paletteOpen && !$panelIssueId) {
			e.preventDefault();
			paletteOpen.set(true);
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
				<button class="hamburger btn ghost" onclick={() => (mobileNav = !mobileNav)}>☰</button>
				<div class="spacer"></div>
				<button class="btn" onclick={() => paletteOpen.set(true)}>+ New</button>
				<span class="email faint">{$me?.email}</span>
				<button class="btn ghost" onclick={logout}>Logout</button>
			</header>
			<div class="content">
				{@render children()}
			</div>
		</main>
	</div>
	<IssuePanel />
	<CommandPalette />
{:else}
	<div class="booting">Loading Kanri…</div>
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
		border-bottom: 1px solid var(--border);
		background: var(--bg);
	}
	.spacer {
		flex: 1;
	}
	.email {
		font-size: 12px;
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
