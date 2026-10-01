<script>
	import { aiName } from '$lib/store.js';
	import '../app.css';
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import Sidebar from '$components/Sidebar.svelte';
	import CommandPalette from '$components/CommandPalette.svelte';
	import QuickCapture from '$components/QuickCapture.svelte';
	import ShortcutHelp from '$components/ShortcutHelp.svelte';
	import ToastStack from '$components/ToastStack.svelte';
	import Composer from '$components/Composer.svelte';
	import ArchiveEpicDialog from '$components/ArchiveEpicDialog.svelte';
	import { api } from '$lib/api.js';
	import { connectSSE } from '$lib/sse.js';
	import { loadMeta, loadIssues, loadWorkspaces, applyEvent, me, activeWorkspace, inboxCount, workspaces, switchWorkspace } from '$lib/store.js';
	import {
		paletteOpen,
		quickCapture,
		shortcutHelp,
		connectionLost,
		composer,
		showToast,
		flashIssue,
		liveEvent,
		navOpen
	} from '$lib/ui.js';
	import { registerServiceWorker } from '$lib/push.js';
	import { CircleCheckBig, Inbox, History, FileText } from '@lucide/svelte';

	// Mobile bottom-tab nav — surfaces the record surfaces (review / work / history / artifacts).
	const tabs = [
		{ label: 'Tasks', href: '/board', icon: CircleCheckBig, match: (p) => ['/tasks', '/list', '/by-epic', '/board', '/links', '/blocked'].includes(p) },
		{ label: 'Inbox', href: '/inbox', icon: Inbox, match: (p) => p === '/inbox' },
		{ label: 'Artifacts', href: '/artifacts', icon: FileText, match: (p) => p.startsWith('/artifacts') },
		{ label: 'Log', href: '/log', icon: History, match: (p) => p === '/log' },
	];

	let { children } = $props();
	let ready = $state(false);
	let noWorkspace = $state(false);
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
			// Resolve the workspace first: every request below is scoped to it.
			const wsp = await loadWorkspaces();
			if (!wsp) {
				noWorkspace = true;
				ready = true;
				return;
			}
			await loadMeta();
			await loadIssues();
			disconnect = connectSSE(handleEvent, handleSSEStatus);
			ready = true;
		} catch (e) {
			if (e?.status === 403) {
				// Not a member of the stored workspace; api.js already cleared
				// it, so a retry falls back to one we do have.
				noWorkspace = true;
				ready = true;
				return;
			}
			goto('/login');
		}
	}

	// The SSE stream is bound to the workspace it opened with, so it has to be
	// torn down and reopened whenever the active workspace changes.
	let streamFor = $state(null);
	$effect(() => {
		const wsp = $activeWorkspace;
		if (!ready || !wsp || streamFor === wsp.id) return;
		streamFor = wsp.id;
		disconnect && disconnect();
		disconnect = connectSSE(handleEvent, handleSSEStatus);
	});

	function handleEvent(ev) {
		liveEvent.set(ev);
		applyEvent(ev);
		if (ev.issue) flashIssue(ev.issue.id);
		if (ev.type === 'issue.state_changed' && ev.issue && ev.to) {
			const who = ev.actor === 'ai' ? $aiName : 'you';
			showToast(`${ev.issue.key} → ${ev.to.name} (by ${who})`);
		}
	}

	// Drives the connection-lost banner (PP-209): EventSource retries on its
	// own, we just surface whether the stream is currently up.
	function handleSSEStatus(status) {
		connectionLost.set(status === 'error');
	}

	function isTypingTarget(el) {
		if (!el) return false;
		const tag = el.tagName;
		return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable;
	}

	function globalKeys(e) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			paletteOpen.update((v) => !v);
			return;
		}
		// Ctrl+1…9 switches to the Nth workspace. Ctrl, not ⌘: ⌘1…9 is the
		// browser's own tab switcher. Works even while typing in a field.
		if (e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey && /^[1-9]$/.test(e.key)) {
			const ws = get(workspaces)[Number(e.key) - 1];
			if (ws) {
				e.preventDefault();
				if (ws.id !== get(activeWorkspace)?.id) switchWorkspace(ws.slug);
			}
			return;
		}
		// "C" (quick capture) and "?" (shortcut help) — never while typing, and
		// never stacked on top of another dialog that's already up.
		if (isTypingTarget(e.target)) return;
		if (get(paletteOpen) || get(quickCapture) || get(shortcutHelp) || get(composer)) return;
		if (e.key === 'c' || e.key === 'C') {
			e.preventDefault();
			quickCapture.set(true);
		} else if (e.key === '?') {
			e.preventDefault();
			shortcutHelp.set(true);
		}
	}

</script>

{#if isLogin}
	{@render children()}
{:else if noWorkspace}
	<div class="empty-shell">
		<h1>No workspace</h1>
		<p>
			This account is not a member of any workspace. Ask an owner to add you, or create one
			from the command line:
		</p>
		<pre>raenil workspace create "My Workspace" MYW</pre>
		<button class="btn" onclick={() => location.reload()}>Retry</button>
	</div>
{:else if ready}
	<div class="shell">
		<div class="nav-col" class:open={$navOpen}>
			<Sidebar onnavigate={() => navOpen.set(false)} />
		</div>
		{#if $navOpen}
			<div class="nav-backdrop" role="presentation" onclick={() => navOpen.set(false)}></div>
		{/if}
		<main>
			<div class="content">
				{@render children()}
			</div>
		</main>
	</div>
	<nav class="btabs" aria-label="Mobile">
		{#each tabs as t (t.href)}
			{@const Icon = t.icon}
			<a href={t.href} class="btab" class:on={t.match($page.url.pathname)} aria-label={t.label}>
				<Icon size={20} strokeWidth={2} />
				<span>{t.label}</span>
			</a>
		{/each}
	</nav>
	<CommandPalette />
	<QuickCapture />
	<ShortcutHelp />
	<Composer />
	<ArchiveEpicDialog />
{:else}
	<div class="booting">Loading Raenil…</div>
{/if}

<ToastStack />

<style>
	.empty-shell {
		max-width: 460px;
		margin: 18vh auto;
		padding: 0 20px;
		text-align: center;
		color: var(--ink-2);
	}
	.empty-shell h1 {
		font-size: var(--t-lg);
		color: var(--ink);
		margin-bottom: 10px;
	}
	.empty-shell pre {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 10px 12px;
		font-family: var(--mono);
		font-size: var(--t-sm);
		margin: 14px 0 18px;
		overflow-x: auto;
		text-align: left;
	}

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
	.content {
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}
	.booting {
		display: grid;
		place-items: center;
		height: 100%;
		color: var(--ink-2);
	}
	.nav-backdrop {
		display: none;
	}
	.btabs {
		display: none;
	}

	@media (max-width: 720px) {
		/* reserve room for the fixed bottom tab bar */
		main {
			padding-bottom: calc(54px + env(safe-area-inset-bottom, 0px));
		}
		.btabs {
			display: flex;
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			z-index: 46;
			background: color-mix(in srgb, var(--paper) 92%, transparent);
			backdrop-filter: saturate(1.4) blur(10px);
			border-top: 1px solid var(--line);
			padding-bottom: env(safe-area-inset-bottom, 0px);
		}
		.btab {
			flex: 1;
			display: flex;
			flex-direction: column;
			align-items: center;
			gap: 3px;
			padding: 8px 0 7px;
			color: var(--ink-3);
			font-size: var(--t-xs);
			font-weight: 500;
		}
		.btab.on {
			color: var(--accent);
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
			background: oklch(0 0 0 / 0.5);
			z-index: 45;
		}
	}
</style>
