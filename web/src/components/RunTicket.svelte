<script>
	// The ticket's one action, by where it stands. Before Ready it is "Start
	// task": the agent reads the ticket and talks it through in the
	// conversation. From Ready it is "Run": queue it for its agent, then follow
	// the job until a runner host has worked it. Done and canceled tickets have
	// neither. You decide when an agent runs — nothing here starts on its own.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents, states, priorityAgents, taskAgentId } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import { Play, LoaderCircle, Check, X } from '@lucide/svelte';

	let { issue } = $props();
	let job = $state(null);

	const agent = $derived($agents.find((a) => a.id === taskAgentId(issue, $priorityAgents)));
	const busy = $derived(job && (job.status === 'queued' || job.status === 'claimed'));

	const state = $derived($states.find((s) => s.id === issue.stateId));
	const ready = $derived($states.find((s) => s.name === 'Ready'));
	const closed = $derived(state && (state.category === 'completed' || state.category === 'canceled'));
	const aligning = $derived(state && ready && state.position < ready.position);
	let starting = $state(false);

	onMount(async () => {
		const list = await api.jobs({ issue: issue.key, kind: 'run_ticket', limit: 1 }).catch(() => []);
		job = list?.[0] || null;
	});
	onMount(() =>
		onLive((ev) => {
			if (ev?.job && ev.job.kind === 'run_ticket' && ev.job.issueId === issue.id) job = ev.job;
		})
	);

	async function run() {
		try {
			job = await api.post(`/issues/${issue.key}/run`, {});
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function start() {
		starting = true;
		try {
			await api.post(`/issues/${issue.key}/ask`, { agent: agent.id, start: true });
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			starting = false;
		}
	}
	const verdict = $derived(job?.result?.status);
</script>

{#if agent && aligning}
	<button class="btn primary sm" onclick={start} disabled={starting || agent.status === 'paused'}>
		<Play size={13} strokeWidth={2.4} />Start task
	</button>
{:else if agent && !closed}
	<div class="rt">
		{#if job}
			<span class="st {job.status}" title={job.error || ''}>
				{#if job.status === 'queued'}<LoaderCircle size={13} class="spin" />Waiting for a host
				{:else if job.status === 'claimed'}<LoaderCircle size={13} class="spin" />{agent.name} is working on {job.host}
				{:else if job.status === 'succeeded'}<Check size={13} strokeWidth={2.4} />Handed back{verdict ? ` · ${verdict}` : ''} · {rel(job.finishedAt)}
				{:else}<X size={13} strokeWidth={2.4} />{job.error ? job.error.slice(0, 80) : 'Failed'}{/if}
			</span>
		{/if}
		<button class="btn primary sm" onclick={run} disabled={busy || agent.status === 'paused'}>
			<Play size={13} strokeWidth={2.4} />Run with {agent.name}
		</button>
	</div>
{/if}

<style>
	.rt {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}
	.st {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12.5px;
		color: var(--text-dim);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 360px;
	}
	.st.succeeded {
		color: #86efac;
	}
	.st.failed {
		color: #fca5a5;
	}
	.st :global(.spin) {
		animation: spin 1s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (max-width: 720px) {
		.st {
			display: none;
		}
	}
</style>
