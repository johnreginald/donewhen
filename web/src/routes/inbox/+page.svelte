<script>
	import { onMount, tick } from 'svelte';
	import { get } from 'svelte/store';
	import { aiName, states, projectById, inboxCount, inboxTotal, refreshInbox, blockLinks, issues, openBlockersByIssue } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { openIssue, onLive, paletteOpen } from '$lib/ui.js';
	import { isTypingTarget } from '$lib/shortcuts.js';
	import PageHeader from '$components/PageHeader.svelte';
	import InboxReviewCard from '$components/InboxReviewCard.svelte';
	import InboxBlockedCard from '$components/InboxBlockedCard.svelte';
	import InboxActivityList from '$components/InboxActivityList.svelte';
	import InboxToast from '$components/InboxToast.svelte';

	// ---- state ----
	let loading = $state(true); // the full-page skeleton — only for the first load
	let error = $state(null); // full-page error, set only by a non-background load
	let needsReview = $state([]);
	let waiting = $state([]); // counts toward the badge, shown nowhere on this page
	let blockedList = $state([]);
	let recent = $state([]);
	let seenAt = $state(null);

	let criteriaById = $state({}); // issueId -> { loading, items }
	let expandedIds = $state(new Set());
	let bounceOpenId = $state(null);
	let bounceText = $state('');
	let busyId = $state('');
	let selectedIndex = $state(0);

	let replyOpenId = $state(null);
	let replyText = $state('');
	let replyBusyId = $state('');

	let toast = $state(null);

	// Plain (non-reactive) bookkeeping — not UI state, so not $state.
	let seenStamped = false; // guards a single inboxSeen() per page visit
	let refreshTimer;
	let toastTimer;

	const newActivityCount = $derived(
		seenAt ? recent.filter((a) => a.createdAt > seenAt).length : recent.length
	);
	const isEmpty = $derived(!needsReview.length && !blockedList.length && !recent.length);

	// ---- data loading ----
	// background=true is a live refresh: it must never flash the skeleton, wipe
	// good data on a transient failure, or re-stamp "seen" (that would erase the
	// new-since-last-visit marks moments after they appear).
	async function load({ background = false } = {}) {
		let ok = false;
		if (!background) {
			loading = true;
			error = null;
		}
		try {
			const r = (await refreshInbox()) || {}; // also sets the sidebar badge
			needsReview = r.needsReview || [];
			waiting = r.waiting || [];
			recent = r.recent || [];
			seenAt = r.seenAt || null;
			clampSelection();

			await Promise.all([loadCriteria(needsReview), loadBlocked()]);
			ok = true;
		} catch (e) {
			if (background) {
				flashToast("Couldn't refresh the inbox.", 'error');
			} else {
				error = e?.message || "Couldn't load the inbox.";
			}
		} finally {
			if (!background) loading = false;
		}
		// Stamp "seen" only once the list has been drawn, and only if it loaded:
		// a failed load must not mark unseen activity as seen.
		if (ok && !seenStamped) {
			seenStamped = true;
			await tick();
			try {
				await api.inboxSeen();
			} catch {
				seenStamped = false; // let the next successful load retry the stamp
			}
		}
	}

	async function loadCriteria(items) {
		await Promise.all(
			items.map(async (it) => {
				criteriaById[it.id] = { loading: true, items: criteriaById[it.id]?.items || [] };
				try {
					const cs = (await api.criteria(it.id)) || [];
					criteriaById[it.id] = { loading: false, items: cs };
				} catch {
					criteriaById[it.id] = { loading: false, items: criteriaById[it.id]?.items || [] };
				}
			})
		);
	}

	async function loadBlocked() {
		const blockedState = get(states).find((s) => s.name === 'Blocked');
		if (!blockedState) {
			blockedList = [];
			return;
		}
		const list = (await api.issues({ state: blockedState.id })) || [];
		blockedList = await Promise.all(
			list.map(async (is) => {
				let reason = '';
				try {
					const cs = (await api.comments(is.id)) || [];
					const last = cs.length ? cs[cs.length - 1] : null;
					if (last) reason = excerptPlain(last.bodyMd, 220);
				} catch {
					/* leave reason blank — the card shows a fallback line */
				}
				return { ...is, reason };
			})
		);
	}

	function excerptPlain(md, max = 220) {
		if (!md) return '';
		const plain = md
			.replace(/```[\s\S]*?```/g, ' ')
			.replace(/!\[[^\]]*]\([^)]*\)/g, ' ')
			.replace(/\[([^\]]*)]\([^)]*\)/g, '$1')
			.replace(/[*_`>#-]/g, '')
			.replace(/\s+/g, ' ')
			.trim();
		return plain.length > max ? plain.slice(0, max).trim() + '…' : plain;
	}

	function blockedByKeyFor(issueId) {
		const ids = openBlockersByIssue(get(blockLinks))[issueId];
		if (!ids || !ids.length) return '';
		return get(issues).find((i) => i.id === ids[0])?.key || '';
	}

	function clampSelection() {
		if (selectedIndex >= needsReview.length) selectedIndex = Math.max(0, needsReview.length - 1);
	}

	// ---- toast (own component: needs an Undo slot the shared one doesn't have) ----
	function flashToast(message, kind = 'info', { mono, onUndo, ms = 4000 } = {}) {
		toast = { message, kind, mono, onUndo };
		clearTimeout(toastTimer);
		toastTimer = setTimeout(() => (toast = null), ms);
	}

	// ---- approve / undo ----
	async function approve(item) {
		if (busyId) return;
		const c = criteriaById[item.id];
		const openCount = c ? c.items.filter((x) => !x.done).length : 0;
		if (c?.loading || openCount > 0) return; // belt-and-braces; the button is disabled too
		const doneState = get(states).find((s) => s.name === 'Done');
		if (!doneState) {
			flashToast('No "Done" state in this workspace.', 'error');
			return;
		}
		const prevStateId = item.stateId;
		busyId = item.id;
		try {
			await api.updateIssue(item.id, { stateId: doneState.id });
			needsReview = needsReview.filter((x) => x.id !== item.id);
			inboxCount.set(inboxTotal({ needsReview, waiting }));
			clampSelection();
			flashToast('→ Done · by you', 'info', {
				mono: item.key,
				onUndo: () => undoApprove(item, prevStateId)
			});
		} catch (e) {
			flashToast(e?.message || 'Approve failed.', 'error');
		} finally {
			busyId = '';
		}
	}

	async function undoApprove(item, prevStateId) {
		clearTimeout(toastTimer);
		toast = null;
		try {
			await api.updateIssue(item.id, { stateId: prevStateId });
			flashToast(`${item.key} restored to In Review.`);
			load({ background: true });
		} catch (e) {
			flashToast(e?.message || 'Undo failed.', 'error');
		}
	}

	// ---- bounce ----
	function openBounce(item) {
		bounceOpenId = item.id;
		bounceText = '';
	}
	function cancelBounce() {
		bounceOpenId = null;
		bounceText = '';
	}
	function toggleBounce(item) {
		if (bounceOpenId === item.id) cancelBounce();
		else openBounce(item);
	}

	async function confirmBounce(item) {
		if (busyId) return;
		const progState = get(states).find((s) => s.name === 'In Progress');
		if (!progState) {
			flashToast('No "In Progress" state in this workspace.', 'error');
			return;
		}
		const text = bounceText.trim();
		busyId = item.id;
		try {
			await api.updateIssue(item.id, { stateId: progState.id });
			if (text) await api.addComment(item.id, text);
			needsReview = needsReview.filter((x) => x.id !== item.id);
			inboxCount.set(inboxTotal({ needsReview, waiting }));
			cancelBounce();
			clampSelection();
			flashToast(`${item.key} → In Progress${text ? ' · comment posted' : ''}`);
		} catch (e) {
			flashToast(e?.message || 'Bounce failed.', 'error');
		} finally {
			busyId = '';
		}
	}

	function toggleExpand(item) {
		const next = new Set(expandedIds);
		if (next.has(item.id)) next.delete(item.id);
		else next.add(item.id);
		expandedIds = next;
	}

	// ---- blocked → reply ----
	function toggleReply(item) {
		if (replyOpenId === item.id) {
			replyOpenId = null;
			replyText = '';
		} else {
			replyOpenId = item.id;
			replyText = '';
		}
	}

	async function sendReply(item) {
		const text = replyText.trim();
		if (!text || replyBusyId) return;
		replyBusyId = item.id;
		try {
			await api.addComment(item.id, text);
			blockedList = blockedList.map((b) => (b.id === item.id ? { ...b, reason: text } : b));
			replyOpenId = null;
			replyText = '';
			flashToast(`Reply posted on ${item.key}.`);
		} catch (e) {
			flashToast(e?.message || 'Reply failed.', 'error');
		} finally {
			replyBusyId = '';
		}
	}

	// ---- keyboard: j/k navigate, a approve, b bounce ----
	function onKeydown(e) {
		if (e.metaKey || e.ctrlKey || e.altKey) return;
		if (isTypingTarget(e.target)) return;
		if (get(paletteOpen)) return;
		const key = e.key.toLowerCase();
		if (key === 'j' || key === 'k') {
			if (!needsReview.length) return;
			e.preventDefault();
			selectedIndex =
				key === 'j' ? Math.min(selectedIndex + 1, needsReview.length - 1) : Math.max(selectedIndex - 1, 0);
			return;
		}
		if (key === 'a') {
			const it = needsReview[selectedIndex];
			if (it) {
				e.preventDefault();
				approve(it);
			}
			return;
		}
		if (key === 'b') {
			const it = needsReview[selectedIndex];
			if (it) {
				e.preventDefault();
				toggleBounce(it);
			}
		}
	}

	// ---- live refresh: any of these means the queues may be stale ----
	const LIVE_TYPES = new Set([
		'issue.created',
		'issue.updated',
		'issue.state_changed',
		'issue.deleted',
		'comment.added',
		'issue.blockers'
	]);

	function scheduleRefresh() {
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(() => load({ background: true }), 400);
	}

	onMount(() => {
		load();
		const offLive = onLive((ev) => {
			if (ev && LIVE_TYPES.has(ev.type)) scheduleRefresh();
		});
		window.addEventListener('keydown', onKeydown);
		return () => {
			offLive();
			window.removeEventListener('keydown', onKeydown);
			clearTimeout(refreshTimer);
			clearTimeout(toastTimer);
		};
	});
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Inbox' }]} />
	<div class="pg-body">
		<div class="inbox">
			<div class="content-inner">
				<div class="pagehead">
					<span class="eyebrow">DoneWhen · Inbox</span>
					<h1>Inbox</h1>
					<span class="sub faint">What {$aiName} did while you were away</span>
				</div>

				{#if loading}
					<div class="skel-body">
						<div class="skel-bar" style="width:120px;height:10px"></div>
						{#each [1, 2, 3] as n (n)}
							<div class="skel-card">
								<div class="skel-bar" style="width:60%"></div>
								<div class="skel-bar" style="width:38%;height:7px"></div>
								<div class="skel-bar" style="width:22%;height:7px"></div>
							</div>
						{/each}
					</div>
				{:else if error}
					<div class="error-wrap">
						<div class="h">Couldn't load the inbox</div>
						<div class="s faint">The server didn't respond. Your review queue hasn't changed.</div>
						<button class="btn sd" onclick={() => load()}>Retry</button>
					</div>
				{:else if isEmpty}
					<div class="empty-wrap">
						<div class="h">Nothing waiting on you.</div>
						<div class="s faint">
							{$aiName} hasn't moved anything to review. Come back when something's ready.
						</div>
					</div>
				{:else}
				<div class="ib-cols">
				<div class="ib-main">
					<div class="sec-head">Needs review<span class="count">{needsReview.length}</span></div>
					{#if needsReview.length}
						<div class="nr-list">
							{#each needsReview as it, i (it.id)}
								<InboxReviewCard
									item={it}
									epicName={projectById(it.projectId)?.name ?? ''}
									aiName={$aiName}
									crit={criteriaById[it.id]}
									expanded={expandedIds.has(it.id)}
									selected={i === selectedIndex}
									busy={busyId === it.id}
									bounceOpen={bounceOpenId === it.id}
									bounceText={bounceOpenId === it.id ? bounceText : ''}
									onOpen={(x) => openIssue(x.key)}
									onToggleExpand={toggleExpand}
									onApprove={approve}
									onToggleBounce={toggleBounce}
									onBounceInput={(v) => (bounceText = v)}
									onBounceConfirm={confirmBounce}
									onBounceCancel={cancelBounce}
								/>
							{/each}
						</div>
					{:else}
						<div class="empty faint">Nothing waiting. {$aiName} hasn't moved anything to review yet.</div>
					{/if}

					<div class="sec-head">Blocked — needs you<span class="count">{blockedList.length}</span></div>
					{#if blockedList.length}
						<div class="bl-list">
							{#each blockedList as b (b.id)}
								<InboxBlockedCard
									item={b}
									epicName={projectById(b.projectId)?.name ?? ''}
									blockedByKey={blockedByKeyFor(b.id)}
									aiName={$aiName}
									replying={replyOpenId === b.id}
									replyText={replyOpenId === b.id ? replyText : ''}
									busy={replyBusyId === b.id}
									onOpen={(x) => openIssue(x.key)}
									onToggleReply={toggleReply}
									onReplyInput={(v) => (replyText = v)}
									onReplySend={sendReply}
								/>
							{/each}
						</div>
					{:else}
						<div class="empty faint">Nothing blocked right now.</div>
					{/if}

					<div class="kbdbar">
						<span class="grp"><span class="kbd">j</span><span class="kbd">k</span>navigate</span>
						<span class="grp"><span class="kbd">a</span>approve</span>
						<span class="grp"><span class="kbd">b</span>bounce</span>
					</div>
				</div>
				<aside class="ib-side" aria-label="Recent {$aiName} activity">
					<div class="sec-head">
						Recent {$aiName} activity
						{#if newActivityCount}<span class="count new">{newActivityCount} new</span>{/if}
					</div>
					<InboxActivityList items={recent} {seenAt} aiName={$aiName} />
				</aside>
				</div>
				{/if}
			</div>
		</div>
	</div>
</div>

<InboxToast {toast} />

<style>
	.inbox {
		height: 100%;
		display: flex;
		flex-direction: column;
	}
	.content-inner {
		max-width: 1240px;
		margin: 0 auto;
		padding: 24px clamp(16px, 4vw, 30px) 48px;
		display: flex;
		flex-direction: column;
		gap: 2px;
		width: 100%;
		box-sizing: border-box;
	}
	.pagehead {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-bottom: 2px;
	}
	.pagehead .eyebrow {
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--ink-3);
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}
	.pagehead h1 {
		font-family: var(--serif);
		font-weight: 400;
		font-size: var(--t-xl);
		margin: 2px 0 0;
		letter-spacing: -0.01em;
		color: var(--ink);
	}
	.pagehead .sub {
		font-size: var(--t-sm);
		margin: 2px 0 0;
	}

	.sec-head {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 14px 0 7px;
		font-size: var(--t-xs);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ink-3);
		font-weight: 600;
	}
	.count {
		background: var(--sunken);
		border: 1px solid var(--line);
		color: var(--ink-2);
		border-radius: var(--r-lg);
		padding: 1px 8px;
		font-size: var(--t-xs);
		letter-spacing: 0;
		font-weight: 600;
	}
	.count.new {
		background: var(--accent-soft);
		border-color: var(--accent);
		color: var(--ink);
	}

	.nr-list,
	.bl-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.empty {
		padding: 16px 4px 28px;
		font-size: var(--t-sm);
	}

	.kbdbar {
		display: flex;
		align-items: center;
		gap: 12px;
		margin-top: 16px;
		padding-top: 8px;
		border-top: 1px solid var(--line);
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.kbdbar .grp {
		display: inline-flex;
		align-items: center;
		gap: 5px;
	}
	.kbd {
		font-family: var(--mono);
		font-size: var(--t-xs);
		padding: 1px 5px;
		border: 1px solid var(--line-strong);
		border-bottom-width: 2px;
		border-radius: var(--r-sm);
		color: var(--ink-2);
		background: var(--surface);
	}

	.btn {
		height: 30px;
		padding: 0 12px;
		border-radius: var(--r);
		font: 500 var(--t-sm) / 1 var(--font);
		display: inline-flex;
		align-items: center;
		gap: 6px;
		border: 1px solid transparent;
		cursor: pointer;
	}
	.btn.sd {
		background: var(--surface);
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.btn.sd:hover {
		background: var(--hover);
	}

	/* loading / error / empty — full-content-area states (Frames 4–6) */
	.empty-wrap,
	.error-wrap {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		padding: 60px 30px;
		gap: 8px;
	}
	.empty-wrap .h {
		font-family: var(--serif);
		font-size: var(--t-2xl);
		font-weight: 400;
		color: var(--ink);
	}
	.empty-wrap .s {
		font-size: var(--t-sm);
		max-width: 34ch;
	}
	.error-wrap .h {
		font-size: var(--t-base);
		font-weight: 600;
		color: var(--ink);
	}
	.error-wrap .s {
		font-size: var(--t-sm);
		max-width: 30ch;
	}
	.error-wrap .btn {
		margin-top: 4px;
	}

	.skel-body {
		padding: 16px 0;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.skel-card {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.skel-bar {
		background: var(--sunken);
		border-radius: var(--r-sm);
		height: 9px;
		position: relative;
		overflow: hidden;
	}
	.skel-bar::after {
		content: '';
		position: absolute;
		inset: 0;
		background: linear-gradient(90deg, transparent, var(--hover), transparent);
		animation: sweep 1.6s ease-in-out infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.skel-bar::after {
			animation: none;
		}
	}
	@keyframes sweep {
		0% {
			transform: translateX(-100%);
		}
		100% {
			transform: translateX(100%);
		}
	}

	/* Two sides: what needs you on the left, what the AI did on the right. */
	.ib-cols {
		display: grid;
		grid-template-columns: minmax(0, 1.35fr) minmax(0, 1fr);
		gap: 0 40px;
		align-items: start;
	}
	.ib-main,
	.ib-side {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.ib-side {
		position: sticky;
		top: 0;
		max-height: calc(100dvh - 120px);
		overflow-y: auto;
		padding-left: 24px;
		border-left: 1px solid var(--line);
	}
	@media (max-width: 1000px) {
		.ib-cols {
			grid-template-columns: minmax(0, 1fr);
		}
		.ib-side {
			position: static;
			max-height: none;
			overflow: visible;
			padding-left: 0;
			border-left: 0;
		}
	}
	@media (max-width: 720px) {
		.content-inner {
			padding: 16px 16px 40px;
		}
		.kbdbar {
			display: none;
		}
	}
</style>
