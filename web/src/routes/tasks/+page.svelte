<script>
	// Tasks, as Paperclip lists them: newest activity first, grouped under a day
	// divider. The epic list and the Kanban board are one toggle away.
	import { visibleIssues, states, projects } from '$lib/store.js';
	import { openIssue } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import IssuesToolbar from '$components/IssuesToolbar.svelte';
	import StateIcon from '$components/StateIcon.svelte';
	import PriorityIcon from '$components/PriorityIcon.svelte';

	const stOf = (id) => $states.find((s) => s.id === id);
	const epicName = (id) => $projects.find((p) => p.id === id)?.name;

	const groups = $derived(byDay($visibleIssues));

	function dayLabel(d) {
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const day = new Date(d);
		day.setHours(0, 0, 0, 0);
		const diff = Math.round((today - day) / 86400000);
		if (diff === 0) return 'Today';
		if (diff === 1) return 'Yesterday';
		if (diff < 7) return day.toLocaleDateString('en-US', { weekday: 'long' });
		return day.toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: day.getFullYear() === today.getFullYear() ? undefined : 'numeric'
		});
	}

	function byDay(list) {
		const sorted = [...list].sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
		const out = [];
		for (const is of sorted) {
			const label = dayLabel(is.updatedAt);
			if (!out.length || out[out.length - 1].label !== label) out.push({ label, items: [] });
			out[out.length - 1].items.push(is);
		}
		return out;
	}
</script>

<div class="page">
	<PageHeader crumbs={[{ label: 'Tasks' }]} />
	<IssuesToolbar />
	<div class="scroll">
		{#if !groups.length}
			<div class="empty">No tasks match.</div>
		{/if}
		{#each groups as g (g.label)}
			<div class="day"><span>{g.label}</span></div>
			{#each g.items as is (is.id)}
				{@const st = stOf(is.stateId)}
				<button class="row" onclick={() => openIssue(is.key)}>
					<StateIcon category={st?.category} color={st?.color} />
					<span class="title">{is.title}</span>
					{#if is.projectId && epicName(is.projectId)}
						<span class="epic">{epicName(is.projectId)}</span>
					{/if}
					<span class="meta">
						{#if is.priority}<PriorityIcon priority={is.priority} />{/if}
						<span class="key">{is.key}</span>
						<span class="when">{rel(is.updatedAt)}</span>
					</span>
				</button>
			{/each}
		{/each}
	</div>
</div>

<style>
	.page {
		height: 100%;
		display: flex;
		flex-direction: column;
	}
	.scroll {
		flex: 1;
		overflow-y: auto;
		padding: 4px 20px 32px;
	}
	.day {
		display: flex;
		align-items: center;
		gap: 12px;
		margin: 14px 0 4px;
		color: var(--text-faint);
		font-size: 11px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	.day::before,
	.day::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text);
		padding: 7px 10px;
		border-radius: 7px;
		text-align: left;
		font-size: 13.5px;
	}
	.row:hover {
		background: var(--bg-hover);
	}
	.title {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.epic {
		font-size: 12px;
		color: var(--text-faint);
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 0 8px;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.meta {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: 12px;
		flex-shrink: 0;
		color: var(--text-faint);
		font-size: 12px;
	}
	.key {
		font-family: var(--mono);
		font-size: 11.5px;
	}
	.when {
		min-width: 56px;
		text-align: right;
	}
	.empty {
		color: var(--text-faint);
		padding: 40px 0;
		text-align: center;
	}
	@media (max-width: 720px) {
		.scroll {
			padding: 4px 10px 24px;
		}
		.epic,
		.when {
			display: none;
		}
	}
</style>
