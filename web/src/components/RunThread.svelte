<script>
	// A run as part of the conversation, as Paperclip shows it: the "Worked"
	// line, then what the agent said as messages and the tools it used folded
	// into groups between them — live while it works. The raw transcript stays
	// behind the "Worked" line.
	//
	// Every harness posts the same readable lines: "· " meta, "→ " a tool call,
	// "← " / "✗ " its result, anything else the agent speaking.
	import { onMount, untrack } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents } from '$lib/store.js';
	import { onLive } from '$lib/ui.js';
	import { Bot, Wrench, ChevronRight } from '@lucide/svelte';
	import RunBlock from './RunBlock.svelte';
	import Markdown from './Markdown.svelte';

	let { run } = $props();
	let lines = $state([]);
	let last = 0;
	let openGroups = $state({});

	function append(newLines, seq) {
		const fresh = newLines.filter((_, i) => seq + i > last);
		if (!fresh.length) return;
		lines = [...lines, ...fresh];
		last = seq + newLines.length - 1;
	}
	onMount(() => {
		const id = untrack(() => run.id);
		api.get(`/runs/${id}/events`).then((evs) => {
			if (evs?.length) append(evs.map((e) => e.text), evs[0].seq);
		}).catch(() => {});
		return onLive((ev) => {
			if (ev.type === 'run.events' && ev.runId === id) append(ev.lines || [], ev.seq || 1);
		});
	});

	// Transcripts from before every harness was rendered are raw JSON: they
	// stay behind the "Worked" line rather than being shown as speech.
	const readable = $derived(run.runner === 'claude' || lines[0]?.startsWith('· '));

	const items = $derived.by(() => {
		if (!readable) return [];
		const out = [];
		for (const l of lines) {
			if (l.startsWith('· ')) continue;
			if (l.startsWith('→ ')) {
				const body = l.slice(2);
				const sp = body.indexOf(' ');
				const call = { name: sp > 0 ? body.slice(0, sp) : body, input: sp > 0 ? body.slice(sp + 1) : '', result: '', err: false };
				const prev = out[out.length - 1];
				if (prev?.kind === 'tools') prev.calls.push(call);
				else out.push({ kind: 'tools', calls: [call] });
			} else if (l.startsWith('← ') || l.startsWith('✗ ')) {
				const prev = out[out.length - 1];
				const call = prev?.kind === 'tools' ? prev.calls[prev.calls.length - 1] : null;
				if (call && !call.result) {
					call.result = l.slice(2);
					call.err = l.startsWith('✗');
				} else if (l.startsWith('✗ ')) {
					out.push({ kind: 'error', text: l.slice(2) });
				}
			} else if (l.trim()) {
				out.push({ kind: 'say', text: l });
			}
		}
		// A chat turn's final words are posted as its reply comment; showing
		// them here too would say everything twice.
		if (run.kind === 'chat' && run.finishedAt) {
			for (let i = out.length - 1; i >= 0; i--) {
				if (out[i].kind === 'say') {
					out.splice(i, 1);
					break;
				}
			}
		}
		return out;
	});

	const agentName = $derived($agents.find((a) => a.id === run.agentId)?.name || 'Agent');
	const toggle = (i) => (openGroups = { ...openGroups, [i]: !openGroups[i] });
</script>

<div class="rt">
	<div class="head"><RunBlock {run} /></div>
	{#each items as it, i (i)}
		{#if it.kind === 'say'}
			<article class="say">
				<header><span class="av"><Bot size={13} strokeWidth={2} /></span><span class="who">{agentName}</span></header>
				<div class="body"><Markdown source={it.text} /></div>
			</article>
		{:else if it.kind === 'tools'}
			<div class="tools" class:open={openGroups[i]}>
				<button class="tg" onclick={() => toggle(i)} aria-expanded={!!openGroups[i]}>
					<Wrench size={13} strokeWidth={2} />
					<span>Used {it.calls.length} {it.calls.length === 1 ? 'tool' : 'tools'}</span>
					<span class="names">{[...new Set(it.calls.map((c) => c.name))].slice(0, 4).join(', ')}</span>
					{#if it.calls.some((c) => c.err)}<span class="bad">{it.calls.filter((c) => c.err).length} failed</span>{/if}
					<span class="chev"><ChevronRight size={13} strokeWidth={2} /></span>
				</button>
				{#if openGroups[i]}
					<ul>
						{#each it.calls as c, j (j)}
							<li>
								<div class="call"><span class="tn">{c.name}</span> <span class="ti">{c.input}</span></div>
								{#if c.result}<div class="res" class:err={c.err}>{c.result}</div>{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{:else if it.kind === 'error'}
			<div class="res err solo">{it.text}</div>
		{/if}
	{/each}
</div>

<style>
	.rt {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.head {
		margin: -4px -8px;
	}
	.say header {
		display: flex;
		align-items: center;
		gap: 7px;
		font-size: 12.5px;
		color: var(--text);
		margin-bottom: 4px;
	}
	.av {
		display: inline-grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--bg-elev2);
		color: var(--text-dim);
	}
	.say .body {
		font-size: 13.5px;
		line-height: 1.55;
		padding-left: 27px;
	}
	.tools {
		margin-left: 27px;
		border: 1px solid var(--border);
		border-radius: 8px;
		background: var(--bg);
	}
	.tg {
		display: flex;
		align-items: center;
		gap: 7px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text-dim);
		font-size: 12.5px;
		padding: 6px 9px;
		text-align: left;
	}
	.tg:hover {
		color: var(--text);
	}
	.names {
		color: var(--text-faint);
		font-family: var(--mono);
		font-size: 11.5px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
		flex: 1;
	}
	.bad {
		color: #f87171;
		font-size: 11.5px;
	}
	.chev {
		display: inline-flex;
		color: var(--text-faint);
		transition: transform 0.15s;
	}
	.tools.open .chev {
		transform: rotate(90deg);
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0 9px 8px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		max-height: 360px;
		overflow-y: auto;
	}
	.call,
	.res {
		font-family: var(--mono);
		font-size: 11.5px;
		line-height: 1.45;
		overflow-wrap: anywhere;
	}
	.tn {
		color: var(--text);
	}
	.ti {
		color: var(--text-dim);
	}
	.res {
		color: var(--text-faint);
		padding-left: 10px;
		border-left: 2px solid var(--border);
		margin-top: 2px;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.res.err {
		color: #fca5a5;
		border-left-color: color-mix(in srgb, #d03b3b 50%, var(--border));
	}
	.res.solo {
		margin-left: 27px;
	}
</style>
