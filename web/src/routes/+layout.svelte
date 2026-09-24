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
	import { get } from 'svelte/store';
	import { loadMeta, loadIssues, loadWorkspaces, applyEvent, me, activeWorkspace, agents, issues, inboxCount } from '$lib/store.js';
	import { paletteOpen, toast, showToast, flashIssue, composer, openComposer, liveEvent, navOpen } from '$lib/ui.js';
	import { registerServiceWorker } from '$lib/push.js';
	import { CircleCheckBig, Inbox, History, LayoutDashboard } from '@lucide/svelte';

	// Mobile bottom-tab nav — surfaces the record surfaces (review / work / history / artifacts).
	const tabs = [
		{ label: 'Home', href: '/dashboard', icon: LayoutDashboard, match: (p) => p === '/dashboard' },
		{ label: 'Inbox', href: '/inbox', icon: Inbox, match: (p) => p === '/inbox' },
		{ label: 'Tasks', href: '/tasks', icon: CircleCheckBig, match: (p) => ['/tasks', '/list', '/board'].includes(p) },
		{ label: 'Audit', href: '/log', icon: History, match: (p) => p === '/log' },
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
			disconnect = connectSSE(handleEvent);
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
		disconnect = connectSSE(handleEvent);
	});

	function handleEvent(ev) {
		liveEvent.set(ev);
		applyEvent(ev);
		if (ev.issue) flashIssue(ev.issue.id);
		// Paperclip's "Agent is asking a question": say so wherever you are.
		if (ev.type === 'interaction.created' && ev.interaction) {
			const who = get(agents).find((a) => a.id === ev.interaction.agentId)?.name || 'An agent';
			const key = get(issues).find((i) => i.id === ev.interaction.issueId)?.key || 'a ticket';
			showToast(ev.interaction.kind === 'questions' ? `${who} is asking you something on ${key}` : `${who} proposes tickets on ${key}`);
			inboxCount.update((n) => n + 1);
		}
		if (ev.type === 'interaction.updated') inboxCount.update((n) => Math.max(0, n - 1));
		if (ev.type === 'issue.state_changed' && ev.issue && ev.to) {
			const who = ev.actor === 'ai' ? 'Clanker' : 'you';
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
	<Composer />
{:else}
	<div class="booting">Loading Raenil…</div>
{/if}

{#if $toast}
	<div class="toast" class:error={$toast.kind === 'error'}>{$toast.message}</div>
{/if}

<style>
	.empty-shell {
		max-width: 460px;
		margin: 18vh auto;
		padding: 0 20px;
		text-align: center;
		color: var(--text-dim);
	}
	.empty-shell h1 {
		font-size: 20px;
		color: var(--text);
		margin-bottom: 10px;
	}
	.empty-shell pre {
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 10px 12px;
		font-family: var(--mono);
		font-size: 12.5px;
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
			background: color-mix(in srgb, var(--bg) 92%, transparent);
			backdrop-filter: saturate(1.4) blur(10px);
			border-top: 1px solid var(--border);
			padding-bottom: env(safe-area-inset-bottom, 0px);
		}
		.btab {
			flex: 1;
			display: flex;
			flex-direction: column;
			align-items: center;
			gap: 3px;
			padding: 8px 0 7px;
			color: var(--text-faint);
			font-size: 10.5px;
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
			background: rgba(0, 0, 0, 0.5);
			z-index: 45;
		}
	}
</style>
