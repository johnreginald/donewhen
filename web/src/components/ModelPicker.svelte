<script>
	// Choose an agent's model from what the host reported its harness can run
	// — Claude's aliases, the Codex login's models, the OpenCode providers that
	// have keys. Searchable, grouped by provider, keyboard-driven; anything
	// typed that is not listed can be used as it is.
	import { tick } from 'svelte';
	import { ChevronDown, Check, Search } from '@lucide/svelte';

	let { value = $bindable(''), models = [], harness = '' } = $props();

	// Claude and Codex fall back to their own configured model; OpenCode has no
	// such default, so it must name one.
	const hasDefault = $derived(harness !== 'opencode');
	const DEFAULT = { id: '', label: 'Default', hint: "the harness's own configured model" };

	let open = $state(false);
	let query = $state('');
	let active = $state(0);
	let searchEl = $state(null);
	let listEl = $state(null);

	// Items: the default (when there is one), then every model, each knowing
	// its provider group.
	const items = $derived.by(() => {
		const out = hasDefault ? [{ ...DEFAULT, group: '' }] : [];
		for (const m of models) {
			if (m === 'default') continue;
			const slash = m.indexOf('/');
			out.push({ id: m, label: slash > 0 ? m.slice(slash + 1) : m, group: slash > 0 ? m.slice(0, slash) : '' });
		}
		return out;
	});
	const q = $derived(query.trim().toLowerCase());
	const shown = $derived(q ? items.filter((i) => i.id.toLowerCase().includes(q) || i.label.toLowerCase().includes(q)) : items);
	// Typing a name nobody listed offers it as a custom model.
	const customOffer = $derived(q && !items.some((i) => i.id.toLowerCase() === q) ? query.trim() : '');
	const rows = $derived([...shown, ...(customOffer ? [{ id: customOffer, label: customOffer, group: '', custom: true }] : [])]);

	const current = $derived(items.find((i) => i.id === (value || '')));
	const display = $derived(value ? current?.label || value : hasDefault ? 'Default' : '');
	const displayGroup = $derived(value && current?.group ? current.group : '');

	async function toggle() {
		open = !open;
		if (open) {
			query = '';
			active = Math.max(0, rows.findIndex((r) => r.id === (value || '')));
			await tick();
			searchEl?.focus();
			scrollActive();
		}
	}
	function choose(r) {
		value = r.id;
		open = false;
	}
	function scrollActive() {
		listEl?.querySelector(`[data-i="${active}"]`)?.scrollIntoView({ block: 'nearest' });
	}
	function onKey(e) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			active = Math.min(rows.length - 1, active + 1);
			scrollActive();
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			active = Math.max(0, active - 1);
			scrollActive();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (rows[active]) choose(rows[active]);
		} else if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			open = false;
		}
	}
	$effect(() => {
		query;
		active = 0;
	});
</script>

<div class="mp">
	<button type="button" class="trigger" class:placeholder={!display} onclick={toggle} aria-haspopup="listbox" aria-expanded={open}>
		{#if displayGroup}<span class="grp">{displayGroup}</span>{/if}
		<span class="val">{display || 'Choose a model…'}</span>
		<span class="count">{models.filter((m) => m !== 'default').length || ''}</span>
		<ChevronDown size={15} strokeWidth={2} />
	</button>

	{#if open}
		<div class="bd" role="presentation" onclick={() => (open = false)}></div>
		<div class="pop">
			<label class="search">
				<Search size={14} strokeWidth={2} />
				<input bind:this={searchEl} bind:value={query} onkeydown={onKey}
					placeholder={models.length ? 'Search models…' : 'Type a model name…'} />
			</label>
			<ul class="list" role="listbox" bind:this={listEl}>
				{#each rows as r, i (r.id + (r.custom ? '#c' : ''))}
					{#if !r.custom && r.group && (i === 0 || rows[i - 1].group !== r.group)}
						<li class="gh" role="presentation">{r.group}</li>
					{/if}
					<li
						role="option"
						aria-selected={r.id === (value || '')}
						data-i={i}
						class="opt"
						class:active={i === active}
						onmousemove={() => (active = i)}
						onclick={() => choose(r)}
						onkeydown={() => {}}
					>
						<span class="ol">
							{#if r.custom}Use “{r.label}”{:else}{r.label}{/if}
							{#if r.hint}<span class="oh">{r.hint}</span>{/if}
						</span>
						{#if r.id === (value || '') && !r.custom}<Check size={14} strokeWidth={2.4} />{/if}
					</li>
				{:else}
					<li class="empty" role="presentation">
						{models.length ? 'No model matches.' : 'No list yet — start the runner host so it can report what this harness offers.'}
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>

<style>
	.mp {
		position: relative;
	}
	.trigger {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		color: var(--text);
		text-align: left;
	}
	.trigger:hover,
	.trigger[aria-expanded='true'] {
		border-color: var(--border-strong);
	}
	.trigger.placeholder .val {
		color: var(--text-faint);
	}
	.grp {
		font-size: 11px;
		color: var(--text-faint);
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 5px;
		padding: 0 6px;
	}
	.val {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.count {
		font-size: 11px;
		color: var(--text-faint);
	}
	.trigger :global(svg) {
		color: var(--text-faint);
		flex-shrink: 0;
	}
	.bd {
		position: fixed;
		inset: 0;
		z-index: 80;
	}
	.pop {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		z-index: 81;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 10px;
		box-shadow: var(--shadow);
		overflow: hidden;
	}
	.search {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 10px;
		border-bottom: 1px solid var(--border);
		color: var(--text-faint);
	}
	.search input {
		flex: 1;
		background: none;
		border: none;
		outline: none;
		font-size: 13px;
		color: var(--text);
	}
	.list {
		list-style: none;
		margin: 0;
		padding: 4px;
		max-height: 280px;
		overflow-y: auto;
	}
	.gh {
		font-size: 10.5px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-faint);
		padding: 8px 8px 3px;
	}
	.opt {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 7px 8px;
		border-radius: 6px;
		font-size: 13px;
		cursor: pointer;
		color: var(--text);
	}
	.opt.active {
		background: var(--bg-hover);
	}
	.opt :global(svg) {
		color: var(--accent2);
		margin-left: auto;
		flex-shrink: 0;
	}
	.ol {
		display: flex;
		flex-direction: column;
		min-width: 0;
		line-height: 1.35;
		font-family: var(--mono);
		font-size: 12.5px;
	}
	.oh {
		font-family: var(--font);
		font-size: 11.5px;
		color: var(--text-faint);
	}
	.empty {
		padding: 12px 10px;
		font-size: 12.5px;
		color: var(--text-faint);
	}
</style>
