<script>
	// "Run" on a ticket: queue it for its agent, then follow the job until a
	// runner host has worked it. You decide when an agent runs — nothing here
	// starts on its own.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import { Play, LoaderCircle, Check, X } from '@lucide/svelte';

	let { issue } = $props();
	let job = $state(null);

	const agent = $derived($agents.find((a) => a.id === issue.agentId));
	const busy = $derived(job && (job.status === 'queued' || job.status === 'claimed'));

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
	const verdict = $derived(job?.result?.status);
</script>

{#if agent}
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
