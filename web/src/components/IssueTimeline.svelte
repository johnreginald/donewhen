<script>
	import { aiName } from '$lib/store.js';
	// One Activity timeline for a ticket: comments (full cards) and history
	// events (compact rows) merged by time, oldest at the top, composer pinned
	// below. Kept live, so a comment or event from another tab or an MCP client
	// appends here without a reload.
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel, activityVerb as verb } from '$lib/format.js';
	import Markdown from '$components/Markdown.svelte';
	import { Plus, CircleDot, SignalHigh, Box, Type, FileText, Trash2 } from '@lucide/svelte';

	let { issue } = $props();

	const FILTERS = [
		['all', 'All'],
		['comments', 'Comments'],
		['history', 'History']
	];
	const KEY = 'donewhen.issueActivityFilter';
	const ICON = {
		created: Plus,
		state_changed: CircleDot,
		priority_changed: SignalHigh,
		epic_changed: Box,
		title_changed: Type,
		artifact_written: FileText,
		deleted: Trash2
	};

	let comments = $state([]);
	let commentsFailed = $state(false); // the comments call failed: say so, not "No comments yet"
	let events = $state([]);
	let filter = $state('all');
	let draft = $state('');
	let sending = $state(false);
	let list = $state(null);
	let nearBottom = true;

	// "commented" rows duplicate the comment card, so they never enter the timeline.
	const history = $derived(events.filter((e) => e.kind !== 'commented'));
	const entries = $derived(
		[
			...comments.map((c) => ({ type: 'comment', id: 'c' + c.id, at: c.createdAt, c })),
			...history.map((a) => ({ type: 'event', id: 'e' + a.id, at: a.createdAt, a }))
		].sort((x, y) => new Date(x.at) - new Date(y.at))
	);
	const shown = $derived(
		filter === 'comments' ? entries.filter((e) => e.type === 'comment') : filter === 'history' ? entries.filter((e) => e.type === 'event') : entries
	);
	const counts = $derived({ all: entries.length, comments: comments.length, history: history.length });

	function setFilter(f) {
		filter = f;
		try {
			localStorage.setItem(KEY, f);
		} catch {}
		toBottom();
	}
	async function toBottom() {
		await tick();
		if (list) list.scrollTop = list.scrollHeight;
	}
	function onScroll() {
		nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 80;
	}

	async function load(initial = false) {
		const stick = initial || nearBottom;
		const FAILED = Symbol('failed');
		const [c, a] = await Promise.all([api.comments(issue.id).catch(() => FAILED), api.issueActivity(issue.id).catch(() => null)]);
		commentsFailed = c === FAILED;
		if (!commentsFailed) comments = c || [];
		if (a) events = a;
		if (stick) toBottom();
	}

	onMount(() => {
		try {
			const f = localStorage.getItem(KEY);
			if (FILTERS.some(([k]) => k === f)) filter = f;
		} catch {}
		load(true);
		// On a narrow window this pane is hidden until its tab is opened; when it
		// first gets a height, show the newest entry.
		let wasHidden = true;
		const ro = new ResizeObserver(() => {
			const hidden = !list || list.clientHeight === 0;
			if (wasHidden && !hidden) list.scrollTop = list.scrollHeight;
			wasHidden = hidden;
		});
		if (list) ro.observe(list);
		const off = onLive((ev) => {
			const id = ev.issue?.id || ev.issueId;
			if (id === issue.id && ev.type !== 'issue.deleted') load();
		});
		return () => {
			ro.disconnect();
			off();
		};
	});

	async function send() {
		const body = draft.trim();
		if (!body || sending) return;
		sending = true;
		try {
			await api.addComment(issue.id, body);
			draft = '';
			nearBottom = true; // our own comment: always show it
			await load();
		} catch (e) {
			showToast('Comment failed: ' + e.message, 'error');
		} finally {
			sending = false;
		}
	}
	function keydown(e) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			send();
		}
	}
</script>

<div class="tl">
	<div class="head">
		<span class="title">Activity</span>
		<div class="seg" role="group" aria-label="Filter activity">
			{#each FILTERS as [k, label] (k)}
				<button class:on={filter === k} aria-pressed={filter === k} onclick={() => setFilter(k)}>
					{label}<span class="n">{counts[k]}</span>
				</button>
			{/each}
		</div>
	</div>
	<div class="list" bind:this={list} onscroll={onScroll}>
		{#each shown as e (e.id)}
			{#if e.type === 'comment'}
				<div class="c">
					<div class="meta">
						<span class="chip" class:ai={e.c.actor === 'ai'}>{e.c.actor === 'ai' ? `✦ ${$aiName}` : 'You'}</span>
						<span class="when">{rel(e.at)}</span>
					</div>
					<div class="body"><Markdown source={e.c.bodyMd} /></div>
				</div>
			{:else}
				{@const Icon = ICON[e.a.kind] || CircleDot}
				<div class="ev">
					<span class="ic"><Icon size={12} strokeWidth={2} /></span>
					<span class="evtext"><span class="actor" class:ai={e.a.actor === 'ai'}>{e.a.actor === 'ai' ? `✦ ${$aiName}` : 'You'}</span> {verb(e.a)}</span>
					<span class="when">{rel(e.at)}</span>
				</div>
			{/if}
		{:else}
			<div class="empty" class:err={commentsFailed && filter !== 'history'}>{commentsFailed && filter !== 'history' ? "Couldn't load comments." : filter === 'comments' ? 'No comments yet. Be the first to leave one.' : 'No activity yet.'}</div>
		{/each}
	</div>
	<div class="add">
		<textarea class="textarea" bind:value={draft} onkeydown={keydown} placeholder="Leave a comment…" rows="3"></textarea>
		<div class="row">
			<span class="hint">⌘↵ to send</span>
			<button class="btn primary sm" onclick={send} disabled={!draft.trim() || sending}>Comment</button>
		</div>
	</div>
</div>

<style>
	.tl {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		flex-wrap: wrap;
		padding: 10px 16px;
		border-bottom: 1px solid var(--line);
	}
	.title {
		font-size: var(--t-xs);
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--ink-2);
	}
	.seg {
		display: inline-flex;
		background: var(--sunken);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 2px;
		gap: 2px;
	}
	.seg button {
		background: none;
		border: none;
		border-radius: var(--r-sm);
		padding: 3px 9px;
		font-size: var(--t-xs);
		color: var(--ink-2);
	}
	.seg button.on {
		background: var(--surface);
		color: var(--ink);
		box-shadow: 0 0 0 1px var(--line-strong);
	}
	.n {
		margin-left: 5px;
		color: var(--ink-3);
	}
	.list {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 12px 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.c {
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 10px 12px;
		background: var(--surface);
	}
	.meta {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 4px;
	}
	.chip {
		font-size: var(--t-xs);
		font-weight: 600;
		color: var(--ink);
		background: var(--sunken);
		border: 1px solid var(--line);
		border-radius: 999px;
		padding: 1px 8px;
	}
	.chip.ai {
		color: var(--accent);
	}
	.when {
		font-size: var(--t-xs);
		color: var(--ink-3);
		flex: none;
	}
	.body {
		font-family: var(--serif);
		font-size: var(--t-base);
		line-height: 1.55;
		overflow-wrap: anywhere;
	}
	.ev {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: var(--t-sm);
		color: var(--ink-2);
		padding: 0 2px;
	}
	.ic {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 1px solid var(--line);
		background: var(--surface);
		display: grid;
		place-items: center;
		flex: none;
	}
	.evtext {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.actor {
		font-weight: 500;
		color: var(--ink);
	}
	.actor.ai {
		color: var(--accent);
	}
	.empty.err {
		color: var(--danger);
	}
	.empty {
		font-size: var(--t-sm);
		color: var(--ink-3);
		text-align: center;
		padding: 12px 0;
	}
	.add {
		border-top: 1px solid var(--line);
		padding: 10px 16px 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	textarea {
		min-height: 64px;
	}
	.row {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.hint {
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
</style>
