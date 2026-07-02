<script>
	import LabelPill from './LabelPill.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import { openIssue } from '$lib/ui.js';

	let { issue } = $props();
</script>

<div
	class="card"
	role="button"
	tabindex="0"
	onclick={() => openIssue(issue.key)}
	onkeydown={(e) => e.key === 'Enter' && openIssue(issue.key)}
>
	<div class="top">
		<span class="key">{issue.key}</span>
		<PriorityIcon priority={issue.priority} />
	</div>
	<div class="title">{issue.title}</div>
	{#if issue.labels && issue.labels.length}
		<div class="labels">
			{#each issue.labels as l (l.id)}
				<LabelPill label={l} />
			{/each}
		</div>
	{/if}
</div>

<style>
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 7px;
		cursor: pointer;
		transition: border-color 0.1s, background 0.1s;
	}
	.card:hover {
		border-color: var(--border-strong);
		background: var(--bg-elev2);
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.key {
		font-size: 12px;
		color: var(--text-faint);
		font-family: var(--mono);
	}
	.title {
		font-size: 13.5px;
		line-height: 1.4;
	}
	.labels {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
</style>
