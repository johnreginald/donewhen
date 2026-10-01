<script>
	import InlineCode from './InlineCode.svelte';
	// One "Needs review" card — an issue the AI moved to In Review. Approve
	// (→ Done) is gated on every done-when criterion being ticked; Bounce
	// (→ In Progress) always works, with an optional reason that becomes a
	// comment only when non-empty.
	import { rel } from '$lib/format.js';
	import { safeHref } from '$lib/url.js';

	let {
		item,
		epicName = '',
		aiName = 'Clanker',
		crit = null, // { loading, items: [{id, body, done}] } | null
		expanded = false,
		selected = false,
		busy = false,
		bounceOpen = false,
		bounceText = '',
		onOpen,
		onToggleExpand,
		onApprove,
		onToggleBounce,
		onBounceInput,
		onBounceConfirm,
		onBounceCancel
	} = $props();

	const total = $derived(crit?.items?.length ?? 0);
	const done = $derived(crit?.items?.filter((c) => c.done).length ?? 0);
	const openCount = $derived(total - done);
	const doneWidth = $derived(total ? Math.round((done / total) * 100) + '%' : '0%');
	const checking = $derived(!!crit?.loading);
	const approveDisabled = $derived(busy || checking || openCount > 0);
	const approveNote = $derived(
		checking
			? 'Checking done-when…'
			: openCount > 0
				? `${openCount} criteria open — approve unlocks at ${total}/${total}`
				: ''
	);

	let textareaEl = $state(null);
	$effect(() => {
		if (bounceOpen && textareaEl) textareaEl.focus();
	});

	// A short, markdown-stripped preview of the ticket body for the expanded
	// card — the full spec lives on the issue page.
	function excerpt(md, max = 260) {
		if (!md) return '';
		const plain = md
			.replace(/```[\s\S]*?```/g, ' ')
			.replace(/!\[[^\]]*]\([^)]*\)/g, ' ')
			.replace(/\[([^\]]*)]\([^)]*\)/g, '$1')
			.replace(/^#{1,6}\s+/gm, '')
			.replace(/[*_`>#-]/g, '')
			.replace(/\s+/g, ' ')
			.trim();
		return plain.length > max ? plain.slice(0, max).trim() + '…' : plain;
	}
</script>

<div class="nr-card" class:selected>
	<div class="nr-top">
		<span class="gl ring review"><span class="tq"></span></span>
		<span class="k">{item.key}</span>
		<button class="ttl-btn" onclick={() => onOpen?.(item)}>{item.title}</button>
		<span class="time">moved {rel(item.enteredReviewAt || item.updatedAt)}</span>
	</div>
	<div class="nr-meta">
		{#if epicName}<span>{epicName}</span><span class="dot2">·</span>{/if}
		<span class="nr-dw"
			><span class="bar"><i style="width:{doneWidth}"></i></span>{done}/{total} done-when</span
		>
		{#if item.commitCount}<span class="dot2">·</span><span>{item.commitCount} commits</span>{/if}
		{#if safeHref(item.prUrl)}
			<span class="dot2">·</span>
			<a class="pr-link" href={safeHref(item.prUrl)} target="_blank" rel="noopener noreferrer">PR ↗</a>
		{/if}
		<span class="dot2">·</span>
		<span>by <span class="actor-ai">✦ {aiName}</span></span>
	</div>

	{#if expanded}
		<div class="nr-exp">
			{#if item.descriptionMd}<p class="prose">{excerpt(item.descriptionMd)}</p>{/if}
			{#if crit?.loading}
				<div class="ck-loading faint">Loading done-when…</div>
			{:else if crit?.items?.length}
				{#each crit.items as c (c.id)}
					<div class="ck" class:done={c.done}>
						<i class="box" class:on={c.done}>{c.done ? '✓' : ''}</i>
						<span><InlineCode text={c.body} /></span>
					</div>
				{/each}
			{:else}
				<div class="ck-loading faint">No done-when criteria set.</div>
			{/if}
		</div>
	{/if}

	{#if bounceOpen}
		<div class="nr-bounce">
			<label for="bounce-{item.id}">Reason (optional)</label>
			<textarea
				class="textarea"
				id="bounce-{item.id}"
				bind:this={textareaEl}
				placeholder="What needs another pass?"
				value={bounceText}
				oninput={(e) => onBounceInput?.(e.target.value)}
			></textarea>
			<div class="help">Leave empty to bounce without a comment.</div>
			<div class="brow">
				<button class="btn gho" onclick={() => onBounceCancel?.(item)}>Cancel</button>
				<button class="btn sd" disabled={busy} onclick={() => onBounceConfirm?.(item)}>
					{busy ? 'Bouncing…' : 'Confirm bounce'}
				</button>
			</div>
		</div>
	{:else}
		<div class="nr-actions">
			<button class="btn sd" disabled={approveDisabled} title={approveNote} onclick={() => onApprove?.(item)}>
				{busy ? 'Approving…' : 'Approve'}
			</button>
			<button class="btn gho" disabled={busy} onclick={() => onToggleBounce?.(item)}>Bounce back</button>
			{#if approveNote}<span class="nr-note">{approveNote}</span>{/if}
			<span class="sp"></span>
			<button class="exp-btn" onclick={() => onToggleExpand?.(item)} aria-label={expanded ? 'Collapse' : 'Expand'}>
				<span class="caret" class:open={expanded}></span>
			</button>
		</div>
	{/if}
</div>

<style>
	.nr-card {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 9px 13px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.nr-card.selected {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--focus);
	}
	.nr-top {
		display: flex;
		align-items: baseline;
		gap: 8px;
		min-width: 0;
	}
	.gl {
		width: 13px;
		height: 13px;
		border-radius: 50%;
		box-sizing: border-box;
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
		top: 1px;
	}
	.gl.ring {
		border: 1.5px solid currentColor;
		background: transparent;
	}
	.gl.review {
		color: var(--st-review);
	}
	.gl .tq {
		width: 6.5px;
		height: 6.5px;
		border-radius: 50%;
		background: conic-gradient(currentColor 0 75%, transparent 75%);
	}
	.k {
		font-family: var(--mono);
		font-size: var(--t-sm);
		color: var(--ink-3);
		flex: none;
	}
	.ttl-btn {
		font-family: var(--serif);
		font-size: var(--t-lg);
		color: var(--ink);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		flex: 1;
		min-width: 0;
		background: none;
		border: none;
		padding: 0;
		text-align: left;
		cursor: pointer;
	}
	.ttl-btn:hover {
		text-decoration: underline;
	}
	.time {
		font-size: var(--t-xs);
		color: var(--ink-3);
		flex: none;
	}
	.nr-meta {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		font-size: var(--t-xs);
		color: var(--ink-2);
	}
	.dot2 {
		opacity: 0.5;
	}
	.nr-dw {
		display: inline-flex;
		align-items: center;
		gap: 5px;
	}
	.nr-dw .bar {
		width: 32px;
		height: 4px;
		border-radius: var(--r-sm);
		background: var(--sunken);
		overflow: hidden;
		display: inline-block;
	}
	.nr-dw .bar i {
		display: block;
		height: 100%;
		background: var(--st-done);
	}
	.actor-ai {
		color: var(--accent);
		font-weight: 600;
	}
	.pr-link {
		color: var(--accent);
	}
	.pr-link:hover {
		text-decoration: underline;
	}
	.nr-exp {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding-top: 5px;
		border-top: 1px dashed var(--line);
	}
	.prose {
		font-family: var(--serif);
		font-size: var(--t-base);
		line-height: 1.6;
		color: var(--ink-2);
		margin: 0;
	}
	.ck {
		display: flex;
		align-items: flex-start;
		gap: 9px;
		font-size: var(--t-sm);
	}
	.ck.done span {
		color: var(--ink-2);
	}
	.box {
		width: 15px;
		height: 15px;
		border-radius: var(--r-sm);
		border: 1.5px solid var(--line-strong);
		flex: none;
		margin-top: 2px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: var(--t-xs);
		font-weight: 700;
		line-height: 1;
		color: var(--accent-ink);
		box-sizing: border-box;
		font-style: normal;
	}
	.box.on {
		background: var(--accent);
		border-color: var(--accent);
	}
	.ck-loading {
		font-size: var(--t-sm);
	}
	.nr-bounce {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding-top: 6px;
		border-top: 1px dashed var(--line);
	}
	.nr-bounce label {
		font-size: var(--t-xs);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-weight: 600;
	}
	.nr-bounce textarea {
		min-height: 54px;
	}
	.help {
		font-size: var(--t-xs);
		color: var(--ink-3);
		margin-top: -2px;
	}
	.brow {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}
	.nr-actions {
		display: flex;
		align-items: center;
		gap: 6px;
		margin-top: 1px;
		flex-wrap: wrap;
	}
	.nr-actions .sp {
		flex: 1;
	}
	.nr-note {
		font-size: var(--t-xs);
		color: var(--ink-3);
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
		transition: background var(--dur) var(--ease), opacity var(--dur) var(--ease);
	}
	.btn.sd {
		background: var(--surface);
		border-color: var(--line-strong);
		color: var(--ink);
	}
	.btn.sd:hover:not(:disabled) {
		background: var(--hover);
	}
	.btn.gho {
		background: transparent;
		color: var(--ink-2);
		border-color: transparent;
	}
	.btn.gho:hover:not(:disabled) {
		background: var(--hover);
		color: var(--ink);
	}
	.btn:disabled {
		opacity: 0.42;
		cursor: not-allowed;
	}
	.caret {
		width: 0;
		height: 0;
		border-left: 4px solid transparent;
		border-right: 4px solid transparent;
		border-top: 5px solid var(--ink-3);
		display: inline-block;
		transition: transform var(--dur) var(--ease);
	}
	.caret.open {
		transform: rotate(180deg);
	}
	.exp-btn {
		width: 26px;
		height: 26px;
		border-radius: var(--r-sm);
		border: 1px solid transparent;
		background: none;
		display: flex;
		align-items: center;
		justify-content: center;
		flex: none;
	}
	.exp-btn:hover {
		background: var(--hover);
	}
	@media (max-width: 720px) {
		.nr-actions {
			gap: 8px;
		}
		.btn {
			flex: 1 1 auto;
			justify-content: center;
		}
		.nr-actions .sp {
			display: none;
		}
		.exp-btn {
			flex: none;
		}
	}
</style>
