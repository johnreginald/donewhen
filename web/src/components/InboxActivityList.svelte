<script>
	// The Inbox's own "Recent AI activity" rows. Not ActivityFeed.svelte (owned
	// by the issue-page agent) — this one needs the "new since last visit"
	// split + divider the design calls for, which ActivityFeed doesn't do.
	import { rel } from '$lib/format.js';
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

	let { items = [], seenAt = null, aiName = 'Clanker' } = $props();

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
				return `wrote a document “${a.detail}”`;
			case 'epic_archived':
				return a.detail ? `archived epic “${a.detail}”` : 'archived an epic';
			case 'epic_unarchived':
				return a.detail ? `unarchived epic “${a.detail}”` : 'unarchived an epic';
			case 'deleted':
				return `deleted`;
			default:
				return a.kind.replace(/_/g, ' ');
		}
	}

	// Never-visited-before (seenAt is null) reads as "everything is new" —
	// same rule the old inbox used for the sidebar badge.
	const newItems = $derived(seenAt ? items.filter((a) => a.createdAt > seenAt) : items);
	const oldItems = $derived(seenAt ? items.filter((a) => !(a.createdAt > seenAt)) : []);
</script>

<div class="act-list">
	{#each newItems as a (a.id)}
		{@const Icon = ICON[a.kind] || CircleDot}
		<div class="act-row new">
			<span class="act-ic"><Icon size={12} strokeWidth={2} /></span>
			<div class="act-body">
				<div class="act-line">
					{#if a.issueKey}
						<button class="act-key" onclick={() => goto('/issue/' + a.issueKey)}>{a.issueKey}</button>
					{/if}
					<span class="act-actor">✦ {aiName}</span>
					<span class="act-verb">{verb(a)}</span>
				</div>
				{#if a.issueTitle}<span class="act-sub">{a.issueTitle}</span>{/if}
			</div>
			<span class="act-time">{rel(a.createdAt)}</span>
		</div>
	{/each}
	{#if newItems.length && oldItems.length}
		<div class="newdiv">New since your last visit</div>
	{/if}
	{#each oldItems as a (a.id)}
		{@const Icon = ICON[a.kind] || CircleDot}
		<div class="act-row">
			<span class="act-ic"><Icon size={12} strokeWidth={2} /></span>
			<div class="act-body">
				<div class="act-line">
					{#if a.issueKey}
						<button class="act-key" onclick={() => goto('/issue/' + a.issueKey)}>{a.issueKey}</button>
					{/if}
					<span class="act-actor">✦ {aiName}</span>
					<span class="act-verb">{verb(a)}</span>
				</div>
				{#if a.issueTitle}<span class="act-sub">{a.issueTitle}</span>{/if}
			</div>
			<span class="act-time">{rel(a.createdAt)}</span>
		</div>
	{/each}
	{#if !items.length}
		<div class="act-empty faint">No {aiName} activity yet.</div>
	{/if}
</div>

<style>
	.act-list {
		display: flex;
		flex-direction: column;
	}
	.act-row {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		padding: 6px 0;
		position: relative;
	}
	.act-row.new {
		background: var(--accent-soft);
		margin: 0 -10px;
		padding-left: 10px;
		padding-right: 10px;
		border-radius: var(--r-sm);
	}
	.act-row:not(:last-child)::before {
		content: '';
		position: absolute;
		left: 10px;
		top: 25px;
		bottom: -3px;
		width: 1px;
		background: var(--line);
	}
	.act-ic {
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--sunken);
		border: 1px solid var(--line);
		color: var(--ink-2);
		display: flex;
		align-items: center;
		justify-content: center;
		flex: none;
		z-index: 1;
	}
	.act-body {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
		padding-top: 1px;
	}
	.act-line {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		font-size: var(--t-sm);
	}
	.act-key {
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--ink-3);
		background: none;
		border: none;
		padding: 0;
	}
	.act-key:hover {
		color: var(--ink);
	}
	.act-actor {
		font-weight: 600;
		color: var(--accent);
	}
	.act-verb {
		color: var(--ink-2);
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.act-sub {
		font-size: var(--t-xs);
		color: var(--ink-3);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.act-time {
		font-size: var(--t-xs);
		color: var(--ink-3);
		flex: none;
		padding-top: 2px;
	}
	.newdiv {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 6px 0 2px;
		font: 600 var(--t-xs) var(--mono);
		color: var(--accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.newdiv::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--accent-soft);
	}
	.act-empty {
		padding: 12px 0;
		font-size: var(--t-sm);
	}
</style>
