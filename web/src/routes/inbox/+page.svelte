<script>
	import PageHeader from '$components/PageHeader.svelte';
	import { get } from 'svelte/store';
	import { api } from '$lib/api.js';
	import { states, projectById, inboxCount } from '$lib/store.js';
	import { openIssue, showToast } from '$lib/ui.js';
	import ActivityFeed from '$components/ActivityFeed.svelte';
	import PriorityIcon from '$components/PriorityIcon.svelte';
	import { GitCommitHorizontal, FileText, Check, Undo2, Inbox as InboxIcon } from '@lucide/svelte';

	let needsReview = $state([]);
	let recent = $state([]);
	let seenAt = $state(null);
	let loading = $state(true);
	let busy = $state(''); // issue id currently being acted on

	const newCount = $derived(
		seenAt ? recent.filter((a) => a.createdAt > seenAt).length : recent.length
	);

	async function load() {
		loading = true;
		try {
			const r = (await api.inbox()) || {};
			needsReview = r.needsReview || [];
			recent = r.recent || [];
			seenAt = r.seenAt || null;
			inboxCount.set(needsReview.length);
			// mark reviewed so the "new" highlight resets next visit + badge is a live queue count
			await api.inboxSeen();
		} finally {
			loading = false;
		}
	}

	async function moveTo(item, stateName, label) {
		if (busy) return;
		const st = get_state(stateName);
		if (!st) {
			showToast(`No "${stateName}" state`, 'error');
			return;
		}
		busy = item.id;
		try {
			await api.updateIssue(item.id, { stateId: st.id });
			needsReview = needsReview.filter((x) => x.id !== item.id);
			inboxCount.set(needsReview.length);
			showToast(`${item.key} — ${label}`);
		} catch (e) {
			showToast(e.message || 'Failed', 'error');
		} finally {
			busy = '';
		}
	}

	function get_state(name) {
		return get(states).find((s) => s.name === name);
	}

	function rel(ts) {
		if (!ts) return '';
		const d = (Date.now() - new Date(ts).getTime()) / 1000;
		if (d < 60) return 'just now';
		if (d < 3600) return `${Math.floor(d / 60)}m ago`;
		if (d < 86400) return `${Math.floor(d / 3600)}h ago`;
		return `${Math.floor(d / 86400)}d ago`;
	}

	$effect(() => {
		load();
	});
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Inbox' }]} />
	<div class="pg-body">
<div class="inbox">
	<div class="ib-head">
		<span class="ib-title"><InboxIcon size={17} strokeWidth={2} /> Inbox</span>
		<span class="ib-sub faint">What Clanker did while you were away</span>
	</div>

	<div class="ib-body">
		<!-- Needs review -->
		<section>
			<h2 class="sec">
				Needs review
				{#if needsReview.length}<span class="count">{needsReview.length}</span>{/if}
			</h2>
			{#if needsReview.length}
				<div class="cards">
					{#each needsReview as it (it.id)}
						<div class="card">
							<button class="card-main" onclick={() => openIssue(it.key)}>
								<div class="row1">
									<PriorityIcon priority={it.priority} />
									<span class="key">{it.key}</span>
									<span class="ttl">{it.title}</span>
								</div>
								<div class="meta faint">
									{#if it.projectId}<span>{projectById(it.projectId)?.name ?? 'Epic'}</span
										><span class="dot">·</span>{/if}
									<span class="mv">moved to review {rel(it.enteredReviewAt)}</span>
									{#if it.commitCount}<span class="dot">·</span><span class="ic"
											><GitCommitHorizontal size={13} /> {it.commitCount}</span
										>{/if}
									{#if it.docCount}<span class="dot">·</span><span class="ic"
											><FileText size={13} /> {it.docCount}</span
										>{/if}
								</div>
							</button>
							<div class="acts">
								<button
									class="act approve"
									disabled={busy === it.id}
									onclick={() => moveTo(it, 'Done', 'approved → Done')}
									title="Approve → Done"><Check size={15} strokeWidth={2.4} /> Approve</button
								>
								<button
									class="act bounce"
									disabled={busy === it.id}
									onclick={() => moveTo(it, 'In Progress', 'bounced → In Progress')}
									title="Bounce → In Progress"><Undo2 size={15} strokeWidth={2.2} /> Bounce</button
								>
							</div>
						</div>
					{/each}
				</div>
			{:else}
				<div class="empty faint">
					{loading ? 'Loading…' : 'Nothing waiting. Clanker hasn’t moved anything to In Review yet.'}
				</div>
			{/if}
		</section>

		<!-- Recent Clanker activity -->
		<section>
			<h2 class="sec">
				Recent Clanker activity
				{#if newCount}<span class="count new">{newCount} new</span>{/if}
			</h2>
			{#if recent.length}
				<ActivityFeed items={recent} showIssue={true} />
			{:else}
				<div class="empty faint">{loading ? 'Loading…' : 'No Clanker activity yet.'}</div>
			{/if}
		</section>
	</div>
</div>
	</div>
</div>

<style>
	.inbox {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.ib-head {
		display: flex;
		align-items: baseline;
		gap: 12px;
		padding: 14px 20px;
		border-bottom: 1px solid var(--line);
		flex: none;
	}
	.ib-title {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		font-size: 15px;
		font-weight: 600;
	}
	.ib-sub {
		font-size: 12.5px;
	}
	.ib-body {
		flex: 1;
		overflow-y: auto;
		padding: 16px clamp(16px, 4vw, 40px) 48px;
		max-width: 820px;
		width: 100%;
		margin: 0 auto;
	}
	.sec {
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ink-3);
		font-weight: 600;
		margin: 18px 0 10px;
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.count {
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--ink-2);
		border-radius: 20px;
		padding: 1px 8px;
		font-size: 11px;
		letter-spacing: 0;
	}
	.count.new {
		background: color-mix(in srgb, var(--accent) 22%, var(--paper));
		border-color: var(--accent);
		color: var(--ink);
	}
	.cards {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.card {
		display: flex;
		align-items: center;
		gap: 10px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 10px;
		padding: 10px 12px;
	}
	.card:hover {
		border-color: var(--line-strong);
	}
	.card-main {
		flex: 1;
		min-width: 0;
		background: none;
		border: none;
		text-align: left;
		padding: 0;
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.row1 {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.key {
		font-size: 12.5px;
		color: var(--ink-2);
		font-variant-numeric: tabular-nums;
		flex: none;
	}
	.ttl {
		font-size: 14px;
		color: var(--ink);
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		flex-wrap: wrap;
	}
	.meta .ic {
		display: inline-flex;
		align-items: center;
		gap: 3px;
	}
	.dot {
		opacity: 0.5;
	}
	.acts {
		display: flex;
		gap: 6px;
		flex: none;
	}
	.act {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12.5px;
		font-weight: 500;
		padding: 6px 10px;
		border-radius: 7px;
		border: 1px solid var(--line);
		background: var(--paper);
		color: var(--ink-2);
	}
	.act:hover {
		color: var(--ink);
		border-color: var(--line-strong);
	}
	.act.approve:hover {
		border-color: var(--st-done);
		color: var(--st-done);
	}
	.act.bounce:hover {
		border-color: var(--st-review);
		color: var(--st-review);
	}
	.act:disabled {
		opacity: 0.5;
	}
	.empty {
		padding: 28px 10px;
		text-align: center;
		font-size: 13px;
	}
</style>
