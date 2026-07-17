<script>
	import { goto } from '$app/navigation';
	import {
		Plus,
		CircleDot,
		SignalHigh,
		Box,
		Type,
		MessageSquare,
		FileText,
		Trash2
	} from '@lucide/svelte';

	let { items = [], showIssue = false } = $props();

	const ICON = {
		created: Plus,
		state_changed: CircleDot,
		priority_changed: SignalHigh,
		epic_changed: Box,
		title_changed: Type,
		commented: MessageSquare,
		artifact_written: FileText,
		deleted: Trash2
	};

	function verb(a) {
		switch (a.kind) {
			case 'created':
				return `created${a.to ? ' in ' + a.to : ''}`;
			case 'state_changed':
				return `${a.from} → ${a.to}`;
			case 'priority_changed':
				return `priority ${a.from} → ${a.to}`;
			case 'epic_changed':
				return `epic ${a.from} → ${a.to}`;
			case 'title_changed':
				return `renamed the issue`;
			case 'commented':
				return `commented: ${a.detail}`;
			case 'artifact_written':
				return `wrote artifact “${a.detail}”`;
			case 'deleted':
				return `deleted`;
			default:
				return a.kind;
		}
	}
	function rel(iso) {
		const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
		if (s < 60) return 'just now';
		if (s < 3600) return Math.floor(s / 60) + 'm ago';
		if (s < 86400) return Math.floor(s / 3600) + 'h ago';
		if (s < 604800) return Math.floor(s / 86400) + 'd ago';
		return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}
</script>

<div class="feed">
	{#each items as a (a.id)}
		{@const Icon = ICON[a.kind] || CircleDot}
		<div class="act">
			<span class="act-ic" class:art={a.kind === 'artifact_written'}><Icon size={13} strokeWidth={2} /></span>
			<div class="act-body">
				<div class="act-line">
					{#if showIssue && a.issueKey}
						<button class="act-key" onclick={() => goto('/issue/' + a.issueKey)}>{a.issueKey}</button>
					{/if}
					<span class="act-actor" class:ai={a.actor === 'ai'}>{a.actor === 'ai' ? '✦ AI' : 'You'}</span>
					<span class="act-verb">{verb(a)}</span>
				</div>
				{#if showIssue && a.issueTitle}<span class="act-sub">{a.issueTitle}</span>{/if}
			</div>
			<span class="act-time">{rel(a.createdAt)}</span>
		</div>
	{:else}
		<div class="act-empty faint">No activity yet.</div>
	{/each}
</div>

<style>
	.feed {
		display: flex;
		flex-direction: column;
	}
	.act {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		padding: 8px 0;
		position: relative;
	}
	.act:not(:last-child)::before {
		content: '';
		position: absolute;
		left: 10px;
		top: 27px;
		bottom: -4px;
		width: 1px;
		background: var(--border);
	}
	.act-ic {
		width: 21px;
		height: 21px;
		border-radius: 50%;
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		color: var(--text-dim);
		display: grid;
		place-items: center;
		flex: none;
		z-index: 1;
	}
	.act-ic.art {
		color: var(--accent2);
		border-color: color-mix(in srgb, var(--accent2) 40%, var(--border));
	}
	.act-body {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
		padding-top: 2px;
	}
	.act-line {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		font-size: 13px;
	}
	.act-key {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--text-faint);
		background: none;
		border: none;
		padding: 0;
	}
	.act-key:hover {
		color: var(--text);
	}
	.act-actor {
		font-weight: 500;
		color: var(--text);
	}
	.act-actor.ai {
		color: var(--accent2);
	}
	.act-verb {
		color: var(--text-dim);
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.act-sub {
		font-size: 12px;
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.act-time {
		font-size: 11.5px;
		color: var(--text-faint);
		flex: none;
		padding-top: 4px;
	}
	.act-empty {
		padding: 12px 0;
		font-size: 13px;
	}
</style>
