<script>
	import { aiName } from '$lib/store.js';
	import PageHeader from '$components/PageHeader.svelte';
	import { onMount } from 'svelte';
	import { api, getWorkspace } from '$lib/api.js';
	import { activeInitiative, initiatives } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import LogTimeline from '$components/LogTimeline.svelte';
	import { autohide } from '$lib/autohide.js';

	const TYPE_CHIPS = [
		{ key: '', label: 'All types' },
		{ key: 'state_changed', label: 'State' },
		{ key: 'label_changed', label: 'Label' },
		{ key: 'assignee_changed', label: 'Assignee' },
		{ key: 'description_changed', label: 'Description' },
		{ key: 'commit_linked', label: 'Commits' },
		{ key: 'criterion_checked', label: 'Criteria' },
		{ key: 'artifact_written', label: 'Docs saved' }
	];

	let items = $state([]);
	let actor = $state(''); // '' all | human | ai
	let typeFilter = $state('');
	let loading = $state(false);
	let error = $state(null);
	let newSince = $state(null); // ISO timestamp of the viewer's last visit, captured once on mount

	const scope = $derived(
		$activeInitiative ? ($initiatives.find((i) => i.id === $activeInitiative)?.name ?? 'Project') : 'All issues'
	);
	const filtered = $derived(typeFilter ? items.filter((a) => a.kind === typeFilter) : items);

	let loadSeq = 0;
	async function load(a, ini) {
		const seq = ++loadSeq;
		loading = true;
		error = null;
		try {
			const list = await api.activity({ initiative: ini || undefined, actor: a || undefined, limit: 300 });
			if (seq !== loadSeq) return; // a newer request already landed
			items = list || [];
		} catch (e) {
			if (seq !== loadSeq) return;
			error = e?.message || 'Failed to load the log.';
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}
	$effect(() => {
		load(actor, $activeInitiative);
	});

	// "New since your last visit": read the stamp from the prior visit before
	// overwriting it, scoped per workspace so switching workspaces doesn't
	// cross-contaminate the divider.
	function lastVisitKey() {
		return 'donewhen_log_last_visit:' + (getWorkspace() || 'default');
	}
	onMount(() => {
		try {
			newSince = localStorage.getItem(lastVisitKey());
			localStorage.setItem(lastVisitKey(), new Date().toISOString());
		} catch {
			/* private mode: no divider, no harm */
		}
		return onLive((ev) => {
			if (
				ev.type?.startsWith('issue.') ||
				ev.type === 'comment.added' ||
				ev.type?.startsWith('document.')
			) {
				load(actor, $activeInitiative);
			}
		});
	});
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Log' }]} />
	<div class="pg-body">
<div class="log">
	<div class="log-head" use:autohide>
		<div class="lh-left">
			<span class="lh-title">Log</span>
			<span class="lh-scope">{scope}</span>
		</div>
		<div class="seg">
			<button class="sg" class:on={actor === ''} onclick={() => (actor = '')}>All</button>
			<button class="sg" class:on={actor === 'human'} onclick={() => (actor = 'human')}>You</button>
			<button class="sg" class:on={actor === 'ai'} onclick={() => (actor = 'ai')}>✦ {$aiName}</button>
		</div>
	</div>
	<div class="log-filters" use:autohide>
		<span class="rh">Filter</span>
		<div class="tchips">
			{#each TYPE_CHIPS as c (c.key)}
				<button class="tchip" class:on={typeFilter === c.key} onclick={() => (typeFilter = c.key)}>{c.label}</button>
			{/each}
		</div>
	</div>
	<div class="log-body">
		<div class="lwrap">
			{#if error}
				<div class="err">
					{error}
					<button class="btn ghost" onclick={() => load(actor, $activeInitiative)}>Retry</button>
				</div>
			{:else if loading && !items.length}
				<div class="sk">
					{#each Array(6) as _, i (i)}
						<div class="skrow"><span class="skb ic"></span><span class="skb line" style:width={70 - i * 6 + '%'}></span></div>
					{/each}
				</div>
			{:else if filtered.length}
				<LogTimeline items={filtered} {newSince} showIssue={true} />
			{:else}
				<div class="empty faint">{items.length ? 'No activity matches this filter.' : 'No activity recorded yet.'}</div>
			{/if}
		</div>
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
		border-bottom: 1px solid var(--line);
		flex: none;
	}
	.lh-left {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}
	.lh-title {
		font-family: var(--serif);
		font-size: var(--t-lg);
		color: var(--ink);
	}
	.lh-scope {
		font-size: var(--t-sm);
		color: var(--ink-3);
	}
	.seg {
		display: flex;
		gap: 2px;
		background: var(--sunken);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		padding: 2px;
	}
	.sg {
		height: 26px;
		padding: 0 12px;
		border-radius: var(--r-sm);
		font-size: var(--t-sm);
		color: var(--ink-2);
		background: none;
		border: none;
	}
	.sg:hover {
		color: var(--ink);
	}
	.sg.on {
		background: var(--surface);
		color: var(--ink);
		font-weight: 500;
		box-shadow: var(--shadow-1);
	}
	.log-filters {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 11px clamp(16px, 4vw, 40px);
		border-bottom: 1px solid var(--line);
		flex: none;
		overflow-x: auto;
	}
	.rh {
		font-family: var(--mono);
		font-size: var(--t-xs);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-3);
		flex: none;
	}
	.tchips {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.tchip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 27px;
		padding: 0 10px;
		border-radius: 999px;
		border: 1px solid var(--line);
		background: var(--surface);
		font-size: var(--t-sm);
		color: var(--ink-2);
		box-sizing: border-box;
		white-space: nowrap;
	}
	.tchip:hover {
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.tchip.on {
		background: var(--accent-soft);
		border-color: var(--accent);
		color: var(--ink);
	}
	.log-body {
		flex: 1;
		overflow-y: auto;
		padding: 0 clamp(16px, 4vw, 40px) 40px;
	}
	.lwrap {
		max-width: 720px;
		margin: 0 auto;
	}
	.empty {
		padding: 40px;
		text-align: center;
	}
	.err {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px;
		margin-top: 14px;
		background: var(--danger-soft);
		border: 1px solid var(--danger);
		border-radius: var(--r);
		color: var(--danger);
		font-size: var(--t-sm);
	}
	.sk {
		padding-top: 14px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.skrow {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.skb {
		border-radius: var(--r-sm);
		background: var(--line);
	}
	.skb.ic {
		width: 23px;
		height: 23px;
		border-radius: 50%;
		flex: none;
	}
	.skb.line {
		height: 10px;
	}
	@media (prefers-reduced-motion: no-preference) {
		.skb {
			animation: skshim 1.6s ease-in-out infinite;
		}
	}
	@keyframes skshim {
		0%,
		100% {
			opacity: 0.55;
		}
		50% {
			opacity: 1;
		}
	}
</style>
