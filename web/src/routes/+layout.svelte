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
	import BlockedReasonDialog from '$components/BlockedReasonDialog.svelte';
	import IssueMenuHost from '$components/IssueMenuHost.svelte';
	import { SHORTCUTS, isTypingTarget, stepIndex, createChord } from '$lib/shortcuts.js';
	import ToastStack from '$components/ToastStack.svelte';
	import Composer from '$components/Composer.svelte';
	import ArchiveEpicDialog from '$components/ArchiveEpicDialog.svelte';
	import { api, expireSession, setNotifier } from '$lib/api.js';
	import { connectSSE } from '$lib/sse.js';
	import { loadMeta, loadIssues, loadViews, loadWorkspaces, applyEvent, catchUp, me, activeWorkspace, workspaces, switchWorkspace, switching } from '$lib/store.js';
	import {
		paletteOpen,
		quickCapture,
		shortcutHelp,
		blockPrompt,
		issueMenu,
		archiveTarget,
		connectionLost,
		streamStatus,
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
	let bootFailed = $state(false); // the server could not be reached, or errored, while starting

	const isLogin = $derived($page.url.pathname === '/login');

	onMount(() => {
		registerServiceWorker();
		window.addEventListener('keydown', globalKeys, true);
		window.addEventListener('keydown', backKey);
		boot();
		return () => {
			window.removeEventListener('keydown', globalKeys, true);
			window.removeEventListener('keydown', backKey);
		};
	});

	// After boot, a network failure is a toast and the screen keeps its data. At
	// most one every few seconds: a dead server fails many calls at once.
	let lastNetToast = 0;
	function toastNetwork(message) {
		if (Date.now() - lastNetToast < 5000) return;
		lastNetToast = Date.now();
		showToast(message, 'error');
	}

	async function boot() {
		bootFailed = false;
		try {
			const status = await api.authStatus();
			if (!status.authenticated) {
				expireSession();
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
			if (!(await loadIssues())) throw new Error("Couldn't load issues");
			loadViews();
			ready = true;
			setNotifier(toastNetwork);
		} catch (e) {
			if (e?.status === 403) {
				// Not a member of the stored workspace; api.js already cleared
				// it, so a retry falls back to one we do have.
				noWorkspace = true;
				ready = true;
				return;
			}
			// A 401 is already on its way to the login page. Anything else (the
			// server is down, a 5xx) is not a logout: offer a retry instead.
			if (e?.status !== 401) bootFailed = true;
		}
	}

	// The SSE stream is bound to the workspace it opened with. This effect owns
	// it: exactly one stream per workspace, closed while a switch is in flight
	// and reopened once the server has recorded the new workspace.
	const streamWs = $derived($activeWorkspace?.id);
	$effect(() => {
		if (!ready || !streamWs || $switching) return;
		const close = connectSSE(streamWs, {
			onEvent: handleEvent,
			onStatus: handleSSEStatus,
			onCatchUp: () => catchUp().catch(() => {})
		});
		return () => {
			close();
			streamStatus.set('');
		};
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

	// Drives the sidebar dot and the connection-lost banner (PP-209).
	function handleSSEStatus(status) {
		streamStatus.set(status);
		connectionLost.set(status === 'reconnecting' || status === 'offline');
	}

	// "g" then a letter jumps somewhere; the second key must land in a short window.
	const chord = createChord();

	// The issue the shortcuts act on: the one holding focus (a card or row), the
	// selected inbox card, or the issue page being read.
	function targetIssueKey() {
		const el = document.activeElement?.closest?.('[data-issue-key]') || document.querySelector('[data-issue-key][data-selected="true"]');
		if (el) return el.getAttribute('data-issue-key');
		const m = /^\/issue\/([^/]+)/.exec(get(page).url.pathname);
		return m ? decodeURIComponent(m[1]) : '';
	}

	function moveIssueFocus(dir) {
		const els = [...document.querySelectorAll('[data-issue-key]')].filter((el) => el.offsetParent !== null);
		const cur = els.indexOf(document.activeElement?.closest?.('[data-issue-key]'));
		const next = els[stepIndex(cur, els.length, dir)];
		if (!next) return;
		next.focus();
		next.scrollIntoView({ block: 'nearest' });
	}

	const dialogOpen = () =>
		get(paletteOpen) || get(quickCapture) || get(shortcutHelp) || get(composer) || get(blockPrompt) || get(issueMenu) || get(archiveTarget);

	// Capture phase, so "g b" is settled before a page's own "b" (the inbox's
	// bounce) can see it.
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
		// Everything below is a bare key: never while typing, never with a
		// modifier, never on top of a dialog that is already up.
		if (isTypingTarget(e.target) || e.metaKey || e.ctrlKey || e.altKey || dialogOpen()) return;
		const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;

		if (chord.armed && chord.take(e.timeStamp)) {
			const go = SHORTCUTS.find((s) => s.to && s.keys[1].toLowerCase() === key);
			if (go) {
				e.preventDefault();
				e.stopImmediatePropagation();
				goto(go.to);
				return;
			}
		}
		if (key === 'g') {
			chord.arm(e.timeStamp);
		} else if (key === 'c') {
			e.preventDefault();
			quickCapture.set(true);
		} else if (key === '?') {
			e.preventDefault();
			shortcutHelp.set(true);
		} else if ((key === 'j' || key === 'k') && get(page).url.pathname !== '/inbox') {
			// The inbox keeps its own j/k: its selection expands a review card.
			e.preventDefault();
			moveIssueFocus(key === 'j' ? 1 : -1);
		} else {
			const menu = SHORTCUTS.find((s) => s.menu && s.keys[0].toLowerCase() === key);
			const issueKey = menu && targetIssueKey();
			if (issueKey) {
				e.preventDefault();
				issueMenu.set({ kind: menu.menu, key: issueKey });
			}
		}
	}

	// Esc on an issue page goes back to the list it was opened from. Bubble
	// phase and defaultPrevented: a menu, field or dialog that used Esc wins.
	function backKey(e) {
		if (e.key !== 'Escape' || e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return;
		if (isTypingTarget(e.target) || dialogOpen() || !get(page).url.pathname.startsWith('/issue/')) return;
		if (document.querySelector('.dd-menu')) return;
		e.preventDefault();
		if (window.history.length > 1) window.history.back();
		else goto('/board');
	}

</script>

{#if isLogin}
	{@render children()}
{:else if bootFailed}
	<div class="empty-shell">
		<h1>Can't reach DoneWhen</h1>
		<p>The server did not answer. Your session is still valid; check the connection and try again.</p>
		<button class="btn" onclick={boot}>Retry</button>
	</div>
{:else if noWorkspace}
	<div class="empty-shell">
		<h1>No workspace</h1>
		<p>
			This account is not a member of any workspace. Ask an owner to add you, or create one
			from the command line:
		</p>
		<pre>donewhen workspace create "My Workspace" MYW</pre>
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
	<BlockedReasonDialog />
	<IssueMenuHost />
	<Composer />
	<ArchiveEpicDialog />
{:else}
	<div class="booting">Loading DoneWhen…</div>
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
