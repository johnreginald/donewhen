<script>
	import { issues, states, projects } from '$lib/store.js';
	import { openIssue } from '$lib/ui.js';
	import PriorityIcon from '$components/PriorityIcon.svelte';
	import LabelPill from '$components/LabelPill.svelte';

	const stateName = (id) => $states.find((s) => s.id === id)?.name ?? '';
	const stateColor = (id) => $states.find((s) => s.id === id)?.color ?? '#888';
	const projName = (id) => $projects.find((p) => p.id === id)?.name ?? '';

	const sorted = $derived(
		[...$issues].sort((a, b) => {
			const sa = $states.find((s) => s.id === a.stateId)?.position ?? 0;
			const sb = $states.find((s) => s.id === b.stateId)?.position ?? 0;
			return sa - sb || a.position - b.position;
		})
	);
</script>

<div class="list-wrap">
	<table>
		<thead>
			<tr>
				<th style="width:70px">Key</th>
				<th style="width:24px"></th>
				<th>Title</th>
				<th style="width:130px">Status</th>
				<th style="width:130px">Epic</th>
				<th>Labels</th>
			</tr>
		</thead>
		<tbody>
			{#each sorted as i (i.id)}
				<tr onclick={() => openIssue(i.key)}>
					<td class="key">{i.key}</td>
					<td><PriorityIcon priority={i.priority} /></td>
					<td class="title">{i.title}</td>
					<td>
						<span class="pill"><span class="dot" style:background={stateColor(i.stateId)}></span>{stateName(i.stateId)}</span>
					</td>
					<td class="faint">{projName(i.projectId)}</td>
					<td>
						<div class="labels">
							{#each i.labels as l (l.id)}<LabelPill label={l} />{/each}
						</div>
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
	{#if sorted.length === 0}
		<div class="empty faint">No issues yet. Press ⌘K to create one.</div>
	{/if}
</div>

<style>
	.list-wrap {
		height: 100%;
		overflow: auto;
		padding: 8px 4px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13.5px;
	}
	th {
		text-align: left;
		font-weight: 500;
		color: var(--text-faint);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 8px 12px;
		border-bottom: 1px solid var(--border);
	}
	td {
		padding: 9px 12px;
		border-bottom: 1px solid var(--border);
		vertical-align: middle;
	}
	tr {
		cursor: pointer;
	}
	tbody tr:hover {
		background: var(--bg-elev);
	}
	.key {
		font-family: var(--mono);
		color: var(--text-faint);
		font-size: 12px;
	}
	.labels {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.empty {
		padding: 40px;
		text-align: center;
	}
</style>
