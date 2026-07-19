<script>
	import LabelPill from './LabelPill.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import StateIcon from './StateIcon.svelte';
	import { Box } from '@lucide/svelte';
	import { openIssue, flashIssueId } from '$lib/ui.js';
	import { states, projects, activeProject, activeInitiative, activeLabel, loadIssues } from '$lib/store.js';

	let { issue } = $props();
	const flashing = $derived($flashIssueId === issue.id);
	const state = $derived($states.find((s) => s.id === issue.stateId));
	const project = $derived($projects.find((p) => p.id === issue.projectId));

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

	<div class="foot">Created {shortDate(issue.createdAt)}</div>
</div>

<style>
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 11px 12px 10px;
		display: flex;
		flex-direction: column;
		gap: 7px;
		cursor: pointer;
		transition: border-color 0.12s, background 0.12s, box-shadow 0.3s;
	}
	.card:hover {
		border-color: var(--border-strong);
		background: var(--bg-elev2);
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
		color: var(--text-faint);
		font-family: var(--mono);
		letter-spacing: -0.02em;
	}
	.nodoc {
		font-size: 10px;
		color: var(--text-faint);
		opacity: 0.6;
	}
	.epic {
		font-size: 10.5px;
		font-family: var(--mono);
		color: var(--accent2);
		background: color-mix(in srgb, var(--accent2) 15%, transparent);
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
		border: 1.4px dashed var(--border-strong);
		flex: none;
	}
	.assignee.on {
		border-style: solid;
		background: linear-gradient(145deg, #4a4f64, #2a2f3d);
	}
	.title {
		font-size: 14px;
		line-height: 1.45;
		font-weight: 500;
		color: var(--text);
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
		color: color-mix(in srgb, var(--accent2) 55%, var(--text));
		background: color-mix(in srgb, var(--accent2) 13%, var(--bg-elev));
		border: 1px solid color-mix(in srgb, var(--accent2) 32%, var(--border));
		white-space: nowrap;
		cursor: pointer;
	}
	.epictag :global(svg) {
		color: var(--accent2);
		flex: none;
	}
	.epictag:hover {
		background: color-mix(in srgb, var(--accent2) 22%, var(--bg-elev));
		border-color: var(--accent2);
	}
	.foot {
		font-size: 12px;
		color: var(--text-faint);
		margin-top: 1px;
	}
</style>
