<script>
	import PageHeader from '$components/PageHeader.svelte';
	import { api } from '$lib/api.js';
	import { activeInitiative, initiatives } from '$lib/store.js';
	import ActivityFeed from '$components/ActivityFeed.svelte';

	let items = $state([]);
	let actor = $state(''); // '' all | human | ai
	let loading = $state(false);

	const scope = $derived(
		$activeInitiative ? ($initiatives.find((i) => i.id === $activeInitiative)?.name ?? 'Project') : 'All issues'
	);

	async function load(a, ini) {
		loading = true;
		try {
			items =
				(await api.activity({ initiative: ini || undefined, actor: a || undefined, limit: 300 })) || [];
		} finally {
			loading = false;
		}
	}
	$effect(() => {
		load(actor, $activeInitiative);
	});
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Audit' }, { label: 'Activity' }]} />
	<div class="pg-body">
<div class="log">
	<div class="log-head">
		<div class="lh-left">
			<span class="lh-title">Activities</span>
			<span class="lh-scope">{scope}</span>
		</div>
		<div class="seg">
			<button class="sg" class:on={actor === ''} onclick={() => (actor = '')}>All</button>
			<button class="sg" class:on={actor === 'human'} onclick={() => (actor = 'human')}>You</button>
			<button class="sg" class:on={actor === 'ai'} onclick={() => (actor = 'ai')}>✦ Clanker</button>
		</div>
	</div>
	<div class="log-body">
		{#if items.length}
			<ActivityFeed {items} showIssue={true} />
		{:else}
			<div class="empty faint">{loading ? 'Loading…' : 'No activity recorded yet.'}</div>
		{/if}
	</div>
</div>
	</div>
</div>

<style>
	.log {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.log-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 14px 20px;
		border-bottom: 1px solid var(--border);
		flex: none;
	}
	.lh-left {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}
	.lh-title {
		font-size: 15px;
		font-weight: 600;
	}
	.lh-scope {
		font-size: 12.5px;
		color: var(--text-faint);
	}
	.seg {
		display: flex;
		gap: 2px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2px;
	}
	.sg {
		padding: 4px 12px;
		border-radius: 6px;
		font-size: 13px;
		color: var(--text-dim);
		background: none;
		border: none;
	}
	.sg:hover {
		color: var(--text);
	}
	.sg.on {
		background: var(--bg-hover);
		color: var(--text);
	}
	.log-body {
		flex: 1;
		overflow-y: auto;
		padding: 10px clamp(16px, 4vw, 40px) 40px;
		max-width: 820px;
		width: 100%;
		margin: 0 auto;
	}
	.empty {
		padding: 40px;
		text-align: center;
	}
</style>
