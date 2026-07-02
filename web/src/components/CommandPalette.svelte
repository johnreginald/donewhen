<script>
	import { goto } from '$app/navigation';
	import { issues, states, loadIssues } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { paletteOpen, openIssue, showToast } from '$lib/ui.js';

	let query = $state('');
	let sel = $state(0);
	let inputEl = $state(null);

	$effect(() => {
		if ($paletteOpen) {
			query = '';
			sel = 0;
			queueMicrotask(() => inputEl && inputEl.focus());
		}
	});

	// Build the candidate list from query.
	const items = $derived(build(query, $issues, $states));

	function build(q, allIssues, allStates) {
		const list = [];
		const ql = q.trim().toLowerCase();

		if (ql) {
			list.push({
				kind: 'action',
				label: `Create issue: “${q.trim()}”`,
				run: async () => {
					const is = await api.createIssue({ title: q.trim(), stateName: 'Backlog' });
					await loadIssues();
					showToast(`${is.key} created`);
					openIssue(is.key);
				}
			});
		}

		const nav = [
			{ label: 'Go to Board', to: '/' },
			{ label: 'Go to List', to: '/list' },
			{ label: 'Go to Documents', to: '/docs' },
			{ label: 'Go to Settings', to: '/settings' }
		];
		for (const n of nav) {
			if (!ql || n.label.toLowerCase().includes(ql)) {
				list.push({ kind: 'nav', label: n.label, run: () => goto(n.to) });
			}
		}

		const matched = allIssues
			.filter(
				(i) =>
					!ql ||
					i.title.toLowerCase().includes(ql) ||
					i.key.toLowerCase().includes(ql)
			)
			.slice(0, 8);
		for (const i of matched) {
			list.push({
				kind: 'issue',
				label: `${i.key}  ${i.title}`,
				run: () => openIssue(i.key)
			});
		}
		return list;
	}

	function onKey(e) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			sel = Math.min(sel + 1, items.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			sel = Math.max(sel - 1, 0);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			choose(items[sel]);
		} else if (e.key === 'Escape') {
			paletteOpen.set(false);
		}
	}

	async function choose(item) {
		if (!item) return;
		paletteOpen.set(false);
		await item.run();
	}
</script>

{#if $paletteOpen}
	<div class="backdrop" role="presentation" onclick={() => paletteOpen.set(false)}></div>
	<div class="palette">
		<input
			bind:this={inputEl}
			bind:value={query}
			onkeydown={onKey}
			class="pal-input"
			placeholder="Search issues, create, navigate…"
		/>
		<div class="results">
			{#each items as item, i (item.label)}
				<button
					class="result"
					class:sel={i === sel}
					onmouseenter={() => (sel = i)}
					onclick={() => choose(item)}
				>
					<span class="kind">{item.kind}</span>
					<span class="label">{item.label}</span>
				</button>
			{:else}
				<div class="empty faint">No matches</div>
			{/each}
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.4);
		z-index: 60;
	}
	.palette {
		position: fixed;
		top: 15vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(560px, 92vw);
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 12px;
		box-shadow: var(--shadow);
		z-index: 61;
		overflow: hidden;
	}
	.pal-input {
		width: 100%;
		background: transparent;
		border: none;
		border-bottom: 1px solid var(--border);
		padding: 16px 18px;
		font-size: 16px;
		outline: none;
	}
	.results {
		max-height: 50vh;
		overflow-y: auto;
		padding: 6px;
	}
	.result {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		text-align: left;
		background: none;
		border: none;
		color: var(--text);
		padding: 9px 12px;
		border-radius: 6px;
		font-size: 13.5px;
	}
	.result.sel {
		background: var(--bg-hover);
	}
	.kind {
		font-size: 10px;
		text-transform: uppercase;
		color: var(--text-faint);
		width: 46px;
		flex-shrink: 0;
	}
	.label {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.empty {
		padding: 20px;
		text-align: center;
	}
</style>
