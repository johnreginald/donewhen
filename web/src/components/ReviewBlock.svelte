<script>
	// Reviewing a handed-back ticket from the dashboard: the diff the agent's
	// run produced (or the worktree as the last Verify saw it), and Verify /
	// Finish — `orchestrator verify` and `finish`, run by the host that keeps
	// the worktree. Finish is gated by the criteria, never by this button.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import { ShieldCheck, Flag, LoaderCircle, Check, X, GitBranch } from '@lucide/svelte';

	let { issue, stateName } = $props();

	let run = $state(null); // latest finished work run, with its diff
	let job = $state(null); // latest verify or finish job
	let loaded = $state(false);

	async function load() {
		const runs = (await api.issueRuns(issue.id).catch(() => [])) || [];
		const last = runs.find((r) => (r.kind || 'work') === 'work' && r.finishedAt && r.verdict);
		run = last ? await api.run(last.id).catch(() => last) : null;
		const [v, f] = await Promise.all([
			api.jobs({ issue: issue.key, kind: 'verify', limit: 1 }).catch(() => []),
			api.jobs({ issue: issue.key, kind: 'finish', limit: 1 }).catch(() => [])
		]);
		job = [v?.[0], f?.[0]].filter(Boolean).sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt))[0] || null;
		loaded = true;
	}
	onMount(() => {
		load();
		return onLive((ev) => {
			if (ev.issueId !== issue.id) return;
			if (ev.job && (ev.job.kind === 'verify' || ev.job.kind === 'finish')) job = ev.job;
			if (ev.type === 'run.finished') load();
		});
	});

	const busy = $derived(job && (job.status === 'queued' || job.status === 'claimed'));
	const diff = $derived(job?.kind === 'verify' && job.result?.diff ? job.result.diff : run?.diff || '');
	const verdict = $derived(job?.result?.verdict?.status || run?.verdict);
	const done = $derived(['In Review', 'Done', 'Canceled'].includes(stateName));

	async function go(kind) {
		try {
			job = await api.post(`/issues/${issue.key}/${kind}`, {});
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	function cls(line) {
		if (line.startsWith('+++') || line.startsWith('---') || line.startsWith('diff ') || line.startsWith('index ')) return 'h';
		if (line.startsWith('@@')) return 'at';
		if (line.startsWith('+')) return 'add';
		if (line.startsWith('-')) return 'del';
		return '';
	}
</script>

{#if loaded && run && !done}
	<section class="rv">
		<header>
			<span class="t">Review</span>
			<span class="v {verdict}">criteria {verdict || '—'}</span>
			{#if issue.gitBranch}<span class="br"><GitBranch size={13} />{issue.gitBranch}</span>{/if}
			<span class="sp"></span>
			<button class="btn sm" onclick={() => go('verify')} disabled={busy}><ShieldCheck size={13} />Verify</button>
			<button class="btn primary sm" onclick={() => go('finish')} disabled={busy}><Flag size={13} />Finish → In Review</button>
		</header>
		{#if job}
			<div class="job {job.status}">
				{#if busy}<LoaderCircle size={13} class="spin" />{job.kind === 'finish' ? 'Finishing' : 'Verifying'} on {job.host || 'a host'}…
				{:else if job.status === 'succeeded'}<Check size={13} strokeWidth={2.4} />
					{job.kind === 'finish' ? 'Finished: moved to In Review' : `Verified: criteria ${job.result?.verdict?.status}`} · {rel(job.finishedAt)}
				{:else}<X size={13} strokeWidth={2.4} />{job.error}{/if}
			</div>
		{/if}
		<p class="hint">Fix anything you want in the worktree on your Mac, then Verify. Finish commits your fixes, links the commit, saves the record and moves the ticket to In Review — only if every gating criterion passes.</p>
		{#if diff}
			<pre class="diff">{#each diff.split('\n') as line}<span class={cls(line)}>{line}</span>
{/each}</pre>
		{:else}
			<div class="faint">No diff recorded for this run.</div>
		{/if}
	</section>
{/if}

<style>
	.rv {
		border: 1px solid var(--border-strong);
		border-radius: 12px;
		background: var(--bg-elev);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	header {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
	}
	.t {
		font-weight: 600;
		font-size: 13.5px;
	}
	.v {
		font-size: 12px;
		color: var(--text-dim);
	}
	.v.passed {
		color: #86efac;
	}
	.v.failed {
		color: #fca5a5;
	}
	.br {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--text-faint);
	}
	.sp {
		flex: 1;
	}
	.job {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12.5px;
		color: var(--text-dim);
	}
	.job.succeeded {
		color: #86efac;
	}
	.job.failed {
		color: #fca5a5;
	}
	.job :global(.spin) {
		animation: spin 1s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.hint {
		margin: 0;
		font-size: 12px;
		color: var(--text-faint);
	}
	.diff {
		margin: 0;
		max-height: 480px;
		overflow: auto;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 0;
		font-family: var(--mono);
		font-size: 12px;
		line-height: 1.5;
		color: var(--text-dim);
	}
	.diff span {
		display: block;
		padding: 0 12px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.diff .add {
		background: color-mix(in srgb, #0ca30c 14%, transparent);
		color: #bbf7d0;
	}
	.diff .del {
		background: color-mix(in srgb, #d03b3b 14%, transparent);
		color: #fecaca;
	}
	.diff .at {
		color: #7dd3fc;
	}
	.diff .h {
		color: var(--text);
		font-weight: 600;
	}
	.faint {
		font-size: 12.5px;
		color: var(--text-faint);
	}
</style>
