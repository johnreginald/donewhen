<script>
	// The s / p / l / e menus: status, priority, labels and epic of the focused
	// or open issue, anchored to its card and driven entirely from the keyboard
	// (arrows, Enter, Esc). Opened through the issueMenu store by the layout.
	import { tick } from 'svelte';
	import { Check } from '@lucide/svelte';
	import { api } from '$lib/api.js';
	import { states, projects, labels, labelGroups, allIssues, replaceIssue, PRIORITIES } from '$lib/store.js';
	import { issueMenu, showToast, blockedReasonFor } from '$lib/ui.js';
	import { gateFailure, gateSummary } from '$lib/gate.js';
	import { toggleLabel } from '$lib/capture.js';
	import { wrapIndex } from '$lib/shortcuts.js';
	import StateIcon from './StateIcon.svelte';

	const TITLES = { status: 'Status', priority: 'Priority', label: 'Labels', epic: 'Epic' };

	let issue = $state(null); // the issue being changed, kept current as choices apply
	let sel = $state(0);
	let pos = $state(null); // { top, left } under the anchor, or null: centred
	let menuEl = $state(null);
	let lastFocused = null;

	let openedFor = '';
	$effect(() => {
		const m = $issueMenu;
		if (!m) {
			openedFor = '';
			if (lastFocused) {
				lastFocused.focus?.();
				lastFocused = null;
			}
			return;
		}
		const live = $allIssues.find((i) => i.key === m.key);
		if (openedFor === m.kind + m.key) {
			// Already open: just follow live changes to the issue.
			if (live) issue = live;
			return;
		}
		openedFor = m.kind + m.key;
		lastFocused = document.activeElement;
		issue = live || null;
		if (!issue) {
			// Not in the live list (an archived epic's issue): fetch it once.
			api.issue(m.key).then((i) => ($issueMenu ? (issue = i) : null)).catch(() => issueMenu.set(null));
		}
		const anchor = document.querySelector(`[data-issue-key="${CSS.escape(m.key)}"]`);
		if (anchor) {
			const r = anchor.getBoundingClientRect();
			const left = Math.max(8, Math.min(r.left, window.innerWidth - 248));
			const top = Math.min(r.bottom + 4, window.innerHeight - 280);
			pos = { top: Math.max(8, top), left };
		} else pos = null;
	});

	// What the menu offers, with the issue's current choice marked.
	const options = $derived.by(() => {
		if (!issue || !$issueMenu) return [];
		switch ($issueMenu.kind) {
			case 'status':
				return $states.map((s) => ({ id: s.id, name: s.name, state: s, on: s.id === issue.stateId }));
			case 'priority':
				return PRIORITIES.map((p) => ({ id: String(p.value), name: p.label, on: p.value === (issue.priority ?? 0) }));
			case 'epic':
				return [
					{ id: '', name: 'No epic', on: !issue.projectId },
					...$projects.map((p) => ({ id: p.id, name: p.name, on: p.id === issue.projectId }))
				];
			case 'label':
				return $labels.map((l) => ({ id: l.id, name: l.name, color: l.color, on: (issue.labels || []).some((x) => x.id === l.id) }));
		}
		return [];
	});

	// Start on the current choice so Enter alone changes nothing by accident.
	let startedFor = '';
	$effect(() => {
		const m = $issueMenu;
		if (!m || !options.length) {
			if (!m) startedFor = '';
			return;
		}
		const id = m.kind + m.key;
		if (startedFor === id) return;
		startedFor = id;
		sel = Math.max(0, options.findIndex((o) => o.on));
		tick().then(() => menuEl?.focus());
	});

	function close() {
		issueMenu.set(null);
	}

	async function choose(opt) {
		const m = $issueMenu;
		if (!m || !issue) return;
		let body;
		if (m.kind === 'status') {
			if (opt.id === issue.stateId) return close();
			const reason = await blockedReasonFor(issue.stateId, opt.id, issue.key);
			if (reason === null) return close();
			body = { stateId: opt.id, ...(reason ? { blockedReason: reason } : {}) };
		} else if (m.kind === 'priority') {
			if (Number(opt.id) === (issue.priority ?? 0)) return close();
			body = { priority: Number(opt.id) };
		} else if (m.kind === 'epic') {
			if (opt.id === (issue.projectId || '')) return close();
			body = { projectId: opt.id };
		} else {
			const next = toggleLabel((issue.labels || []).map((l) => l.id), opt.id, $labels, $labelGroups);
			body = { labelIds: next };
		}
		try {
			const saved = await api.updateIssue(issue.id, body);
			replaceIssue(saved);
			issue = saved;
			// Labels are a multi-choice: stay open for the next one.
			if (m.kind !== 'label') close();
			else menuEl?.focus();
		} catch (e) {
			const g = gateFailure(e);
			showToast(g ? gateSummary(g, issue.key) : 'Update failed: ' + (e?.message || 'unknown error'), 'error');
			close();
		}
	}

	function onKey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			close();
		} else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			sel = wrapIndex(sel, options.length, e.key === 'ArrowDown' ? 1 : -1);
		} else if (e.key === 'Home' || e.key === 'End') {
			e.preventDefault();
			sel = e.key === 'Home' ? 0 : options.length - 1;
		} else if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			if (options[sel]) choose(options[sel]);
		} else if (e.key === 'Tab') {
			e.preventDefault(); // the menu is the only stop while it is open
		}
		e.stopPropagation();
	}

	$effect(() => {
		// Keep the highlighted row in view in a long list.
		sel;
		menuEl?.querySelector('.im-item.sel')?.scrollIntoView({ block: 'nearest' });
	});
</script>

{#if $issueMenu}
	<div class="im-bd" role="presentation" onclick={close}></div>
	<div
		class="im-menu"
		class:centred={!pos}
		style:top={pos ? pos.top + 'px' : undefined}
		style:left={pos ? pos.left + 'px' : undefined}
		bind:this={menuEl}
		role="listbox"
		tabindex="-1"
		aria-label="{TITLES[$issueMenu.kind]} for {$issueMenu.key}"
		aria-multiselectable={$issueMenu.kind === 'label'}
		aria-activedescendant={options[sel] ? `im-opt-${sel}` : undefined}
		onkeydown={onKey}
	>
		<div class="im-head">{TITLES[$issueMenu.kind]} · {$issueMenu.key}</div>
		{#each options as o, i (o.id)}
			<div
				id={`im-opt-${i}`}
				class="im-item"
				class:sel={i === sel}
				role="option"
				aria-selected={o.on}
				tabindex="-1"
				onmouseenter={() => (sel = i)}
				onclick={() => choose(o)}
				onkeydown={(e) => e.key === 'Enter' && choose(o)}
			>
				<span class="im-check">{#if o.on}<Check size={13} strokeWidth={2.6} />{/if}</span>
				{#if o.state}<StateIcon category={o.state.category} color={o.state.color} />{/if}
				{#if o.color}<span class="im-dot" style:background={o.color}></span>{/if}
				<span class="im-name">{o.name}</span>
			</div>
		{:else}
			<div class="im-empty">Loading…</div>
		{/each}
	</div>
{/if}

<style>
	.im-bd {
		position: fixed;
		inset: 0;
		z-index: 70;
	}
	.im-menu {
		position: fixed;
		z-index: 71;
		width: 240px;
		max-height: min(360px, 70vh);
		overflow-y: auto;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		padding: 5px;
		outline: none;
	}
	.im-menu:focus-visible {
		box-shadow:
			var(--shadow-2),
			0 0 0 3px var(--focus);
	}
	.im-menu.centred {
		top: 20vh;
		left: 50%;
		transform: translateX(-50%);
	}
	.im-head {
		font-family: var(--mono);
		font-size: var(--t-xs);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--ink-3);
		padding: 6px 8px 7px;
	}
	.im-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 7px 8px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
		color: var(--ink-2);
		cursor: pointer;
	}
	.im-item.sel {
		background: var(--hover);
		color: var(--ink);
	}
	.im-check {
		width: 14px;
		height: 14px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--accent);
		flex: none;
	}
	.im-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
	}
	.im-empty {
		padding: 10px;
		color: var(--ink-3);
		font-size: var(--t-sm);
	}
</style>
