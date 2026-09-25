<script>
	// Running: every ticket an agent is working on or waiting to, what it is
	// doing right now, and what finished lately. Kept live from job and run
	// events; nothing here starts work.
	import { onMount } from 'svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import { api } from '$lib/api.js';
	import { activeJobs, agents, issues } from '$lib/store.js';
	import { onLive } from '$lib/ui.js';
	import { rel, duration } from '$lib/format.js';
	import { Activity, LoaderCircle, Check, X, Bot } from '@lucide/svelte';

	const KIND = { run_ticket: 'Run', chat: 'Conversation', verify: 'Verify', finish: 'Finish' };
	const DOING = { run_ticket: 'Working', chat: 'Thinking', verify: 'Verifying', finish: 'Finishing' };

	let finished = $state([]);
	let now = $state(Date.now()); // ticks so "working for" keeps counting
	onMount(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});
	let lastLine = $state({}); // issue id -> latest transcript line of its running run
	let runIssue = {}; // run id -> issue id, for live lines

	const agentName = (id) => $agents.find((a) => a.id === id)?.name || 'Agent';
	const titleOf = (j) => $issues.find((i) => i.id === j.issueId)?.title || '';

	const working = $derived($activeJobs.filter((j) => j.status === 'claimed'));
	const queued = $derived($activeJobs.filter((j) => j.status === 'queued'));

	// What a working run last said or did, one line.
	const readable = (l) => (l.startsWith('→ ') ? l.slice(2) : l.startsWith('· ') || l.startsWith('← ') ? '' : l);
	async function watch(j) {
		const runs = (await api.issueRuns(j.issueId).catch(() => [])) || [];
		const r = runs.find((x) => x.status === 'running');
		if (!r) return;
		runIssue[r.id] = j.issueId;
		const evs = (await api.get(`/runs/${r.id}/events`).catch(() => [])) || [];
		for (let i = evs.length - 1; i >= 0; i--) {
			const t = readable(evs[i].text);
			if (t) {
				lastLine = { ...lastLine, [j.issueId]: t };
				break;
			}
		}
	}
	let watched = new Set();
	$effect(() => {
		for (const j of working) {
			if (watched.has(j.id)) continue;
			watched.add(j.id);
			watch(j);
		}
	});

	async function loadFinished() {
		const all = (await api.jobs({ limit: 60 }).catch(() => [])) || [];
		finished = all
			.filter((j) => j.issueId && j.kind !== 'test_env' && (j.status === 'succeeded' || j.status === 'failed'))
			.slice(0, 15);
	}
	onMount(loadFinished);
	onMount(() =>
		onLive((ev) => {
			if (ev?.job && ev.job.issueId && (ev.job.status === 'succeeded' || ev.job.status === 'failed')) loadFinished();
			if (ev?.type === 'run.started' || ev?.run) {
				const j = $activeJobs.find((x) => x.issueId === ev.run?.issueId);
				if (j) watch(j);
			}
			if (ev?.type === 'run.events' && runIssue[ev.runId]) {
				const lines = (ev.lines || []).map(readable).filter(Boolean);
				if (lines.length) lastLine = { ...lastLine, [runIssue[ev.runId]]: lines[lines.length - 1] };
			}
		})
	);
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Running' }]} />
	<div class="body">
		<section>
			<h2><span class="dot live"></span>Working <span class="n">{working.length}</span></h2>
			{#each working as j (j.id)}
				<a class="row" href="/issue/{j.issueKey}">
					<span class="ic spin"><LoaderCircle size={15} strokeWidth={2.2} /></span>
					<span class="main">
						<span class="t"><span class="key">{j.issueKey}</span>{titleOf(j)}</span>
						<span class="line">{lastLine[j.issueId] || `${DOING[j.kind] || 'Working'}…`}</span>
					</span>
					<span class="who"><Bot size={13} strokeWidth={2} />{agentName(j.agentId)}</span>
					<span class="kind">{KIND[j.kind] || j.kind}</span>
					<span class="when" title={j.host ? `On ${j.host}` : ''}>{duration(j.claimedAt || j.createdAt, new Date(now).toISOString())}</span>
				</a>
			{:else}
				<div class="empty">No agent is working right now.</div>
			{/each}
		</section>

		<section>
			<h2><span class="dot"></span>Queued <span class="n">{queued.length}</span></h2>
			{#each queued as j (j.id)}
				<a class="row" href="/issue/{j.issueKey}">
					<span class="ic"><LoaderCircle size={15} strokeWidth={2.2} /></span>
					<span class="main">
						<span class="t"><span class="key">{j.issueKey}</span>{titleOf(j)}</span>
						<span class="line">Waiting for a host to pick it up</span>
					</span>
					<span class="who"><Bot size={13} strokeWidth={2} />{agentName(j.agentId)}</span>
					<span class="kind">{KIND[j.kind] || j.kind}</span>
					<span class="when">{rel(j.createdAt)}</span>
				</a>
			{:else}
				<div class="empty">Nothing waiting.</div>
			{/each}
		</section>

		<section>
			<h2>Finished lately</h2>
			{#each finished as j (j.id)}
				<a class="row" href="/issue/{j.issueKey}">
					<span class="ic {j.status}">
						{#if j.status === 'succeeded'}<Check size={15} strokeWidth={2.4} />{:else}<X size={15} strokeWidth={2.4} />{/if}
					</span>
					<span class="main">
						<span class="t"><span class="key">{j.issueKey}</span>{titleOf(j)}</span>
						{#if j.status === 'failed' && j.error}<span class="line err">{j.error}</span>{/if}
					</span>
					<span class="who"><Bot size={13} strokeWidth={2} />{agentName(j.agentId)}</span>
					<span class="kind">{KIND[j.kind] || j.kind}</span>
					<span class="when">{rel(j.finishedAt || j.createdAt)}</span>
				</a>
			{:else}
				<div class="empty"><Activity size={14} strokeWidth={2} /> Nothing yet.</div>
			{/each}
		</section>
	</div>
</div>

<style>
	.pg {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.body {
		flex: 1;
		overflow-y: auto;
		padding: 16px clamp(16px, 4vw, 40px) 48px;
		display: flex;
		flex-direction: column;
		gap: 22px;
		max-width: 1000px;
	}
	h2 {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 0 0 6px;
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-dim);
	}
	.n {
		color: var(--text-faint);
		font-weight: 500;
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--text-faint);
	}
	.dot.live {
		background: var(--st-progress);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--st-progress) 25%, transparent);
	}
	.row {
		display: grid;
		grid-template-columns: 22px minmax(0, 1fr) minmax(0, 160px) 96px 120px;
		align-items: center;
		gap: 12px;
		padding: 9px 10px;
		border-radius: 8px;
		color: var(--text);
		text-decoration: none;
		border-bottom: 1px solid var(--border);
	}
	.row:hover {
		background: var(--bg-hover);
	}
	.ic {
		display: inline-flex;
		color: var(--text-faint);
	}
	.ic.spin {
		color: var(--st-progress);
	}
	.ic.spin :global(svg) {
		animation: rp-spin 1.2s linear infinite;
	}
	.ic.succeeded {
		color: #4ade80;
	}
	.ic.failed {
		color: #f87171;
	}
	@keyframes rp-spin {
		to {
			transform: rotate(360deg);
		}
	}
	.main {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.t {
		font-size: 13.5px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.key {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--text-faint);
		margin-right: 8px;
	}
	.line {
		font-size: 12px;
		color: var(--text-faint);
		font-family: var(--mono);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.line.err {
		color: #fca5a5;
		font-family: inherit;
	}
	.who {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12.5px;
		color: var(--text-dim);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	.kind,
	.when {
		font-size: 12px;
		color: var(--text-faint);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.when {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.empty {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 10px;
		font-size: 13px;
		color: var(--text-faint);
	}
	@media (max-width: 720px) {
		.row {
			grid-template-columns: 22px minmax(0, 1fr) auto;
		}
		.kind,
		.when {
			display: none;
		}
	}
</style>
