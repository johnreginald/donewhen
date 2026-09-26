<script>
	// Run a whole epic: every Ready ticket in it that nothing blocks starts
	// now, and each of the rest starts on its own as soon as its blockers are
	// Done — until the epic is stopped.
	import { api } from '$lib/api.js';
	import { projects, loadMeta } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { Play, Square } from '@lucide/svelte';

	let { epicId } = $props();
	let busy = $state(false);
	const epic = $derived($projects.find((p) => p.id === epicId));

	async function run() {
		busy = true;
		try {
			const r = await api.post(`/projects/${epicId}/run`, {});
			const parts = [];
			if (r.queued.length) parts.push(`started ${r.queued.length}`);
			if (r.blocked.length) parts.push(`${r.blocked.length} waiting on blockers`);
			if (r.noAgent.length) parts.push(`${r.noAgent.length} with no agent (${r.noAgent.join(', ')})`);
			if (r.noChecks?.length) parts.push(`${r.noChecks.length} with no done-when (${r.noChecks.join(', ')})`);
			showToast(`${epic?.name || 'Epic'} running — ${parts.join(' · ') || 'no Ready tickets yet'}`);
			await loadMeta();
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			busy = false;
		}
	}
	async function stop() {
		busy = true;
		try {
			await api.post(`/projects/${epicId}/stop`, {});
			showToast(`${epic?.name || 'Epic'} stopped — work already started carries on`);
			await loadMeta();
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			busy = false;
		}
	}
</script>

{#if epic}
	{#if epic.autorun}
		<span class="running" title="Each Ready ticket starts once nothing blocks it"><span class="pulse"></span>Epic running</span>
		<button class="btn sm" onclick={stop} disabled={busy}><Square size={12} strokeWidth={2.4} />Stop epic</button>
	{:else}
		<button class="btn primary sm" onclick={run} disabled={busy}
			title="Start every Ready ticket in this epic that nothing blocks; the rest start as their blockers are Done">
			<Play size={13} strokeWidth={2.4} />Run epic
		</button>
	{/if}
{/if}

<style>
	.running {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		font-size: 12.5px;
		color: var(--st-progress);
	}
	.pulse {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--st-progress);
		animation: er-pulse 1.4s ease-in-out infinite;
	}
	@keyframes er-pulse {
		50% {
			opacity: 0.3;
		}
	}
</style>
