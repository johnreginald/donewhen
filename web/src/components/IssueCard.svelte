<script>
	import LabelPill from './LabelPill.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import StateIcon from './StateIcon.svelte';
	import { Box } from '@lucide/svelte';
	import { flashIssueId } from '$lib/ui.js';
	import {
		states, projects, activeProject, activeInitiative, activeLabel, loadIssues,
		blockLinks, issues, aiName
	} from '$lib/store.js';

	let { issue } = $props();
	const flashing = $derived($flashIssueId === issue.id);
	const state = $derived($states.find((s) => s.id === issue.stateId));
	const project = $derived($projects.find((p) => p.id === issue.projectId));

	// Tickets it still waits on (blocked by, not yet Done) — one chip per key.
	const waitingOn = $derived(
		$blockLinks.filter((l) => l.issueId === issue.id && !l.done).map((l) => $issues.find((i) => i.id === l.blockerId)?.key).filter(Boolean)
	);

	// Summaries ride on the issue list payload (no per-card requests).
	const crit = $derived({ done: issue.criteriaDone ?? 0, total: issue.criteriaTotal ?? 0 });
	const lastActor = $derived(issue.lastActor || null);

	// Click (or keyboard-activate) the epic tag → filter the board to that
	// epic. It sits inside the card's <a>, so both the click and the keydown
	// that browsers synthesize from a focused button's Enter/Space must be
	// stopped here, or they bubble up and also navigate the card (the
	// Enter-key bubbling bug this replaces).
	function filterEpic(e) {
		e.stopPropagation();
		e.preventDefault();
		activeProject.set(issue.projectId);
		activeInitiative.set('');
		activeLabel.set('');
		loadIssues();
	}
	function stopKey(e) {
		e.stopPropagation();
	}

	function shortDate(s) {
		try {
			return new Date(s).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
		} catch {
			return '';
		}
	}
</script>

<a class="card" class:live={flashing} href={'/issue/' + encodeURIComponent(issue.key)}>
	<div class="top">
		{#if state}
			<StateIcon category={state.category} color={state.color} name={state.name} />
		{/if}
		<span class="key">{issue.key}</span>
		{#if issue.childCount > 0}
			<span class="sub" title="{issue.childCount} sub-issues">↳ {issue.childCount}</span>
		{/if}
		{#if state?.category === 'completed' && !issue.docCount}
			<span class="nodoc" title="No implementation doc yet">✦</span>
		{/if}
		<span class="spacer"></span>
		<span class="date">{shortDate(issue.createdAt)}</span>
	</div>

	<div class="title">{issue.title}</div>

	{#if crit.total > 0}
		<div class="prog-row" title="Done-when: {crit.done}/{crit.total}">
			<div class="prog-bar"><span style:width="{(crit.done / crit.total) * 100}%"></span></div>
			<span class="prog-txt">{crit.done}/{crit.total}</span>
		</div>
	{/if}

	<div class="meta">
		<PriorityIcon priority={issue.priority} />
		{#if project}
			<button class="epictag" onclick={filterEpic} onkeydown={stopKey} title="Show all issues in {project.name}">
				<Box size={12} strokeWidth={2.2} />{project.name}
			</button>
		{/if}
		{#each issue.labels ?? [] as l (l.id)}
			<LabelPill label={l} />
		{/each}
		{#each waitingOn as key (key)}
			<span class="blk">⊘ {key}</span>
		{/each}
		<span class="sp2"></span>
		<span
			class="actor"
			class:on={lastActor}
			class:ai={lastActor === 'ai'}
			title={lastActor === 'ai' ? `Last changed by ${$aiName}` : lastActor === 'human' ? 'Last changed by you' : 'No activity yet'}
		>
			{lastActor === 'ai' ? $aiName[0] : lastActor === 'human' ? 'Y' : ''}
		</span>
	</div>
</a>

<style>
	.card {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 11px 12px 10px;
		display: flex;
		flex-direction: column;
		gap: 7px;
		cursor: pointer;
		color: inherit;
		transition: border-color 0.12s, background 0.12s, box-shadow 0.3s;
	}
	.card:hover {
		border-color: var(--line-strong);
		background: var(--surface);
	}
	.card:focus-visible {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--focus);
	}
	.card.live {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-soft);
	}
	.top {
		display: flex;
		align-items: center;
		gap: 7px;
	}
	.key {
		font-size: var(--t-sm);
		color: var(--ink-3);
		font-family: var(--mono);
		letter-spacing: -0.02em;
	}
	.nodoc {
		font-size: var(--t-xs);
		color: var(--ink-3);
		opacity: 0.6;
	}
	.sub {
		font-size: var(--t-xs);
		font-family: var(--mono);
		color: var(--ink-3);
		background: var(--hover);
		padding: 1px 6px;
		border-radius: var(--r-lg);
		letter-spacing: -0.02em;
	}
	.spacer {
		flex: 1;
	}
	.date {
		font-size: var(--t-sm);
		color: var(--ink-3);
		white-space: nowrap;
	}
	.title {
		font-size: var(--t-base);
		line-height: 1.45;
		font-weight: 500;
		color: var(--ink);
		letter-spacing: -0.011em;
	}
	.prog-row {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.prog-bar {
		flex: 1;
		height: 4px;
		border-radius: var(--r-sm);
		background: var(--sunken);
		overflow: hidden;
	}
	.prog-bar span {
		display: block;
		height: 100%;
		background: var(--st-done);
	}
	.prog-txt {
		font-size: var(--t-xs);
		font-family: var(--mono);
		color: var(--ink-3);
		flex: none;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}
	.sp2 {
		flex: 1;
	}
	/* Epic = structural (accent tint + box icon) → visually distinct from labels */
	.epictag {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		height: 20px;
		padding: 0 8px 0 6px;
		border-radius: var(--r-sm);
		font-size: var(--t-xs);
		font-weight: 500;
		line-height: 1.3;
		color: var(--accent);
		background: var(--accent-soft);
		border: none;
		white-space: nowrap;
		cursor: pointer;
	}
	.epictag:hover {
		filter: brightness(0.96);
	}
	.blk {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		height: 22px;
		padding: 0 7px;
		border-radius: var(--r-sm);
		background: var(--danger-soft);
		color: var(--danger);
		font: 500 var(--t-xs) var(--mono);
		white-space: nowrap;
	}
	.actor {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 1.4px dashed var(--line-strong);
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		font: 600 var(--t-xs) var(--mono);
		color: var(--ink-2);
		box-sizing: border-box;
	}
	.actor.on {
		border-style: solid;
		border-color: var(--line);
		background: var(--surface);
	}
	.actor.on.ai {
		background: var(--accent-soft);
		color: var(--accent);
		border-color: transparent;
	}
</style>
