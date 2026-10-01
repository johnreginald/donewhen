<script>
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { workspaces, initiatives, archivedProjects, activeInitiative, activeProject, loadIssues, switchWorkspace } from '$lib/store.js';
	import { paletteOpen, quickCapture, openIssue, showToast } from '$lib/ui.js';

	// mode: 'search' (issues + nav + switch commands) or one of the two
	// switcher sub-lists a "Switch workspace…" / "Switch project…" result
	// drops into.
	let mode = $state('search');
	let query = $state('');
	let sel = $state(0);
	let inputEl = $state(null);
	let lastFocused = null;

	// Server-side issue search (GET /api/issues?q=), debounced.
	let serverIssues = $state([]);
	$effect(() => {
		const q = query.trim();
		if (mode !== 'search' || !q) {
			serverIssues = [];
			return;
		}
		const t = setTimeout(async () => {
			try {
				serverIssues = (await api.issues({ q, limit: 8, includeArchived: 1 })) || [];
			} catch {
				serverIssues = [];
			}
		}, 180);
		return () => clearTimeout(t);
	});

	$effect(() => {
		if ($paletteOpen) {
			lastFocused = document.activeElement;
			mode = 'search';
			query = '';
			sel = 0;
			serverIssues = [];
			queueMicrotask(() => inputEl && inputEl.focus());
		} else if (lastFocused) {
			lastFocused.focus?.();
			lastFocused = null;
		}
	});

	const NAV = [
		{ label: 'Go to Board', to: '/board' },
		{ label: 'Go to List', to: '/list' },
		{ label: 'Go to Tasks', to: '/tasks' },
		{ label: 'Go to Links', to: '/links' },
		{ label: 'Go to Inbox', to: '/inbox' },
		{ label: 'Go to Artifacts', to: '/artifacts' },
		{ label: 'Go to Log', to: '/log' },
		{ label: 'Go to Settings', to: '/settings' }
	];

	function enterMode(m) {
		return () => {
			mode = m;
			query = '';
			sel = 0;
		};
	}

	const items = $derived(build(mode, query, serverIssues, $workspaces, $initiatives, $archivedProjects));

	function build(mode, q, srvIssues, wsList, iniList, archList) {
		const ql = q.trim().toLowerCase();
		if (mode === 'workspace') {
			return wsList
				.filter((w) => !ql || w.name.toLowerCase().includes(ql) || w.keyPrefix.toLowerCase().includes(ql))
				.map((w) => ({ kind: 'switch', id: 'ws-' + w.id, label: w.name, run: () => switchWorkspace(w.slug) }));
		}
		if (mode === 'project') {
			const all = { kind: 'switch', id: 'proj-all', label: 'All projects', run: pickInitiative('') };
			const rest = iniList
				.filter((i) => !ql || i.name.toLowerCase().includes(ql))
				.map((i) => ({ kind: 'switch', id: 'proj-' + i.id, label: i.name, run: pickInitiative(i.id) }));
			return !ql || 'all projects'.includes(ql) ? [all, ...rest] : rest;
		}
		const list = [];
		if (ql) {
			list.push({
				kind: 'create',
				id: 'create',
				label: `Create issue "${q.trim()}"`,
				run: async () => {
					const is = await api.createIssue({ title: q.trim(), stateName: 'Triage' });
					showToast(`${is.key} created`);
					openIssue(is.key);
				}
			});
		}
		if (!ql || 'create issue'.includes(ql) || 'new issue'.includes(ql)) {
			list.push({ kind: 'create', id: 'create-dialog', label: 'Create issue', run: () => quickCapture.set(true) });
		}
		if (!ql || 'switch workspace'.includes(ql)) {
			list.push({ kind: 'switch', id: 'sw-ws', label: 'Switch workspace…', keepOpen: true, run: enterMode('workspace') });
		}
		if (!ql || 'switch project'.includes(ql)) {
			list.push({ kind: 'switch', id: 'sw-proj', label: 'Switch project…', keepOpen: true, run: enterMode('project') });
		}
		for (const n of NAV) {
			if (!ql || n.label.toLowerCase().includes(ql)) {
				list.push({ kind: 'nav', id: 'nav-' + n.to, label: n.label, run: () => goto(n.to) });
			}
		}
		for (const is of srvIssues) {
			list.push({ kind: 'issue', id: 'issue-' + is.id, label: `${is.key}  ${is.title}` + (archList.some((p) => p.id === is.projectId) ? '  · archived' : ''), run: () => openIssue(is.key) });
		}
		return list;
	}

	function pickInitiative(id) {
		return () => {
			activeInitiative.set(id);
			activeProject.set('');
			loadIssues();
		};
	}

	// Selection stays in range as the list changes under it (async search
	// results landing, or a mode switch shrinking the list).
	$effect(() => {
		if (sel > items.length - 1) sel = Math.max(items.length - 1, 0);
	});

	function onKey(e) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			sel = Math.min(sel + 1, Math.max(items.length - 1, 0));
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			sel = Math.max(sel - 1, 0);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			choose(items[sel]);
		} else if (e.key === 'Escape') {
			e.preventDefault();
			paletteOpen.set(false);
		} else if (e.key === 'Tab') {
			// Nothing else in the dialog takes focus — keep it trapped on the input.
			e.preventDefault();
		}
	}

	async function choose(item) {
		if (!item) return;
		if (!item.keepOpen) paletteOpen.set(false);
		await item.run();
	}

	const placeholder = $derived(
		mode === 'workspace' ? 'Switch workspace…' : mode === 'project' ? 'Switch project…' : 'Search issues, create, navigate…'
	);
	const activeId = $derived(items[sel] ? `pal-opt-${items[sel].id}` : undefined);
</script>

{#if $paletteOpen}
	<div class="backdrop" role="presentation" onclick={() => paletteOpen.set(false)}></div>
	<div class="palette" role="dialog" aria-modal="true" aria-label="Command palette">
		<div class="pal-input-row">
			<span class="pico" aria-hidden="true"></span>
			<input
				bind:this={inputEl}
				bind:value={query}
				onkeydown={onKey}
				class="input pal-input"
				role="combobox"
				aria-expanded="true"
				aria-controls="palette-listbox"
				aria-autocomplete="list"
				aria-activedescendant={activeId}
				placeholder={placeholder}
			/>
		</div>
		<div class="presults" role="listbox" id="palette-listbox" aria-label="Command palette results">
			{#each items as item, i (item.id)}
				<button
					id={`pal-opt-${item.id}`}
					role="option"
					aria-selected={i === sel}
					class="pres"
					class:sel={i === sel}
					onmouseenter={() => (sel = i)}
					onclick={() => choose(item)}
				>
					<span class="pkind">{item.kind}</span>
					<span class="plabel">{item.label}</span>
				</button>
			{:else}
				<div class="pempty">No matches for &ldquo;{query}&rdquo;.<br />Try a different search, or press <kbd>Esc</kbd> to close.</div>
			{/each}
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: oklch(0.22 0.01 70 / 0.42);
		z-index: 60;
	}
	.palette {
		position: fixed;
		top: 15vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(560px, 92vw);
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		z-index: 61;
		overflow: hidden;
	}
	.pal-input-row {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 16px 18px;
		border-bottom: 1px solid var(--line);
	}
	.pico {
		width: 15px;
		height: 15px;
		border: 1.8px solid var(--ink-3);
		border-radius: 50%;
		flex: none;
	}
	.pal-input {
		font-size: var(--t-md);
	}
	.presults {
		max-height: 340px;
		overflow-y: auto;
		padding: 6px;
	}
	.pres {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		text-align: left;
		background: none;
		border: none;
		color: var(--ink);
		padding: 9px 12px;
		border-radius: var(--r-sm);
		font-size: var(--t-base);
	}
	.pres.sel {
		background: var(--accent-soft);
	}
	.pkind {
		font-family: var(--mono);
		font-size: var(--t-xs);
		text-transform: uppercase;
		color: var(--ink-3);
		width: 44px;
		flex: none;
	}
	.pres.sel .pkind {
		color: var(--accent);
	}
	.plabel {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pempty {
		padding: 44px 20px;
		text-align: center;
		color: var(--ink-3);
		font-size: var(--t-base);
		line-height: 1.6;
	}
	.pempty kbd {
		font-family: var(--mono);
		font-size: var(--t-xs);
		padding: 1px 5px;
		border: 1px solid var(--line-strong);
		border-bottom-width: 2px;
		border-radius: var(--r-sm);
		color: var(--ink-2);
		background: var(--surface);
	}
	@media (max-width: 720px) {
		.palette {
			top: auto;
			bottom: 0;
			left: 0;
			right: 0;
			transform: none;
			width: 100%;
			max-width: 100%;
			border-radius: var(--r-lg) var(--r-lg) 0 0;
			max-height: 80vh;
			display: flex;
			flex-direction: column;
		}
		.presults {
			flex: 1;
		}
	}
</style>
