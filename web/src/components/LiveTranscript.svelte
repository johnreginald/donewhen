<script>
	// A run's transcript as it happens: the lines the machine running it has
	// posted so far, then each new batch as it arrives.
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api.js';
	import { onLive } from '$lib/ui.js';

	let { runId, fallback = '', maxHeight = '320px' } = $props();
	let lines = $state([]);
	let last = 0;
	let box = $state(null);
	let stick = true; // follow the end unless the reader scrolled up

	async function append(newLines, seq) {
		// Ignore a batch already seen (the initial load and a live batch can overlap).
		const fresh = newLines.filter((_, i) => seq + i > last);
		if (!fresh.length) return;
		lines = [...lines, ...fresh];
		last = seq + newLines.length - 1;
		if (stick) {
			await tick();
			box && (box.scrollTop = box.scrollHeight);
		}
	}

	onMount(() => {
		api.get(`/runs/${runId}/events`).then((evs) => {
			if (evs?.length) append(evs.map((e) => e.text), evs[0].seq);
		});
		return onLive((ev) => {
			if (ev.type === 'run.events' && ev.runId === runId) append(ev.lines || [], ev.seq || 1);
		});
	});
	const onScroll = () => box && (stick = box.scrollTop + box.clientHeight >= box.scrollHeight - 8);
</script>

{#if lines.length}
	<pre class="log" bind:this={box} onscroll={onScroll} style:max-height={maxHeight}>{lines.join('\n')}</pre>
{:else if fallback}
	<pre class="log" style:max-height={maxHeight}>{fallback}</pre>
{:else}
	<div class="wait">Waiting for the first lines…</div>
{/if}

<style>
	.log {
		margin: 0;
		overflow: auto;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 8px 10px;
		font-family: var(--mono);
		font-size: 11.5px;
		line-height: 1.55;
		color: var(--text-dim);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.wait {
		font-size: 12.5px;
		color: var(--text-faint);
	}
</style>
