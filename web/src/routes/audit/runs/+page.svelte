<script>
	// Every agent run in the workspace, newest first.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents } from '$lib/store.js';
	import { onLive } from '$lib/ui.js';
	import PageHeader from '$components/PageHeader.svelte';
	import AuditTabs from '$components/AuditTabs.svelte';
	import RunBlock from '$components/RunBlock.svelte';

	let runs = $state([]);
	let agent = $state('');
	let status = $state('');
	let kind = $state('');

	onMount(async () => {
		runs = (await api.runs({ limit: 200 }).catch(() => [])) || [];
		return onLive((ev) => {
			if (!ev.run) return;
			const i = runs.findIndex((r) => r.id === ev.run.id);
			runs = i >= 0 ? runs.map((r) => (r.id === ev.run.id ? ev.run : r)) : [ev.run, ...runs];
		});
	});
	const shown = $derived(
		runs.filter((r) => (!agent || r.agentId === agent) && (!status || r.status === status) && (!kind || r.kind === kind))
	);
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Audit' }, { label: 'Runs' }]} />
	<AuditTabs />
	<div class="pg-body">
		<div class="filters">
			<select bind:value={agent}>
				<option value="">All agents</option>
				{#each $agents as a (a.id)}<option value={a.id}>{a.name}</option>{/each}
			</select>
			<select bind:value={status}>
				<option value="">All statuses</option>
				{#each ['running', 'succeeded', 'failed', 'aborted'] as s}<option value={s}>{s}</option>{/each}
			</select>
			<select bind:value={kind}>
				<option value="">Work and chat</option>
				<option value="work">Work runs</option>
				<option value="chat">Chat turns</option>
			</select>
			<span class="n">{shown.length} runs</span>
		</div>
		<div class="list">
			{#each shown as r (r.id)}
				<div class="line">
					{#if r.issueKey}<a class="key" href="/issue/{r.issueKey}">{r.issueKey}</a>{/if}
					<div class="rb"><RunBlock run={r} /></div>
				</div>
			{:else}
				<div class="empty">No runs match.</div>
			{/each}
		</div>
	</div>
</div>

<style>
	.filters {
		display: flex;
		gap: 8px;
		align-items: center;
		padding: 12px 20px 4px;
		flex-wrap: wrap;
	}
	select {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 5px 8px;
		font-size: 12.5px;
		color: var(--text);
	}
	.n {
		margin-left: auto;
		font-size: 12px;
		color: var(--text-faint);
	}
	.list {
		padding: 4px 12px 32px;
	}
	.line {
		display: flex;
		align-items: flex-start;
		gap: 8px;
	}
	.key {
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--text-faint);
		padding-top: 8px;
		min-width: 64px;
	}
	.key:hover {
		color: var(--text);
	}
	.rb {
		flex: 1;
		min-width: 0;
	}
	.empty {
		padding: 30px 10px;
		color: var(--text-faint);
		font-size: 13px;
	}
</style>
