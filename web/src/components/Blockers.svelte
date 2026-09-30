<script>
	// "Blocked by" on a ticket: the tickets it waits on, each with its state,
	// removable on hover, and a picker to add one.
	// Also lists, read-only, the tickets this one blocks.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { issues, states } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import StateIcon from './StateIcon.svelte';
	import { Plus, X } from '@lucide/svelte';

	let { issue, which = 'blockedBy' } = $props();
	let data = $state({ blockedBy: [], blocking: [] });
	let adding = $state(false);
	let query = $state('');
	let inputEl = $state(null);

	const list = $derived(data[which] || []);
	const stateOf = (b) => $states.find((s) => s.name === b.state);

	async function load() {
		data = (await api.get(`/issues/${issue.key}/blockers`).catch(() => null)) || data;
	}
	onMount(load);
	onMount(() =>
		onLive((ev) => {
			if (ev?.type === 'issue.blockers' || ev?.type === 'issue.state_changed') load();
		})
	);

	async function save(keys) {
		try {
			data = await api.put(`/issues/${issue.key}/blockers`, { blockedBy: keys });
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	const remove = (key) => save(data.blockedBy.map((b) => b.key).filter((k) => k !== key));
	function add(key) {
		adding = false;
		query = '';
		if (!data.blockedBy.some((b) => b.key === key)) save([...data.blockedBy.map((b) => b.key), key]);
	}

	const q = $derived(query.trim().toLowerCase());
	const matches = $derived(
		q
			? $issues
					.filter((i) => i.id !== issue.id && !data.blockedBy.some((b) => b.id === i.id))
					.filter((i) => i.key.toLowerCase().includes(q) || i.title.toLowerCase().includes(q))
					.slice(0, 8)
			: []
	);
	function open() {
		adding = true;
		queueMicrotask(() => inputEl?.focus());
	}
	function onKey(e) {
		if (e.key === 'Escape') {
			adding = false;
			query = '';
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (matches[0]) add(matches[0].key);
			else if (/^[A-Za-z][A-Za-z0-9]*-\d+$/.test(query.trim())) add(query.trim().toUpperCase());
		}
	}
</script>

<div class="bl">
	{#each list as b (b.id)}
		<span class="chip" class:done={b.done} class:review={b.state === 'In Review'} title="{b.key} · {b.title} — {b.state}">
			<button class="go" onclick={() => goto('/issue/' + b.key)}>
				<StateIcon category={stateOf(b)?.category || b.category} color={stateOf(b)?.color} />
				<span class="k">{b.key}</span>
			</button>
			{#if which === 'blockedBy'}
				<button class="x" onclick={() => remove(b.key)} aria-label="Remove {b.key}"><X size={11} strokeWidth={2.4} /></button>
			{/if}
		</span>
	{:else}
		{#if which === 'blocking'}<span class="none">—</span>{/if}
	{/each}
	{#if which === 'blockedBy'}
		<span class="add">
			{#if adding}
				<input bind:this={inputEl} bind:value={query} onkeydown={onKey} onblur={() => setTimeout(() => (adding = false), 150)}
					placeholder="Ticket key or title…" />
				{#if matches.length}
					<div class="menu">
						{#each matches as m (m.id)}
							<button onmousedown={(e) => (e.preventDefault(), add(m.key))}><span class="k">{m.key}</span>{m.title}</button>
						{/each}
					</div>
				{/if}
			{:else}
				<button class="addbtn" onclick={open}><Plus size={12} strokeWidth={2.4} />blocker</button>
			{/if}
		</span>
	{/if}
</div>

<style>
	.bl {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		border: 1px solid color-mix(in srgb, var(--st-blocked) 40%, var(--line));
		border-radius: 6px;
		background: var(--surface);
	}
	.chip.review {
		border-color: color-mix(in srgb, var(--st-done) 40%, var(--line));
	}
	.chip.done {
		border-color: var(--line);
		opacity: 0.7;
	}
	.go {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		background: none;
		border: none;
		padding: 2px 7px;
		color: var(--ink);
	}
	.k {
		font-family: var(--mono);
		font-size: 12px;
	}
	.chip.done .k {
		text-decoration: line-through;
		color: var(--ink-3);
	}
	.x {
		display: inline-flex;
		background: none;
		border: none;
		padding: 2px 5px 2px 0;
		color: var(--ink-3);
		opacity: 0;
	}
	.chip:hover .x,
	.x:focus-visible {
		opacity: 1;
	}
	.x:hover {
		color: var(--danger);
	}
	.none {
		color: var(--ink-3);
		font-size: 13px;
	}
	.add {
		position: relative;
	}
	.addbtn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: none;
		border: 1px dashed var(--line);
		border-radius: 6px;
		color: var(--ink-3);
		padding: 2px 8px;
		font-size: 12.5px;
	}
	.addbtn:hover {
		color: var(--ink);
		border-color: var(--ink-3);
	}
	input {
		background: var(--paper);
		border: 1px solid var(--line-strong);
		border-radius: 6px;
		padding: 3px 8px;
		font-size: 12.5px;
		color: var(--ink);
		outline: none;
		width: 200px;
	}
	.menu {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		z-index: 40;
		width: 320px;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: 8px;
		box-shadow: var(--shadow-2);
		padding: 4px;
		display: flex;
		flex-direction: column;
	}
	.menu button {
		display: flex;
		gap: 8px;
		align-items: baseline;
		background: none;
		border: none;
		text-align: left;
		padding: 6px 8px;
		border-radius: 6px;
		font-size: 12.5px;
		color: var(--ink);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	.menu button:hover {
		background: var(--hover);
	}
	.menu .k {
		color: var(--ink-3);
		flex: none;
	}
</style>
