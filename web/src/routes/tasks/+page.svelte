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
	import LabelPill from '$components/LabelPill.svelte';

	const stOf = (id) => $states.find((s) => s.id === id);
	const epicName = (id) => $projects.find((p) => p.id === id)?.name;

	const groups = $derived(byDay($visibleIssues));
	const loading = $derived($states.length === 0);

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
	<PageHeader crumbs={[{ label: 'Tasks', href: '/board' }, { label: 'Tasks' }]} />
	<IssuesToolbar />
	<div class="scroll">
		{#if loading}
			{#each Array(2) as _, gi (gi)}
				<div class="day"><span class="skel" style="width:70px;height:10px"></span></div>
				{#each Array(4) as _2, i (i)}
					<div class="row skrow">
						<span class="skel" style="width:14px;height:14px;border-radius:50%"></span>
						<span class="skel" style="width:{45 + ((i + gi * 4) * 11) % 40}%;height:12px"></span>
					</div>
				{/each}
			{/each}
		{:else if !groups.length}
			<div class="empty">
				<div class="ic">—</div>
				<p class="etitle">No tasks match</p>
				<p class="esub">Try clearing filters, or press ⌘K to create one.</p>
			</div>
		{/if}
		{#if !loading}
		{#each groups as g (g.label)}
			<div class="day"><span>{g.label}</span></div>
			{#each g.items as is (is.id)}
				{@const st = stOf(is.stateId)}
				<button class="row" onclick={() => openIssue(is.key)}>
					<StateIcon category={st?.category} color={st?.color} />
					<span class="title">{is.title}</span>
					{#if is.labels.length}
						<span class="lbls">{#each is.labels as l (l.id)}<LabelPill label={l} />{/each}</span>
					{/if}
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
		{/if}
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
		color: var(--ink-3);
		font-size: var(--t-xs);
		font-family: var(--mono);
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}
	.day::before,
	.day::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--line);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		min-height: 36px;
		background: none;
		border: none;
		color: var(--ink);
		padding: 7px 10px;
		border-radius: var(--r);
		text-align: left;
		font-size: var(--t-sm);
	}
	.row:hover {
		background: var(--hover);
	}
	.row:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.title {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.lbls {
		display: flex;
		gap: 4px;
		flex-shrink: 0;
		max-width: 30%;
		overflow: hidden;
	}
	.epic {
		font-size: var(--t-sm);
		color: var(--ink-3);
		border: 1px solid var(--line);
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
		color: var(--ink-3);
		font-size: var(--t-sm);
	}
	.key {
		font-family: var(--mono);
		font-size: var(--t-xs);
	}
	.when {
		min-width: 56px;
		text-align: right;
	}
	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 9px;
		padding: 60px 24px;
		text-align: center;
	}
	.ic {
		width: 38px;
		height: 38px;
		border-radius: 50%;
		border: 1.5px dashed var(--line-strong);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--ink-3);
		font-size: var(--t-base);
	}
	.etitle {
		margin: 0;
		font-size: var(--t-base);
		font-weight: 600;
		color: var(--ink);
	}
	.esub {
		margin: 0;
		font-size: var(--t-sm);
		color: var(--ink-3);
		max-width: 260px;
		line-height: 1.45;
	}

	/* loading */
	.skel {
		background: linear-gradient(90deg, var(--sunken) 25%, var(--hover) 37%, var(--sunken) 63%);
		background-size: 400% 100%;
		animation: skshim 1.6s ease infinite;
		border-radius: var(--r-sm);
		display: inline-block;
	}
	@keyframes skshim {
		0% {
			background-position: 100% 0;
		}
		100% {
			background-position: 0 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.skel {
			animation: none;
		}
	}
	.skrow {
		cursor: default;
	}

	@media (max-width: 720px) {
		.scroll {
			padding: 4px 10px 24px;
		}
		.epic,
		.when,
		.lbls {
			display: none;
		}
	}
</style>
