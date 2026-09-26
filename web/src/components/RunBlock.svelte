<script>
	// One agent attempt on a ticket: a collapsed "Worked" line, as Paperclip
	// shows it, that opens onto what the run cost, what it was refused, and how
	// its transcript ended.
	import { api } from '$lib/api.js';
	import { rel, tokens, duration, usd } from '$lib/format.js';
	import { Check, X, LoaderCircle, CircleSlash, ChevronRight, ExternalLink } from '@lucide/svelte';
	import LiveTranscript from './LiveTranscript.svelte';

	let { run } = $props();
	let open = $state(false);
	// A run still going counts up live.
	let now = $state(Date.now());
	$effect(() => {
		if (run.finishedAt) return;
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});
	let full = $state(null); // the run with its log, fetched on first open

	const LABEL = { running: 'Working', queued: 'Queued', succeeded: 'Worked', failed: 'Failed', aborted: 'Stopped' };

	async function toggle() {
		open = !open;
		if (open && !full && run.finishedAt) {
			full = await api.run(run.id).catch(() => null);
		}
	}
	// A run that finishes while open is re-read so its log appears.
	$effect(() => {
		if (open && run.finishedAt && full && !full.finishedAt) {
			api.run(run.id).then((r) => (full = r)).catch(() => {});
		}
	});
</script>

<div class="run" class:open>
	<button class="row" onclick={toggle} aria-expanded={open}>
		<span class="st {run.status}">
			{#if run.status === 'running' || run.status === 'queued'}
				<LoaderCircle size={14} strokeWidth={2.2} class="spin" />
			{:else if run.status === 'succeeded'}
				<Check size={14} strokeWidth={2.4} />
			{:else if run.status === 'aborted'}
				<CircleSlash size={14} strokeWidth={2.2} />
			{:else}
				<X size={14} strokeWidth={2.4} />
			{/if}
		</span>
		<span class="lbl">{LABEL[run.status] || run.status}</span>
		<span class="meta">
			{run.runner}{#if run.model} · {run.model}{/if}
			{#if run.attempt > 1} · attempt {run.attempt}{/if}
			{#if run.tokens?.total} · {tokens(run.tokens.total)} tok{/if}
			· {duration(run.startedAt, run.finishedAt || new Date(now).toISOString())}
		</span>
		{#if run.verdict}
			<span class="verdict {run.verdict}">{run.verdict}</span>
		{/if}
		<span class="when">{rel(run.startedAt)}</span>
		<span class="chev"><ChevronRight size={14} strokeWidth={2} /></span>
	</button>

	{#if open}
		<div class="body">
			<dl class="facts">
				<dt>Billing</dt>
				<dd>
					{#if run.billing === 'subscription'}
						Subscription{#if run.notionalCostUsd} · {usd(run.notionalCostUsd)} at list price{/if}
					{:else if run.billing === 'api'}
						API · {usd(run.costUsd)}
					{:else}
						Unknown{#if run.costUsd} · {usd(run.costUsd)}{/if}
					{/if}
				</dd>
				{#if run.tokens?.total}
					<dt>Tokens</dt>
					<dd>
						{tokens(run.tokens.input)} in · {tokens(run.tokens.cacheRead)} cache read ·
						{tokens(run.tokens.cacheCreation)} cache write · {tokens(run.tokens.output)} out
					</dd>
				{/if}
				{#if run.sessionId}
					<dt>Session</dt>
					<dd class="mono">{run.sessionId}</dd>
				{/if}
				{#if run.host}
					<dt>Host</dt>
					<dd>{run.host}</dd>
				{/if}
			</dl>

			{#if run.agentError}
				<div class="err">{run.agentError}</div>
			{/if}

			{#if run.deniedTools?.length}
				<div class="sub">Refused ({run.deniedTools.length})</div>
				<ul class="denied">
					{#each run.deniedTools as d}<li class="mono">{d}</li>{/each}
				</ul>
			{/if}

			<div class="sub">{run.finishedAt ? 'Transcript' : 'Live transcript'}</div>
			<LiveTranscript runId={run.id} fallback={full?.logTail || ''} />

			<a class="open-run" href="/runs/{run.id}">Open run <ExternalLink size={12} strokeWidth={2} /></a>
		</div>
	{/if}
</div>

<style>
	.run {
		border-radius: var(--radius);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text-dim);
		font-size: 13px;
		padding: 6px 8px;
		border-radius: var(--radius);
		text-align: left;
	}
	.row:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.st {
		display: inline-flex;
		flex-shrink: 0;
	}
	.st.succeeded {
		color: var(--st-done);
	}
	.st.failed {
		color: #f87171;
	}
	.st.aborted {
		color: var(--st-review);
	}
	.st.running,
	.st.queued {
		color: var(--st-progress);
	}
	.st :global(.spin) {
		animation: spin 1s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.lbl {
		color: var(--text);
		font-weight: 500;
	}
	.meta {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.verdict {
		font-size: 11px;
		padding: 1px 7px;
		border-radius: 999px;
		border: 1px solid var(--border-strong);
		flex-shrink: 0;
	}
	.verdict.passed {
		color: var(--st-done);
		border-color: color-mix(in srgb, var(--st-done) 40%, transparent);
	}
	.verdict.failed {
		color: #f87171;
		border-color: color-mix(in srgb, #f87171 40%, transparent);
	}
	.verdict.blocked {
		color: var(--st-review);
		border-color: color-mix(in srgb, var(--st-review) 40%, transparent);
	}
	.when {
		margin-left: auto;
		color: var(--text-faint);
		flex-shrink: 0;
	}
	.chev {
		display: inline-flex;
		color: var(--text-faint);
		transition: transform 0.15s;
	}
	.open .chev {
		transform: rotate(90deg);
	}
	.body {
		margin: 2px 0 8px 30px;
		padding: 10px 12px;
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--bg-elev);
		display: flex;
		flex-direction: column;
		gap: 8px;
		font-size: 12.5px;
	}
	.facts {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 4px 14px;
		margin: 0;
	}
	dt {
		color: var(--text-faint);
	}
	dd {
		margin: 0;
		color: var(--text-dim);
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.mono {
		font-family: var(--mono);
		font-size: 11.5px;
	}
	.err {
		color: #fca5a5;
		background: color-mix(in srgb, #f87171 10%, transparent);
		border-radius: 6px;
		padding: 6px 8px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.sub {
		color: var(--text-faint);
		font-size: 11.5px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.denied {
		margin: 0;
		padding-left: 16px;
		color: var(--text-dim);
	}
	.faint {
		color: var(--text-faint);
	}
	.open-run {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		color: var(--accent2);
		width: fit-content;
	}
	.open-run:hover {
		text-decoration: underline;
	}
</style>
