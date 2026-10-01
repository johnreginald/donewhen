<script>
	// The Log's day-grouped timeline: a vertical feed of activity entries, with
	// a divider marking what's new since the viewer last looked. Built separate
	// from ActivityFeed (the issue page's flat, undated feed) because the Log
	// needs day grouping, the new-since divider, and icon/verb coverage for
	// kinds ActivityFeed doesn't render (commit_linked, criterion_checked,
	// label_changed, assignee_changed, description_changed — arriving with
	// PP-186). A kind this doesn't recognize still renders, generically.
	import { aiName } from '$lib/store.js';
	import { goto } from '$app/navigation';
	import { rel } from '$lib/format.js';

	let { items = [], newSince = null, showIssue = true } = $props();

	const ICON = {
		created: '✚',
		state_changed: '⟶',
		priority_changed: '▲',
		epic_changed: '▢',
		title_changed: '✎',
		commented: '✎',
		artifact_written: '▤',
		deleted: '✕',
		commit_linked: '⎇',
		criterion_checked: '✓',
		label_changed: '◧',
		assignee_changed: '◐',
		description_changed: '≡'
	};
	const ACCENT = new Set(['artifact_written', 'commit_linked', 'criterion_checked']);
	const DANGER = new Set(['deleted']);

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
				return 'renamed the issue';
			case 'commented':
				return `commented: “${a.detail}”`;
			case 'artifact_written':
				return `wrote artifact “${a.detail}”`;
			case 'deleted':
				return 'deleted';
			case 'criterion_checked':
				return a.detail ? `done-when ${a.detail}` : a.from && a.to ? `done-when ${a.from} → ${a.to}` : 'done-when updated';
			case 'description_changed':
				return 'description edited';
			case 'assignee_changed':
				return a.to ? `assignee set to ${a.to}` : 'assignee changed';
			default:
				return a.kind.replace(/_/g, ' ');
		}
	}

	// commit_linked and label_changed splice a mono fragment into the line —
	// everything else is one plain run of text from verb().
	function monoLead(a) {
		if (a.kind === 'commit_linked') return 'linked commit';
		if (a.kind === 'label_changed') return a.detail ? (a.action === 'removed' ? 'label removed' : 'label added') : null;
		return null;
	}

	function dayKey(iso) {
		const d = new Date(iso);
		return d.getFullYear() + '-' + d.getMonth() + '-' + d.getDate();
	}
	function dayLabel(iso) {
		const d = new Date(iso);
		const now = new Date();
		const startOf = (x) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
		const diff = Math.round((startOf(now) - startOf(d)) / 86400000);
		const md = d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
		if (diff === 0) return `Today · ${md}`;
		if (diff === 1) return `Yesterday · ${md}`;
		return md;
	}

	// rows interleaves day headers, the new-since divider, and entries — in
	// one pass over the (already newest-first) list.
	const rows = $derived.by(() => {
		const out = [];
		let lastDay = null;
		const haveNewer = newSince ? items.some((i) => new Date(i.createdAt) > new Date(newSince)) : false;
		let dividerDone = !haveNewer;
		for (const it of items) {
			const dk = dayKey(it.createdAt);
			if (dk !== lastDay) {
				out.push({ row: 'day', id: 'day-' + dk, label: dayLabel(it.createdAt) });
				lastDay = dk;
			}
			if (!dividerDone && new Date(it.createdAt) <= new Date(newSince)) {
				out.push({ row: 'divider', id: 'divider' });
				dividerDone = true;
			}
			out.push({ row: 'entry', id: it.id, item: it });
		}
		return out;
	});
</script>

<div class="timeline">
	{#each rows as r (r.id)}
		{#if r.row === 'day'}
			<div class="day">{r.label}</div>
		{:else if r.row === 'divider'}
			<div class="newdiv"><span class="ln"></span><span class="lab">New since your last visit</span><span class="ln"></span></div>
		{:else}
			{@const a = r.item}
			<div class="entry">
				<span class="eic" class:acc={ACCENT.has(a.kind)} class:dan={DANGER.has(a.kind)}>{ICON[a.kind] || '•'}</span>
				<div class="eline">
					<div class="erow">
						{#if showIssue && a.issueKey}
							<button class="ekey" onclick={() => goto('/issue/' + a.issueKey)}>{a.issueKey}</button>
						{/if}
						<span class="eactor" class:ai={a.actor === 'ai'}>{a.actor === 'ai' ? `✦ ${$aiName}` : 'You'}</span>
						{#if monoLead(a)}
							<span class="everb">{monoLead(a)}</span><span class="everb mono">{a.detail}</span>
						{:else}
							<span class="everb">{verb(a)}</span>
						{/if}
					</div>
				</div>
				<span class="etime">{rel(a.createdAt)}</span>
			</div>
		{/if}
	{:else}
		<div class="empty faint">No activity recorded yet.</div>
	{/each}
</div>

<style>
	.timeline {
		display: flex;
		flex-direction: column;
	}
	.day {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 18px 0 8px;
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.day::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--line);
	}
	.day:first-child {
		padding-top: 2px;
	}
	.newdiv {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 2px 0 10px;
	}
	.newdiv .ln {
		flex: 1;
		height: 1px;
		background: var(--accent);
	}
	.newdiv .lab {
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--accent);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		white-space: nowrap;
	}
	.entry {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		padding: 7px 0;
		position: relative;
	}
	.entry:not(:last-child)::before {
		content: '';
		position: absolute;
		left: 11px;
		top: 29px;
		bottom: -7px;
		width: 1px;
		background: var(--line);
	}
	.eic {
		width: 23px;
		height: 23px;
		border-radius: 50%;
		background: var(--surface);
		border: 1px solid var(--line);
		display: flex;
		align-items: center;
		justify-content: center;
		flex: none;
		font-size: var(--t-xs);
		color: var(--ink-2);
		box-sizing: border-box;
		z-index: 1;
	}
	.eic.acc {
		color: var(--accent);
		border-color: color-mix(in srgb, var(--accent) 40%, var(--line));
	}
	.eic.dan {
		color: var(--danger);
		border-color: color-mix(in srgb, var(--danger) 40%, var(--line));
	}
	.eline {
		flex: 1;
		min-width: 0;
		padding-top: 2px;
	}
	.erow {
		display: flex;
		align-items: baseline;
		gap: 7px;
		flex-wrap: wrap;
		font-size: var(--t-base);
	}
	.ekey {
		font-family: var(--mono);
		font-size: var(--t-sm);
		color: var(--ink-3);
		background: none;
		border: none;
		padding: 0;
	}
	.ekey:hover {
		color: var(--ink);
	}
	.eactor {
		font-weight: 500;
		color: var(--ink);
	}
	.eactor.ai {
		color: var(--accent);
	}
	.everb {
		color: var(--ink-2);
	}
	.everb.mono {
		font-family: var(--mono);
		font-size: var(--t-sm);
		color: var(--ink);
	}
	.etime {
		font-size: var(--t-xs);
		color: var(--ink-3);
		flex: none;
		padding-top: 4px;
	}
	.empty {
		padding: 30px 0;
		font-size: var(--t-sm);
	}
</style>
