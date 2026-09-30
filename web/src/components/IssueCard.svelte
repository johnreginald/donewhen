<script>
	import LabelPill from './LabelPill.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import StateIcon from './StateIcon.svelte';
	import { Box, Lock } from '@lucide/svelte';
	import { openIssue, flashIssueId } from '$lib/ui.js';
	import {
		states, projects, activeProject, activeInitiative, activeLabel, loadIssues,
		blockLinks, issues
	} from '$lib/store.js';

	let { issue } = $props();
	const flashing = $derived($flashIssueId === issue.id);
	const state = $derived($states.find((s) => s.id === issue.stateId));
	const project = $derived($projects.find((p) => p.id === issue.projectId));

	// Tickets it still waits on (blocked by, not yet Done).
	const waitingOn = $derived(
		$blockLinks.filter((l) => l.issueId === issue.id && !l.done).map((l) => $issues.find((i) => i.id === l.blockerId)?.key || '')
	);

	// Click the epic tag → filter the board to that epic.
	function filterEpic(e) {
		e.stopPropagation();
		activeProject.set(issue.projectId);
		activeInitiative.set('');
		activeLabel.set('');
		loadIssues();
	}

	function shortDate(s) {
		try {
			return new Date(s).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
		} catch {
			return '';
		}
	}
</script>

<div
	class="card"
	class:live={flashing}
	role="button"
	tabindex="0"
	onclick={() => openIssue(issue.key)}
	onkeydown={(e) => e.key === 'Enter' && openIssue(issue.key)}
>
	<div class="top">
		{#if state}
			<StateIcon category={state.category} color={state.color} />
		{/if}
		<span class="key">{issue.key}</span>
		{#if issue.childCount > 0}
			<span class="epic" title="{issue.childCount} sub-issues">↳ {issue.childCount}</span>
		{/if}
		{#if state?.category === 'completed' && !issue.docCount}
			<span class="nodoc" title="No implementation doc yet">✦</span>
		{/if}
		<span class="spacer"></span>
		{#if waitingOn.length}
			<span class="lock" title="Blocked by {waitingOn.filter(Boolean).join(', ')}"><Lock size={11} strokeWidth={2.4} />{waitingOn.length}</span>
		{/if}
		<span class="assignee" class:on={issue.assigneeId}></span>
	</div>

	<div class="title">{issue.title}</div>

	<div class="meta">
		<PriorityIcon priority={issue.priority} />
		{#if project}
			<button class="epictag" onclick={filterEpic} title="Show all issues in {project.name}">
				<Box size={12} strokeWidth={2.2} />{project.name}
			</button>
		{/if}
		{#each issue.labels ?? [] as l (l.id)}
			<LabelPill label={l} />
		{/each}
	</div>

	<div class="foot">
		<span class="created">Created {shortDate(issue.createdAt)}</span>
	</div>
</div>

<style>
	.card {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 11px 12px 10px;
		display: flex;
		flex-direction: column;
		gap: 7px;
		cursor: pointer;
		transition: border-color 0.12s, background 0.12s, box-shadow 0.3s;
	}
	.card:hover {
		border-color: var(--line-strong);
		background: var(--surface);
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
		font-size: 12.5px;
		color: var(--ink-3);
		font-family: var(--mono);
		letter-spacing: -0.02em;
	}
	.nodoc {
		font-size: 10px;
		color: var(--ink-3);
		opacity: 0.6;
	}
	.epic {
		font-size: 10.5px;
		font-family: var(--mono);
		color: var(--accent);
		background: color-mix(in srgb, var(--accent) 15%, transparent);
		padding: 1px 6px;
		border-radius: 10px;
		letter-spacing: -0.02em;
	}
	.spacer {
		flex: 1;
	}
	.assignee {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 1.4px dashed var(--line-strong);
		flex: none;
	}
	.assignee.on {
		border-style: solid;
		background: var(--line-strong);
	}
	.title {
		font-size: 14px;
		line-height: 1.45;
		font-weight: 500;
		color: var(--ink);
		letter-spacing: -0.011em;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}
	/* Epic = structural (accent2 tint + box icon) → visually distinct from labels */
	.epictag {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 3px 9px 3px 7px;
		border-radius: 6px;
		font-size: 12.5px;
		font-weight: 500;
		line-height: 1.3;
		color: color-mix(in srgb, var(--accent) 55%, var(--ink));
		background: color-mix(in srgb, var(--accent) 13%, var(--surface));
		border: 1px solid color-mix(in srgb, var(--accent) 32%, var(--line));
		white-space: nowrap;
		cursor: pointer;
	}
	.epictag :global(svg) {
		color: var(--accent);
		flex: none;
	}
	.epictag:hover {
		background: color-mix(in srgb, var(--accent) 22%, var(--surface));
		border-color: var(--accent);
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 12px;
		color: var(--ink-3);
		margin-top: 1px;
		min-width: 0;
	}
	.created {
		margin-left: auto;
		white-space: nowrap;
	}
	.lock {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: 11px;
		color: var(--st-blocked);
		background: color-mix(in srgb, var(--st-blocked) 12%, transparent);
		border-radius: 5px;
		padding: 1px 5px;
		font-variant-numeric: tabular-nums;
	}
</style>
