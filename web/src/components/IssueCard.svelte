<script>
	import LabelPill from './LabelPill.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import StateIcon from './StateIcon.svelte';
	import { openIssue, flashIssueId } from '$lib/ui.js';
	import { states, projects } from '$lib/store.js';

	let { issue } = $props();
	const flashing = $derived($flashIssueId === issue.id);
	const state = $derived($states.find((s) => s.id === issue.stateId));
	const project = $derived($projects.find((p) => p.id === issue.projectId));

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
		<span class="spacer"></span>
		<span class="assignee" class:on={issue.assigneeId}></span>
	</div>

	<div class="title">{issue.title}</div>

	{#if issue.priority || project || (issue.labels && issue.labels.length)}
		<div class="meta">
			{#if issue.priority}
				<PriorityIcon priority={issue.priority} />
			{/if}
			{#if project}
				<span class="pill proj"><span class="pglyph">▢</span>{project.name}</span>
			{/if}
			{#each issue.labels ?? [] as l (l.id)}
				<LabelPill label={l} />
			{/each}
		</div>
	{/if}

	<div class="foot">Created {shortDate(issue.createdAt)}</div>
</div>

<style>
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 10px 11px 9px;
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
		font-size: 12px;
		color: var(--text-faint);
		font-family: var(--mono);
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
		font-size: 13px;
		line-height: 1.4;
		color: var(--text);
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}
	.proj {
		color: var(--text-dim);
		gap: 5px;
	}
	.pglyph {
		font-size: 9px;
		color: var(--text-faint);
	}
	.foot {
		font-size: 11px;
		color: var(--text-faint);
		margin-top: 1px;
	}
</style>
