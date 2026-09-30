<script>
	// A ticket's comments, beside the task: read them, add one. Kept live, so a
	// comment written elsewhere — another tab, an MCP client — appears here.
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import Markdown from '$components/Markdown.svelte';

	let { issue } = $props();

	let comments = $state([]);
	let draft = $state('');
	let sending = $state(false);
	let list = $state(null);

	async function load() {
		comments = (await api.comments(issue.id).catch(() => [])) || [];
		await tick();
		if (list) list.scrollTop = list.scrollHeight;
	}
	onMount(() => {
		load();
		return onLive((ev) => {
			const id = ev.issue?.id || ev.issueId;
			if (ev.type === 'comment.added' && id === issue.id) load();
		});
	});

	async function send() {
		const body = draft.trim();
		if (!body || sending) return;
		sending = true;
		try {
			await api.addComment(issue.id, body);
			draft = '';
			await load();
		} catch (e) {
			showToast('Comment failed: ' + e.message, 'error');
		} finally {
			sending = false;
		}
	}
	function keydown(e) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			send();
		}
	}
</script>

<div class="cm">
	<div class="head">Comments{#if comments.length}<span class="n">{comments.length}</span>{/if}</div>
	<div class="list" bind:this={list}>
		{#each comments as c (c.id)}
			<div class="c" class:ai={c.actor === 'ai'}>
				<div class="meta"><span class="who">{c.actor === 'ai' ? '✦ Clanker' : c.actor || 'someone'}</span><span class="when">{rel(c.createdAt)}</span></div>
				<div class="body"><Markdown source={c.bodyMd} /></div>
			</div>
		{:else}
			<div class="empty">No comments yet.</div>
		{/each}
	</div>
	<div class="add">
		<textarea bind:value={draft} onkeydown={keydown} placeholder="Leave a comment…" rows="3"></textarea>
		<div class="row">
			<span class="hint">⌘↵ to send</span>
			<button class="btn primary sm" onclick={send} disabled={!draft.trim() || sending}>Comment</button>
		</div>
	</div>
</div>

<style>
	.cm {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 12px 16px;
		font-size: 12px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--text-dim);
		border-bottom: 1px solid var(--border);
	}
	.n {
		font-weight: 500;
		color: var(--text-faint);
	}
	.list {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 12px 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.c {
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 10px 12px;
		background: var(--bg-elev);
	}
	.meta {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		font-size: 12px;
		margin-bottom: 4px;
	}
	.who {
		font-weight: 600;
		color: var(--text);
	}
	.c.ai .who {
		color: var(--text-dim);
	}
	.when {
		color: var(--text-faint);
	}
	.body {
		font-size: 13.5px;
		line-height: 1.55;
		overflow-wrap: anywhere;
	}
	.empty {
		font-size: 13px;
		color: var(--text-faint);
	}
	.add {
		border-top: 1px solid var(--border);
		padding: 10px 16px 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	textarea {
		width: 100%;
		resize: vertical;
		min-height: 64px;
		font: inherit;
		font-size: 13.5px;
	}
	.row {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.hint {
		font-size: 11.5px;
		color: var(--text-faint);
	}
</style>
