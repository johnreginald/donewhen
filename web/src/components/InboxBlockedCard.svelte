<script>
	// One "Blocked — needs you" row: the issue, why it's stuck (the latest
	// comment's excerpt — real blocked-reason data is PP-207, out of scope
	// here), and a quick Open / Reply.
	import { rel } from '$lib/format.js';

	let {
		item,
		epicName = '',
		blockedByKey = '',
		aiName = 'Clanker',
		replying = false,
		replyText = '',
		busy = false,
		onOpen,
		onToggleReply,
		onReplyInput,
		onReplySend
	} = $props();
</script>

<div class="bl-card">
	<div class="bl-top">
		<span class="gl ring blocked"><span class="bar"></span></span>
		<span class="k">{item.key}</span>
		<button class="ttl-btn" onclick={() => onOpen?.(item)}>{item.title}</button>
		<span class="time">updated {rel(item.updatedAt)}</span>
	</div>
	<div class="bl-meta">
		{#if epicName}<span>{epicName}</span>{/if}
		{#if blockedByKey}<span class="dot2">·</span><span class="blk">⛌ {blockedByKey}</span>{/if}
	</div>
	<p class="bl-reason">
		<b>✦ {aiName} —</b>
		{item.reason || "No update yet — open the issue to see what's stuck."}
	</p>
	<div class="bl-actions">
		<button class="btn" onclick={() => onOpen?.(item)}>Open</button>
		<button class="btn ghost" onclick={() => onToggleReply?.(item)}>Reply</button>
	</div>
	{#if replying}
		<div class="bl-reply">
			<textarea
				class="textarea"
				placeholder="Reply to {aiName}…"
				value={replyText}
				oninput={(e) => onReplyInput?.(e.target.value)}
			></textarea>
			<div class="brow">
				<button class="btn ghost" onclick={() => onToggleReply?.(item)}>Cancel</button>
				<button class="btn" disabled={busy || !replyText.trim()} onclick={() => onReplySend?.(item)}>
					{busy ? 'Sending…' : 'Send reply'}
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.bl-card {
		background: var(--surface);
		border: 1px solid var(--line);
		border-left: 2px solid var(--st-blocked);
		border-radius: var(--r);
		padding: 9px 13px;
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	.bl-top {
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
	.gl.blocked {
		color: var(--st-blocked);
	}
	.gl .bar {
		width: 5.5px;
		height: 1.5px;
		background: currentColor;
	}
	.k {
		font-family: var(--mono);
		font-size: var(--t-sm);
		color: var(--ink-3);
		flex: none;
	}
	.ttl-btn {
		font-family: var(--font);
		font-weight: 500;
		font-size: var(--t-base);
		color: var(--ink);
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
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
	.bl-meta {
		font-size: var(--t-xs);
		color: var(--ink-2);
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.dot2 {
		opacity: 0.5;
	}
	.blk {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		height: 20px;
		padding: 0 7px;
		border-radius: var(--r-sm);
		background: var(--danger-soft);
		color: var(--danger);
		font: 500 var(--t-xs) var(--mono);
	}
	.bl-reason {
		font-family: var(--serif);
		font-size: var(--t-sm);
		line-height: 1.5;
		color: var(--ink-2);
		margin: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.bl-reason b {
		font-family: var(--font);
		font-weight: 600;
		color: var(--ink);
		font-size: var(--t-xs);
	}
	.bl-actions {
		display: flex;
		gap: 6px;
		margin-top: 2px;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.bl-reply {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding-top: 6px;
		border-top: 1px dashed var(--line);
	}
	.bl-reply textarea {
		min-height: 54px;
	}
	.brow {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}
</style>
