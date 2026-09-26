<script>
	// A ticket's conversation, as Paperclip draws a task: what you said, what
	// agents replied, each run they did, the questions they asked and how you
	// answered — in time order — and a box to talk back.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents, priorityAgents, taskAgentId } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import Markdown from './Markdown.svelte';
	import PermissionCard from './PermissionCard.svelte';
	import RunThread from './RunThread.svelte';
	import QuestionCard from './QuestionCard.svelte';
	import ProposalCard from './ProposalCard.svelte';
	import EpicMenu from './EpicMenu.svelte';
	import { tick } from 'svelte';
	import { Bot, LoaderCircle, CircleHelp, CircleAlert, Send, Play, ArrowDown } from '@lucide/svelte';

	let { issue } = $props();

	let comments = $state([]);
	let runs = $state([]);
	let interactions = $state([]);
	let turn = $state(null); // the latest chat job
	let work = $state(null); // the latest run_ticket job
	let draft = $state('');
	let askAgent = $state('');
	let sending = $state(false);
	let scroller = $state(null);

	// Keep the newest message in view, as a chat does — including a run's
	// lines as they stream in — unless the reader has scrolled up to read
	// something older; then offer a way back down.
	let pinned = $state(true);
	const toBottom = (smooth = false) =>
		scroller && scroller.scrollTo({ top: scroller.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
	$effect(() => {
		if (!scroller) return;
		const onScroll = () => (pinned = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 60);
		scroller.addEventListener('scroll', onScroll);
		// Whatever grows the thread — a message, a card, a transcript line, an
		// image loading — follows the bottom while pinned.
		const grew = new ResizeObserver(() => pinned && toBottom());
		for (const child of scroller.children) grew.observe(child);
		const added = new MutationObserver(() => {
			for (const child of scroller.children) grew.observe(child);
			if (pinned) toBottom();
		});
		added.observe(scroller, { childList: true, subtree: true });
		tick().then(() => toBottom());
		return () => {
			scroller?.removeEventListener('scroll', onScroll);
			grew.disconnect();
			added.disconnect();
		};
	});
	$effect(() => {
		timeline.length;
		thinking;
		if (pinned) tick().then(() => toBottom());
	});

	const agentById = (id) => $agents.find((a) => a.id === id);
	// The ticket's agent, or the default for its priority when none is chosen.
	const ticketAgentId = $derived(taskAgentId(issue, $priorityAgents));
	const theAgent = $derived(agentById(askAgent || ticketAgentId));

	async function load() {
		const [c, r, it, jobs, wjobs] = await Promise.all([
			api.comments(issue.id).catch(() => []),
			api.issueRuns(issue.id).catch(() => []),
			api.get(`/issues/${issue.key}/interactions`).catch(() => []),
			api.jobs({ issue: issue.key, kind: 'chat', limit: 1 }).catch(() => []),
			api.jobs({ issue: issue.key, kind: 'run_ticket', limit: 1 }).catch(() => [])
		]);
		work = wjobs?.[0] || null;
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
			if (ev.job && ev.job.kind === 'run_ticket') work = ev.job;
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
				.map((x) => ({ kind: 'answers', at: x.resolvedAt, x })),
			...(workFailed ? [{ kind: 'startfail', at: work.finishedAt || work.createdAt, j: work }] : [])
		].sort((a, b) => new Date(a.at) - new Date(b.at))
	);
	// The refused-commands card belongs to the newest work run only: once the
	// ticket runs again, the old refusals are history.
	const lastWork = $derived(
		runs.filter((r) => r.kind === 'work').sort((a, b) => new Date(b.startedAt) - new Date(a.startedAt))[0]
	);
	// A run job does its checks before its run exists — about the agent, the
	// repo and whether the harness can answer at all. Until the run appears
	// that time shows here, and a failure in it is said here, not only in a
	// truncated line in the header.
	// The card's title already says it could not start; the host's
	// "not starting KEY:" and "<harness> cannot run <model>:" prefixes only
	// push the reason further down.
	const startError = (e) =>
		(e || 'Failed').replace(/^not starting [A-Z0-9]+-\d+: /, '').replace(/^\w+ cannot run "[^"]*": /, '');
	const workRun = $derived(
		work && runs.find((r) => r.kind === 'work' && new Date(r.startedAt) >= new Date(work.claimedAt || work.createdAt))
	);
	const workPending = $derived(work && !workRun && (work.status === 'queued' || work.status === 'claimed'));
	const workFailed = $derived(work && !workRun && work.status === 'failed');
	const thinking = $derived(turn && (turn.status === 'queued' || turn.status === 'claimed'));
	// Once a run exists it speaks for itself in the thread; the line below
	// only covers the gap before it starts.
	const runLive = $derived(runs.some((r) => r.status === 'running'));

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
		<span class="rh">{theAgent ? 'Conversation' : 'Comments'}</span>
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
				<div class="st-t">No comments yet</div>
				<p>Leave a note below. To have an agent pick this up, choose one under Agent, or ask one from here.</p>
			{/if}
		</div>
	{/if}

	{#each timeline as item (item.kind + (item.c?.id || item.r?.id || item.x?.id || item.j?.id))}
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
			{#key item.r.id}<RunThread run={item.r} />{/key}
			{#if item.r.id === lastWork?.id && item.r.agentId === ticketAgentId && item.r.status !== 'running' && item.r.deniedTools?.length}
				<PermissionCard run={item.r} {issue} />
			{/if}
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
		{:else if item.kind === 'startfail'}
			<div class="startfail">
				<div class="sf-t"><CircleAlert size={14} strokeWidth={2} />{agentById(item.j.agentId)?.name || 'The agent'} could not start{item.j.host ? ` on ${item.j.host}` : ''}</div>
				<div class="sf-e">{startError(item.j.error)}</div>
				<span class="when">{rel(item.at)}</span>
			</div>
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

	{#if !pinned}
		<button class="jump" onclick={() => ((pinned = true), toBottom(true))} aria-label="Jump to the latest">
			<ArrowDown size={13} strokeWidth={2.4} />Latest
		</button>
	{/if}

	{#if workPending && !runLive}
		<div class="thinking">
			<LoaderCircle size={14} class="spin" />
			{#if work.status === 'queued'}Waiting for a host to pick up the run…
			{:else}{agentById(work.agentId)?.name || 'The agent'} is getting ready on {work.host} — checking it can run…{/if}
		</div>
	{/if}
	{#if thinking && !runLive}
		<div class="thinking">
			<LoaderCircle size={14} class="spin" />
			{#if turn.status === 'queued'}Waiting for a host to pick up {agentById(turn.agentId)?.name || 'the agent'}…
			{:else}Starting {agentById(turn.agentId)?.name || 'the agent'} on {turn.host}…{/if}
		</div>
	{:else if !thinking && turn?.status === 'failed'}
		<div class="failed">{agentById(turn.agentId)?.name || 'The agent'} could not reply: {turn.error}</div>
	{/if}

	<div class="composer">
		<textarea bind:value={draft} onkeydown={onKey} rows="3"
			placeholder={theAgent ? `Message ${theAgent.name}… (⌘↵ to ask)` : 'Leave a comment…'}></textarea>
		<div class="cf">
			{#if !ticketAgentId}
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
		position: relative;
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
	.note {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		font-size: 13px;
		color: var(--text-faint);
	}
	.startfail {
		border: 1px solid color-mix(in srgb, #d03b3b 40%, var(--border));
		border-radius: 10px;
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 13px;
	}
	.sf-t {
		display: flex;
		align-items: center;
		gap: 7px;
		color: #f87171;
	}
	.sf-e {
		color: var(--text-dim);
		font-size: 12.5px;
		line-height: 1.45;
		word-break: break-word;
	}
	.startfail .when {
		align-self: flex-end;
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
	.jump {
		position: absolute;
		left: 50%;
		bottom: 150px;
		transform: translateX(-50%);
		display: inline-flex;
		align-items: center;
		gap: 5px;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 999px;
		padding: 4px 12px;
		font-size: 12px;
		color: var(--text);
		box-shadow: var(--shadow);
		z-index: 5;
	}
	.jump:hover {
		background: var(--bg-hover);
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
