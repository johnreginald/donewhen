<script>
	import { onMount } from 'svelte';
	// One run in full: the page Paperclip opens from "Inspect run".
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel, tokens, duration, usd } from '$lib/format.js';
	import { Check, X, LoaderCircle, CircleSlash } from '@lucide/svelte';

	let run = $state(null);
	let missing = $state(false);

	$effect(() => {
		const id = $page.params.id;
		if (id) load(id);
	});

	async function load(id) {
		try {
			run = await api.run(id);
			missing = false;
		} catch (e) {
			if (e.status === 404) missing = true;
			else showToast('Load failed: ' + e.message, 'error');
		}
	}

	// Finishing while open fills in the transcript and totals.
	onMount(() =>
		onLive((ev) => {
			if (ev?.run && run && ev.run.id === run.id && ev.type === 'run.finished') load(run.id);
		})
	);

	const fmt = (s) => (s ? new Date(s).toLocaleString() : '—');
	const STATUS = { running: 'running', queued: 'queued', succeeded: 'succeeded', failed: 'failed', aborted: 'stopped' };
</script>

{#if run}
	<div class="page">
		<div class="top">
			<button class="crumb" onclick={() => history.back()}>← Back</button>
			<span class="sep">/</span>
			<span class="crumb-static">Runs</span>
			<span class="sep">/</span>
			<span class="rid">{run.id.slice(0, 8)}</span>
		</div>

		<div class="content">
			<header class="head">
				<span class="pill {run.status}">
					{#if run.status === 'running' || run.status === 'queued'}
						<LoaderCircle size={13} strokeWidth={2.2} class="spin" />
					{:else if run.status === 'succeeded'}
						<Check size={13} strokeWidth={2.4} />
					{:else if run.status === 'aborted'}
						<CircleSlash size={13} strokeWidth={2.2} />
					{:else}
						<X size={13} strokeWidth={2.4} />
					{/if}
					{STATUS[run.status] || run.status}
				</span>
				<span class="chip mono">{run.runner}</span>
				{#if run.model}<span class="chip mono">{run.model}</span>{/if}
				{#if run.verdict}<span class="verdict {run.verdict}">criteria {run.verdict}</span>{/if}
				<span class="when">{rel(run.startedAt)}</span>
			</header>

			{#if run.issueKey}
				<button class="ticket" onclick={() => goto('/issue/' + run.issueKey)}>
					<span class="tk">{run.issueKey}</span>
					<span class="tt">{run.issueTitle}</span>
					{#if run.attempt > 1}<span class="faint">attempt {run.attempt}</span>{/if}
				</button>
			{/if}

			<div class="grid">
				<section class="card">
					<h3>Timing</h3>
					<dl>
						<dt>Started</dt><dd>{fmt(run.startedAt)}</dd>
						<dt>Finished</dt><dd>{fmt(run.finishedAt)}</dd>
						<dt>Duration</dt><dd>{duration(run.startedAt, run.finishedAt)}</dd>
						{#if run.exitCode !== null && run.exitCode !== undefined}<dt>Exit</dt><dd class="mono">{run.exitCode}</dd>{/if}
						{#if run.host}<dt>Host</dt><dd>{run.host}</dd>{/if}
					</dl>
				</section>
				<section class="card">
					<h3>Usage</h3>
					<dl>
						<dt>Billing</dt>
						<dd>
							{#if run.billing === 'subscription'}Subscription
							{:else if run.billing === 'api'}API
							{:else}Unknown{/if}
						</dd>
						<dt>Cost</dt>
						<dd>
							{usd(run.costUsd)}
							{#if run.notionalCostUsd}<span class="faint"> · {usd(run.notionalCostUsd)} at list price</span>{/if}
						</dd>
						<dt>Tokens</dt><dd>{tokens(run.tokens?.total)}</dd>
						<dt class="ind">Input</dt><dd>{tokens(run.tokens?.input)}</dd>
						<dt class="ind">Cache read</dt><dd>{tokens(run.tokens?.cacheRead)}</dd>
						<dt class="ind">Cache write</dt><dd>{tokens(run.tokens?.cacheCreation)}</dd>
						<dt class="ind">Output</dt><dd>{tokens(run.tokens?.output)}</dd>
					</dl>
				</section>
			</div>

			{#if run.sessionId}
				<section class="card">
					<h3>Session</h3>
					<div class="mono dim">{run.sessionId}</div>
				</section>
			{/if}

			{#if run.agentError}
				<section class="card">
					<h3>Error</h3>
					<div class="err">{run.agentError}</div>
				</section>
			{/if}

			{#if run.deniedTools?.length}
				<section class="card">
					<h3>Refused tool calls <span class="faint">{run.deniedTools.length}</span></h3>
					<ul class="denied">
						{#each run.deniedTools as d}<li class="mono">{d}</li>{/each}
					</ul>
				</section>
			{/if}

			<section class="card">
				<h3>Transcript <span class="faint">end of run · redacted</span></h3>
				{#if run.logTail}
					<pre class="log">{run.logTail}</pre>
				{:else if !run.finishedAt}
					<div class="faint">Still running — the transcript arrives when it finishes.</div>
				{:else}
					<div class="faint">No transcript was recorded.</div>
				{/if}
			</section>
		</div>
	</div>
{:else if missing}
	<div class="empty">Run not found.</div>
{/if}

<style>
	.page {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.top {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 12px 20px;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
		font-size: 13.5px;
	}
	.crumb {
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 3px 6px;
		border-radius: 6px;
	}
	.crumb:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.crumb-static,
	.sep {
		color: var(--text-faint);
	}
	.rid {
		font-family: var(--mono);
		font-size: 12.5px;
		color: var(--text-faint);
	}
	.content {
		overflow-y: auto;
		padding: 24px clamp(16px, 4vw, 48px);
		display: flex;
		flex-direction: column;
		gap: 14px;
		max-width: 920px;
		width: 100%;
		margin: 0 auto;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12.5px;
		padding: 3px 10px;
		border-radius: 999px;
		border: 1px solid var(--border-strong);
		color: var(--text-dim);
	}
	.pill.succeeded {
		color: var(--st-done);
		border-color: color-mix(in srgb, var(--st-done) 45%, transparent);
	}
	.pill.failed {
		color: #f87171;
		border-color: color-mix(in srgb, #f87171 45%, transparent);
	}
	.pill.aborted {
		color: var(--st-review);
	}
	.pill.running,
	.pill.queued {
		color: var(--st-progress);
	}
	.pill :global(.spin) {
		animation: spin 1s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.chip {
		font-size: 12px;
		padding: 2px 8px;
		border-radius: 6px;
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		color: var(--text-dim);
	}
	.verdict {
		font-size: 12px;
		color: var(--text-dim);
	}
	.verdict.passed {
		color: var(--st-done);
	}
	.verdict.failed {
		color: #f87171;
	}
	.when {
		margin-left: auto;
		color: var(--text-faint);
		font-size: 12.5px;
	}
	.ticket {
		display: flex;
		align-items: center;
		gap: 10px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 10px 12px;
		color: var(--text);
		text-align: left;
		font-size: 13.5px;
	}
	.ticket:hover {
		border-color: var(--border-strong);
	}
	.tk {
		font-family: var(--mono);
		font-size: 12.5px;
		color: var(--text-faint);
	}
	.tt {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
		gap: 14px;
	}
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 14px 16px;
		min-width: 0;
	}
	h3 {
		margin: 0 0 10px;
		font-size: 12px;
		font-weight: 500;
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	dl {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 5px 16px;
		margin: 0;
		font-size: 13px;
	}
	dt {
		color: var(--text-faint);
	}
	dt.ind {
		padding-left: 12px;
	}
	dd {
		margin: 0;
		color: var(--text-dim);
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.mono {
		font-family: var(--mono);
		font-size: 12px;
	}
	.dim {
		color: var(--text-dim);
		overflow-wrap: anywhere;
	}
	.faint {
		color: var(--text-faint);
		font-weight: 400;
		text-transform: none;
		letter-spacing: 0;
	}
	.err {
		color: #fca5a5;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-size: 13px;
	}
	.denied {
		margin: 0;
		padding-left: 18px;
		color: var(--text-dim);
	}
	.log {
		margin: 0;
		max-height: 60vh;
		overflow: auto;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 10px 12px;
		font-family: var(--mono);
		font-size: 12px;
		line-height: 1.6;
		color: var(--text-dim);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.empty {
		padding: 40px;
		color: var(--text-faint);
	}
</style>
