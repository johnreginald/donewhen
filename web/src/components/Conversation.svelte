<script>
	// A ticket's conversation, as Paperclip draws a task: what you said, what
	// agents replied, each run they did, the questions they asked and how you
	// answered — in time order — and a box to talk back.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import Markdown from './Markdown.svelte';
	import RunBlock from './RunBlock.svelte';
	import QuestionCard from './QuestionCard.svelte';
	import ProposalCard from './ProposalCard.svelte';
	import EpicMenu from './EpicMenu.svelte';
	import { tick } from 'svelte';
	import { Bot, LoaderCircle, CircleHelp, Send, Play } from '@lucide/svelte';

	let { issue } = $props();

	let comments = $state([]);
	let runs = $state([]);
	let interactions = $state([]);
	let turn = $state(null); // the latest chat job
	let draft = $state('');
	let askAgent = $state('');
	let sending = $state(false);
	let scroller = $state(null);

	// Keep the newest message in view, as a chat does, unless the reader has
	// scrolled up to read something older.
	let pinned = true;
	$effect(() => {
		timeline.length;
		thinking;
		if (!scroller) return;
		if (pinned) tick().then(() => scroller && (scroller.scrollTop = scroller.scrollHeight));
	});
	$effect(() => {
		if (!scroller) return;
		const onScroll = () => (pinned = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 40);
		scroller.addEventListener('scroll', onScroll);
		return () => scroller?.removeEventListener('scroll', onScroll);
	});

	const agentById = (id) => $agents.find((a) => a.id === id);
	const theAgent = $derived(agentById(askAgent || issue.agentId || ''));

	async function load() {
		const [c, r, it, jobs] = await Promise.all([
			api.comments(issue.id).catch(() => []),
			api.issueRuns(issue.id).catch(() => []),
			api.get(`/issues/${issue.key}/interactions`).catch(() => []),
			api.jobs({ issue: issue.key, kind: 'chat', limit: 1 }).catch(() => [])
		]);
		comments = c || [];
		runs = r || [];
		interactions = it || [];
		turn = jobs?.[0] || null;
	}
	onMount(load);

	onMount(() =>
		onLive((ev) => {
			if (!ev || (ev.issueId !== issue.id && ev.issue?.id !== issue.id)) return;
			if (ev.type === 'comment.added' && ev.comment && !comments.some((c) => c.id === ev.comment.id)) {
				comments = [...comments, ev.comment];
			}
			if (ev.run) {
				const i = runs.findIndex((r) => r.id === ev.run.id);
				runs = i >= 0 ? runs.map((r) => (r.id === ev.run.id ? ev.run : r)) : [ev.run, ...runs];
			}
			if (ev.interaction) {
				const i = interactions.findIndex((x) => x.id === ev.interaction.id);
				interactions = i >= 0 ? interactions.map((x) => (x.id === ev.interaction.id ? ev.interaction : x)) : [...interactions, ev.interaction];
			}
			if (ev.job && ev.job.kind === 'chat') turn = ev.job;
		})
	);

	// Everything in one timeline. A run sits where it started.
	const timeline = $derived(
		[
			...comments.map((c) => ({ kind: 'comment', at: c.createdAt, c })),
			...runs.map((r) => ({ kind: 'run', at: r.startedAt, r })),
			...interactions.map((x) => ({ kind: 'interaction', at: x.createdAt, x })),
			...interactions
				.filter((x) => x.kind === 'questions' && x.status === 'answered')
				.map((x) => ({ kind: 'answers', at: x.resolvedAt, x }))
		].sort((a, b) => new Date(a.at) - new Date(b.at))
	);
	const thinking = $derived(turn && (turn.status === 'queued' || turn.status === 'claimed'));

	async function comment() {
		if (!draft.trim()) return;
		sending = true;
		try {
			const c = await api.addComment(issue.id, draft.trim());
			if (!comments.some((x) => x.id === c.id)) comments = [...comments, c];
			draft = '';
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			sending = false;
		}
	}
	// Start: the agent opens the conversation from the ticket itself — nothing
	// to type first.
	async function start() {
		if (!theAgent) return;
		sending = true;
		try {
			turn = await api.post(`/issues/${issue.key}/ask`, { agent: theAgent.id, start: true });
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			sending = false;
		}
	}

	async function ask() {
		if (!theAgent) return;
		sending = true;
		try {
			turn = await api.post(`/issues/${issue.key}/ask`, { agent: theAgent.id, message: draft.trim() });
			draft = '';
			comments = (await api.comments(issue.id)) || comments;
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			sending = false;
		}
	}
	function onKey(e) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			theAgent ? ask() : comment();
		}
	}

	function answerLines(x) {
		const qs = x.payload?.questions || [];
		return (x.response?.answers || []).map((a) => {
			const q = qs.find((qq) => qq.id === a.questionId);
			return { q: q?.text || '', a: [...(a.choices || []), a.other].filter(Boolean).join('; ') };
		});
	}
</script>

<section class="conv">
	<header class="ch">
		<span class="rh">Conversation</span>
		{#if theAgent}<span class="with"><Bot size={13} strokeWidth={2} />{theAgent.name}</span>{/if}
	</header>

	<div class="scroll" bind:this={scroller}>
	{#if !timeline.length && !thinking}
		<div class="start">
			{#if theAgent}
				<div class="st-t">Start with {theAgent.name}</div>
				<p>It reads the ticket and its done-when, looks at the code, and either asks you what it needs to know, proposes how to split the work, or tells you it's ready to run.</p>
				<button class="btn primary" onclick={start} disabled={sending || theAgent.status === 'paused'}>
					<Play size={14} strokeWidth={2.4} />Start task
				</button>
			{:else}
				<div class="st-t">No agent yet</div>
				<p>Choose an agent for this ticket — in the bar under the title, or below — to start the conversation.</p>
			{/if}
		</div>
	{/if}

	{#each timeline as item (item.kind + (item.c?.id || item.r?.id || item.x?.id))}
		{#if item.kind === 'comment'}
			{@const c = item.c}
			{#if c.agentId || c.actor === 'ai'}
				<article class="msg agent">
					<header>
						<span class="av"><Bot size={13} strokeWidth={2} /></span>
						<span class="who">{agentById(c.agentId)?.name || 'Clanker'}</span>
						<span class="when">{rel(c.createdAt)}</span>
					</header>
					<div class="body"><Markdown source={c.bodyMd} /></div>
				</article>
			{:else}
				<article class="msg me">
					<div class="bubble"><Markdown source={c.bodyMd} /></div>
					<span class="when">{rel(c.createdAt)}</span>
				</article>
			{/if}
		{:else if item.kind === 'run'}
			<div class="runline"><RunBlock run={item.r} /></div>
		{:else if item.kind === 'interaction'}
			{@const x = item.x}
			{#if x.kind === 'questions'}
				{#if x.status === 'open'}
					<QuestionCard interaction={x} agentName={agentById(x.agentId)?.name}
						ondone={(res) => (interactions = interactions.map((y) => (y.id === res.id ? res : y)))} />
				{:else}
					<div class="note"><CircleHelp size={14} strokeWidth={2} />Questions {x.status}</div>
				{/if}
			{:else}
				<ProposalCard interaction={x} agentName={agentById(x.agentId)?.name}
					ondone={(res) => (interactions = interactions.map((y) => (y.id === res.id ? res : y)))} />
			{/if}
		{:else if item.kind === 'answers'}
			<article class="msg me">
				<div class="bubble">
					<div class="bt">Answered questions</div>
					<ul>
						{#each answerLines(item.x) as l}<li><span class="aq">{l.q}</span> {l.a}</li>{/each}
					</ul>
				</div>
				<span class="when">{rel(item.x.resolvedAt)}</span>
			</article>
		{/if}
	{/each}

	</div>

	{#if thinking}
		<div class="thinking">
			<LoaderCircle size={14} class="spin" />
			{#if turn.status === 'queued'}Waiting for a host to pick up {agentById(turn.agentId)?.name || 'the agent'}…
			{:else}{agentById(turn.agentId)?.name || 'The agent'} is thinking on {turn.host}…{/if}
		</div>
	{:else if turn?.status === 'failed'}
		<div class="failed">{agentById(turn.agentId)?.name || 'The agent'} could not reply: {turn.error}</div>
	{/if}

	<div class="composer">
		<textarea bind:value={draft} onkeydown={onKey} rows="3"
			placeholder={theAgent ? `Message ${theAgent.name}… (⌘↵ to ask)` : 'Leave a comment…'}></textarea>
		<div class="cf">
			{#if !issue.agentId}
				<EpicMenu value={askAgent} options={$agents} icon={Bot} none="Pick an agent" onchange={(v) => (askAgent = v)} />
			{/if}
			<span class="spacer"></span>
			<button class="btn sm" onclick={comment} disabled={sending || !draft.trim()}>Comment</button>
			{#if theAgent}
				<button class="btn primary sm" onclick={ask} disabled={sending || thinking || theAgent.status === 'paused'}>
					<Send size={13} strokeWidth={2.2} />Ask {theAgent.name}
				</button>
			{/if}
		</div>
	</div>
</section>

<style>
	.conv {
		height: 100%;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.ch {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px 16px;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	.with {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12.5px;
		color: var(--text-dim);
		margin-left: auto;
	}
	.scroll {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.start {
		margin: auto 0;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 10px;
		padding: 24px 12px;
	}
	.st-t {
		font-weight: 600;
		font-size: 15px;
	}
	.start p {
		margin: 0;
		font-size: 13px;
		color: var(--text-dim);
		max-width: 340px;
		line-height: 1.5;
	}
	.rh {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-dim);
	}
	.msg.agent header {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 6px;
	}
	.av {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		display: grid;
		place-items: center;
		color: var(--text-dim);
	}
	.who {
		font-weight: 500;
		font-size: 13.5px;
	}
	.when {
		font-size: 11.5px;
		color: var(--text-faint);
	}
	.msg.agent .when {
		margin-left: auto;
	}
	.msg.agent .body {
		padding-left: 30px;
		font-size: 14px;
	}
	.msg.me {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 3px;
	}
	.bubble {
		background: var(--accent);
		color: #fff;
		border-radius: 14px;
		padding: 9px 14px;
		max-width: min(560px, 88%);
		font-size: 14px;
	}
	.bubble :global(p) {
		margin: 0.2em 0;
	}
	.bubble :global(a) {
		color: #fff;
		text-decoration: underline;
	}
	.bt {
		font-weight: 500;
		margin-bottom: 4px;
	}
	.bubble ul {
		margin: 0;
		padding-left: 18px;
	}
	.aq {
		opacity: 0.85;
	}
	.aq::after {
		content: ' →';
	}
	.runline {
		margin: -4px -8px;
	}
	.note {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		font-size: 13px;
		color: var(--text-faint);
	}
	.thinking,
	.failed {
		padding: 0 16px 8px;
	}
	.thinking,
	.failed {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		color: var(--text-dim);
	}
	.failed {
		color: #fca5a5;
	}
	.thinking :global(.spin) {
		animation: spin 1s linear infinite;
		color: var(--st-progress);
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.composer {
		flex-shrink: 0;
		margin: 0 12px 12px;
		border: 1px solid var(--border);
		border-radius: 12px;
		background: var(--bg-elev);
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.composer:focus-within {
		border-color: var(--border-strong);
	}
	.composer textarea {
		background: transparent;
		border: none;
		outline: none;
		resize: vertical;
		color: var(--text);
		font-size: 14px;
		font-family: inherit;
		line-height: 1.5;
	}
	.composer textarea::placeholder {
		color: var(--text-faint);
	}
	.cf {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.spacer {
		flex: 1;
	}
</style>
