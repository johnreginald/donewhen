<script>
	// Blocked — every ticket sitting in the Blocked state, workspace-wide (not
	// scoped to the current epic/label filter — a blocker worth seeing might
	// live in a different epic). Each card: the latest comment as the reason,
	// the blockers it waits on with their current state, how long it's been
	// stuck, and the one action that matters: Unblock → In Progress.
	// This is the UI half of PP-207.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { states, blockLinks, loadBlockLinks } from '$lib/store.js';
	import { openIssue, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import StateIcon from '$components/StateIcon.svelte';

	const stOf = (id) => $states.find((s) => s.id === id);

	let loading = $state(true);
	let allIssues = $state([]);
	let comments = $state({}); // issueId -> latest comment bodyMd (or '')
	let query = $state('');
	let busy = $state(new Set()); // issue ids mid-unblock

	async function load() {
		loading = true;
		try {
			const [list] = await Promise.all([api.issues(), loadBlockLinks()]);
			allIssues = list || [];
			const blocked = allIssues.filter((i) => stOf(i.stateId)?.name === 'Blocked');
			const pairs = await Promise.all(
				blocked.map(async (i) => {
					const list = await api.comments(i.key).catch(() => []);
					const last = (list || [])[list?.length - 1];
					return [i.id, last?.bodyMd || ''];
				})
			);
			comments = Object.fromEntries(pairs);
		} finally {
			loading = false;
		}
	}
	onMount(load);

	// A plain-text excerpt of a markdown comment: strip the common markdown
	// marks rather than rendering them, then clip to one line's worth.
	function excerpt(md) {
		if (!md) return 'No comment yet explaining why.';
		const text = md
			.replace(/```[\s\S]*?```/g, ' ')
			.replace(/[#>*_`~]/g, '')
			.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
			.replace(/\s+/g, ' ')
			.trim();
		return text.length > 160 ? text.slice(0, 157) + '…' : text || 'No comment yet explaining why.';
	}

	const issueById = $derived(new Map(allIssues.map((i) => [i.id, i])));
	const blockedIssues = $derived(
		allIssues
			.filter((i) => stOf(i.stateId)?.name === 'Blocked')
			.filter((i) => !query.trim() || i.title.toLowerCase().includes(query.trim().toLowerCase()) || i.key.toLowerCase().includes(query.trim().toLowerCase()))
			.map((i) => ({
				...i,
				reason: excerpt(comments[i.id]),
				blockers: $blockLinks
					.filter((l) => l.issueId === i.id)
					.map((l) => ({ ...issueById.get(l.blockerId), done: l.done }))
					.filter((b) => b.id)
			}))
			.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt))
	);

	async function unblock(i) {
		busy = new Set([...busy, i.id]);
		try {
			await api.updateIssue(i.id, { stateName: 'In Progress' });
			showToast(`${i.key} moved to In Progress`, 'info');
			allIssues = allIssues.map((x) => (x.id === i.id ? { ...x, stateId: $states.find((s) => s.name === 'In Progress')?.id ?? x.stateId } : x));
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			const n = new Set(busy);
			n.delete(i.id);
			busy = n;
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
			{#if !loading}{blockedIssues.length} ticket{blockedIssues.length === 1 ? '' : 's'} waiting on another ticket{/if}
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
	{:else if !blockedIssues.length}
		<div class="empty">
			<div class="ic">✓</div>
			<p class="etitle">Nothing is blocked</p>
			<p class="esub">Every ticket in this workspace is free to move.</p>
		</div>
	{:else}
		<div class="blist">
			{#each blockedIssues as i (i.id)}
				{@const st = stOf(i.stateId)}
				<div class="bcard">
					<div class="btop">
						<StateIcon category={st?.category} color={st?.color} size={13} />
						<span class="k">{i.key}</span>
						<span class="t">{i.title}</span>
						<span class="age">blocked {rel(i.updatedAt)}</span>
					</div>
					<div class="breason"><span class="av">C</span><p>{i.reason}</p></div>
					<div class="bfoot">
						{#if i.blockers.length}
							<span class="wl">Waiting on</span>
							{#each i.blockers as b (b.id)}
								{@const bst = stOf(b.stateId)}
								<span class="wchip" class:done={b.done}>
									<StateIcon category={bst?.category} color={bst?.color} size={11} />{b.key} · {bst?.name || ''}
								</span>
							{/each}
						{:else}
							<span class="wl">No linked blockers</span>
						{/if}
						<span class="spacer"></span>
						<button class="btn gho" disabled={busy.has(i.id)} onclick={() => unblock(i)}>
							{busy.has(i.id) ? 'Unblocking…' : 'Unblock'}
						</button>
						<button class="btn sd" onclick={() => openIssue(i.key)}>Open ↗</button>
					</div>
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
		border-radius: var(--r-sm);
		padding: 5px 10px;
		color: var(--ink-3);
		width: min(220px, 100%);
	}
	.search input {
		background: none;
		border: none;
		outline: none;
		font-size: 12.5px;
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
		font-size: 12.5px;
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
		font-size: 11.5px;
		flex: none;
	}
	.btop .t {
		font-weight: 500;
		font-size: 13px;
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.btop .age {
		font-family: var(--mono);
		font-size: 10px;
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
		font: 600 9px var(--font);
		flex: none;
		margin-top: 1px;
	}
	.breason p {
		margin: 0;
		font-family: var(--serif);
		font-size: 12.5px;
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
		font-size: 10px;
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
		font-size: 10.5px;
		color: var(--ink-2);
	}
	.wchip.done {
		opacity: 0.65;
	}

	.btn {
		height: 26px;
		padding: 0 10px;
		border-radius: var(--r-sm);
		font: 500 12px/1 var(--font);
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
		border-radius: 4px;
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
		font-size: 14px;
	}
	.etitle {
		margin: 0;
		font-size: 13.5px;
		font-weight: 600;
		color: var(--ink);
	}
	.esub {
		margin: 0;
		font-size: 12px;
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
