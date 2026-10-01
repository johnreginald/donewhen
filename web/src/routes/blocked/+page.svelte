<script>
	// Blocked — every ticket sitting in the Blocked state, workspace-wide (not
	// scoped to the current epic/label filter — a blocker worth seeing might
	// live in a different epic). Each card: the reason it was blocked with,
	// who blocked it and since when, the tickets it still waits on, and the
	// actions that matter: open, move back to In Progress, reply.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { states, aiName, loadBlockLinks } from '$lib/store.js';
	import { openIssue, showToast, onLive } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import StateIcon from '$components/StateIcon.svelte';

	const stOf = (id) => $states.find((s) => s.id === id);

	let loading = $state(true);
	let items = $state([]); // GET /api/blocked: oldest blocked first, reason attached
	let query = $state('');
	let busy = $state(new Set()); // issue ids mid-unblock
	let replyFor = $state(''); // issue id whose reply box is open
	let replyText = $state('');
	let replying = $state(false);

	async function load() {
		try {
			items = (await api.blocked()) || [];
		} catch (e) {
			if (e?.status !== 401 && !e?.network) showToast("Couldn't load blocked issues: " + (e?.message || e), 'error');
		} finally {
			loading = false;
		}
	}
	onMount(() => {
		load();
		loadBlockLinks();
		// A state change, comment or blockers edit anywhere may add or drop a card.
		let timer;
		const off = onLive((ev) => {
			if (!['issue.state_changed', 'issue.deleted', 'issue.blockers', 'comment.added'].includes(ev.type)) return;
			clearTimeout(timer);
			timer = setTimeout(load, 200);
		});
		return () => {
			off();
			clearTimeout(timer);
		};
	});

	// A plain-text excerpt of a markdown reason: strip the common markdown
	// marks rather than rendering them, then clip to one line's worth.
	function excerpt(md) {
		if (!md) return 'No reason recorded.';
		const text = md
			.replace(/```[\s\S]*?```/g, ' ')
			.replace(/[#>*_`~]/g, '')
			.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
			.replace(/\s+/g, ' ')
			.trim();
		return text.length > 240 ? text.slice(0, 237) + '…' : text || 'No reason recorded.';
	}

	const shown = $derived(
		items.filter((i) => {
			const q = query.trim().toLowerCase();
			return !q || i.title.toLowerCase().includes(q) || i.key.toLowerCase().includes(q);
		})
	);

	async function unblock(i) {
		busy = new Set([...busy, i.id]);
		try {
			await api.updateIssue(i.id, { stateName: 'In Progress' });
			showToast(`${i.key} moved to In Progress`, 'info');
			items = items.filter((x) => x.id !== i.id);
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			const n = new Set(busy);
			n.delete(i.id);
			busy = n;
		}
	}

	function openReply(i) {
		replyFor = replyFor === i.id ? '' : i.id;
		replyText = '';
	}
	async function sendReply(i) {
		const body = replyText.trim();
		if (!body || replying) return;
		replying = true;
		try {
			await api.addComment(i.id, body);
			showToast(`Reply added to ${i.key}`, 'info');
			replyFor = '';
			replyText = '';
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			replying = false;
		}
	}
	function replyKey(e, i) {
		if (e.key === 'Escape') {
			e.stopPropagation();
			replyFor = '';
		} else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			sendReply(i);
		}
	}
</script>

<div class="page">
	<PageHeader crumbs={[{ label: 'Blocked' }]} />
	<div class="tbar">
		<label class="search">
			⌕ <input bind:value={query} placeholder="Search blocked…" />
		</label>
		<span class="spacer"></span>
		<span class="count">
			{#if !loading}{items.length} blocked, oldest first{/if}
		</span>
	</div>

	{#if loading}
		<div class="blist">
			{#each Array(4) as _, i (i)}
				<div class="bcard skel-card">
					<div class="btop">
						<span class="skel" style="width:13px;height:13px;border-radius:50%"></span>
						<span class="skel" style="width:46px;height:11px"></span>
						<span class="skel" style="width:{140 + i * 25}px;height:13px"></span>
					</div>
					<div class="skel" style="width:80%;height:12px;margin:8px 0"></div>
				</div>
			{/each}
		</div>
	{:else if !shown.length}
		<div class="empty">
			<div class="ic">✓</div>
			<p class="etitle">Nothing is blocked</p>
			<p class="esub">Every ticket in this workspace is free to move.</p>
		</div>
	{:else}
		<div class="blist">
			{#each shown as i (i.id)}
				{@const st = stOf(i.stateId)}
				<div class="bcard">
					<div class="btop">
						<StateIcon category={st?.category} color={st?.color} size={13} />
						<span class="k">{i.key}</span>
						<span class="t">{i.title}</span>
						<span class="age" title={new Date(i.since).toLocaleString()}>blocked {rel(i.since)} by {i.actor === 'ai' ? $aiName : 'you'}</span>
					</div>
					<div class="breason"><span class="av">{i.actor === 'ai' ? 'C' : 'Y'}</span><p>{excerpt(i.reason)}</p></div>
					<div class="bfoot">
						{#if i.waitingOn.length}
							<span class="wl">Waiting on</span>
							{#each i.waitingOn as key (key)}
								<button class="wchip" onclick={() => openIssue(key)}>{key}</button>
							{/each}
						{:else}
							<span class="wl">Not waiting on another ticket</span>
						{/if}
						<span class="spacer"></span>
						<button class="btn gho" onclick={() => openReply(i)} aria-expanded={replyFor === i.id}>Reply</button>
						<button class="btn gho" disabled={busy.has(i.id)} onclick={() => unblock(i)}>
							{busy.has(i.id) ? 'Moving…' : 'Back to In Progress'}
						</button>
						<button class="btn sd" onclick={() => openIssue(i.key)}>Open ↗</button>
					</div>
					{#if replyFor === i.id}
						<div class="reply">
							<textarea
								class="textarea"
								rows="2"
								bind:value={replyText}
								onkeydown={(e) => replyKey(e, i)}
								placeholder="Reply on {i.key} (Markdown)"
								aria-label="Reply"
							></textarea>
							<button class="btn sd" disabled={replying || !replyText.trim()} onclick={() => sendReply(i)}>Send</button>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page {
		height: 100%;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.tbar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 9px 20px;
		border-bottom: 1px solid var(--line);
		flex: none;
		flex-wrap: wrap;
	}
	.search {
		display: flex;
		align-items: center;
		gap: 7px;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r);
		padding: 5px 10px;
		color: var(--ink-3);
		width: min(220px, 100%);
		transition:
			border-color 0.12s,
			box-shadow 0.12s;
	}
	/* the wrapper is the field: it shares the .input border and focus ring */
	.search:focus-within {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--focus);
	}
	.search input {
		background: none;
		border: none;
		outline: none;
		font-size: var(--t-base);
		width: 100%;
		color: var(--ink);
	}
	.search input::placeholder {
		color: var(--ink-3);
	}
	.spacer {
		flex: 1;
	}
	.count {
		font-size: var(--t-sm);
		color: var(--ink-3);
		white-space: nowrap;
	}

	.blist {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 14px 20px 24px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.bcard {
		background: var(--surface);
		border: 1px solid var(--line);
		border-left: 3px solid var(--danger);
		border-radius: var(--r);
		padding: 8px 12px 9px;
	}
	.btop {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.btop .k {
		font-family: var(--mono);
		color: var(--ink-3);
		font-size: var(--t-xs);
		flex: none;
	}
	.btop .t {
		font-weight: 500;
		font-size: var(--t-sm);
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.btop .age {
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--danger);
		background: var(--danger-soft);
		padding: 2px 7px;
		border-radius: 999px;
		flex: none;
		white-space: nowrap;
	}
	.breason {
		display: flex;
		gap: 7px;
		margin: 5px 0;
	}
	.breason .av {
		width: 17px;
		height: 17px;
		border-radius: 50%;
		background: var(--accent-soft);
		color: var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font: 600 var(--t-xs) var(--font);
		flex: none;
		margin-top: 1px;
	}
	.breason p {
		margin: 0;
		font-family: var(--serif);
		font-size: var(--t-sm);
		line-height: 1.4;
		color: var(--ink-2);
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.bfoot {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.bfoot .wl {
		font-size: var(--t-xs);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-family: var(--mono);
		flex: none;
	}
	.wchip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		height: 19px;
		padding: 0 6px;
		border-radius: var(--r-sm);
		background: var(--sunken);
		border: 1px solid var(--line);
		font-size: var(--t-xs);
		color: var(--ink-2);
	}
	.wchip {
		font-family: var(--mono);
	}
	.wchip:hover {
		background: var(--hover);
	}

	.reply {
		display: flex;
		gap: 8px;
		align-items: flex-end;
		margin-top: 8px;
	}
	.reply .textarea {
		flex: 1;
	}

	.btn {
		height: 26px;
		padding: 0 10px;
		border-radius: var(--r-sm);
		font: 500 var(--t-sm)/1 var(--font);
		display: inline-flex;
		align-items: center;
		gap: 5px;
		border: 1px solid transparent;
	}
	.btn.sd {
		background: var(--surface);
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.btn.sd:hover {
		background: var(--hover);
	}
	.btn.gho {
		background: transparent;
		color: var(--ink-2);
		border-color: var(--line);
	}
	.btn.gho:hover {
		color: var(--ink);
		background: var(--hover);
	}
	.btn:disabled {
		opacity: 0.6;
		cursor: default;
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

	/* empty */
	.empty {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 9px;
		padding: 24px;
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

	@media (max-width: 720px) {
		.tbar {
			padding: 8px 14px;
		}
		.blist {
			padding: 10px 14px 20px;
		}
		.btop .t {
			white-space: normal;
			overflow: visible;
		}
	}
</style>
