<script>
	// A right-side panel that shows one issue next to whatever page mounts it:
	// header, status/priority, epic, labels, done-when progress, description and
	// the latest activity. It loads and saves its own data, so any page can use
	// it. Full-screen sheet on a phone. Esc, Close and the backdrop call onclose.
	import { onMount, tick } from 'svelte';
	import { X } from '@lucide/svelte';
	import { api } from '$lib/api.js';
	import { aiName, projects, states } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel, activityVerb } from '$lib/format.js';
	import StateIcon from './StateIcon.svelte';
	import StatusMenu from './StatusMenu.svelte';
	import PriorityMenu from './PriorityMenu.svelte';
	import LabelPill from './LabelPill.svelte';
	import Markdown from './Markdown.svelte';

	let { issueKey, onclose } = $props();

	let issue = $state(null);
	let criteria = $state([]);
	let events = $state([]);
	let error = $state(null);
	let panel = $state(null);

	const st = $derived(issue ? $states.find((s) => s.id === issue.stateId) : null);
	const epic = $derived(issue?.projectId ? $projects.find((p) => p.id === issue.projectId) : null);
	const doneCount = $derived(criteria.filter((c) => c.done).length);
	const recent = $derived(
		events
			.filter((e) => e.kind !== 'commented')
			.slice(-5)
			.reverse()
	);

	let seq = 0;
	async function load(key) {
		const my = ++seq;
		error = null;
		try {
			const i = await api.issue(key);
			if (my !== seq) return;
			issue = i;
			const [c, a] = await Promise.all([api.criteria(i.id).catch(() => null), api.issueActivity(i.id).catch(() => null)]);
			if (my !== seq) return;
			criteria = c || [];
			events = a || [];
		} catch (e) {
			if (my !== seq) return;
			error = e?.status === 404 ? `${key} was not found.` : e?.message || 'Could not load this issue.';
		}
	}
	async function refresh() {
		if (!issue) return;
		const my = seq;
		const [i, c, a] = await Promise.all([api.issue(issue.id).catch(() => null), api.criteria(issue.id).catch(() => null), api.issueActivity(issue.id).catch(() => null)]);
		if (my !== seq) return;
		if (i) issue = i;
		if (c) criteria = c;
		if (a) events = a;
	}
	async function patch(body) {
		try {
			issue = await api.updateIssue(issue.id, body);
			api.issueActivity(issue.id).then((a) => a && (events = a)).catch(() => {});
		} catch (e) {
			showToast('Update failed: ' + (e?.message || 'unknown error'), 'error');
		}
	}

	$effect(() => {
		load(issueKey);
	});

	// ---- focus: trap inside the dialog, give it back on close ----
	const FOCUSABLE = 'a[href], button:not([disabled]), input, textarea, select, [tabindex]:not([tabindex="-1"])';
	function onkey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			// A status/priority menu is open: Esc closes only that menu.
			const bd = panel?.querySelector('.dd-bd');
			if (bd) {
				bd.click();
				return;
			}
			onclose?.();
			return;
		}
		if (e.key !== 'Tab' || !panel) return;
		const items = [...panel.querySelectorAll(FOCUSABLE)].filter((el) => el.offsetParent !== null);
		if (!items.length) return;
		const first = items[0];
		const last = items[items.length - 1];
		if (!panel.contains(document.activeElement)) {
			e.preventDefault();
			first.focus();
		} else if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	onMount(() => {
		const opener = document.activeElement;
		tick().then(() => panel?.focus());
		const off = onLive((ev) => {
			const id = ev.issue?.id || ev.issueId;
			if (!issue || id !== issue.id) return;
			if (ev.type === 'issue.deleted') onclose?.();
			else refresh();
		});
		window.addEventListener('keydown', onkey, true);
		return () => {
			off();
			window.removeEventListener('keydown', onkey, true);
			if (opener instanceof HTMLElement && document.contains(opener)) opener.focus({ preventScroll: true });
		};
	});
</script>

<div class="peek-bd" role="presentation" onclick={() => onclose?.()}></div>
<div class="peek" role="dialog" aria-modal="true" aria-label={`Issue ${issueKey}`} tabindex="-1" bind:this={panel}>
	<div class="ph">
		<span class="pk mono">{issue?.key ?? issueKey}</span>
		{#if st}<StateIcon category={st.category} color={st.color} name={st.name} />{/if}
		<span class="sp"></span>
		<a class="full" href={`/issue/${issue?.key ?? issueKey}`}>Open full page ↗</a>
		<button class="close" onclick={() => onclose?.()} aria-label="Close"><X size={16} strokeWidth={2} /></button>
	</div>

	<div class="pbody">
		{#if issue}
			<h2 class="ptitle">{issue.title}</h2>
			<div class="props">
				<StatusMenu value={issue.stateId} onchange={(v) => patch({ stateId: v })} />
				<PriorityMenu value={issue.priority} onchange={(v) => patch({ priority: v })} />
			</div>
			<dl class="facts">
				<dt>Epic</dt>
				<dd>{epic ? epic.name : 'No epic'}</dd>
				<dt>Labels</dt>
				<dd class="lbls">{#each issue.labels || [] as l (l.id)}<LabelPill label={l} />{:else}<span class="faint">None</span>{/each}</dd>
				<dt>Done when</dt>
				<dd class="mono">{criteria.length ? `${doneCount}/${criteria.length}` : 'No criteria'}</dd>
			</dl>

			<div class="sh">Description</div>
			{#if issue.descriptionMd?.trim()}
				<div class="desc"><Markdown source={issue.descriptionMd} /></div>
			{:else}
				<p class="faint small">No description.</p>
			{/if}

			<div class="sh">Recent activity</div>
			{#if recent.length}
				<ul class="acts">
					{#each recent as a (a.id)}
						<li>
							<span class="who" class:ai={a.actor === 'ai'}>{a.actor === 'ai' ? `✦ ${$aiName}` : 'You'}</span>
							<span class="av">{activityVerb(a)}</span>
							<span class="when">{rel(a.createdAt)}</span>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="faint small">No activity yet.</p>
			{/if}
		{:else if error}
			<div class="perr">{error}</div>
		{:else}
			<div class="sk"><span class="skb" style="width:70%;height:24px"></span><span class="skb" style="width:40%;height:13px"></span><span class="skb" style="width:100%;height:11px"></span><span class="skb" style="width:85%;height:11px"></span></div>
		{/if}
	</div>
</div>

<style>
	.peek-bd { position: fixed; inset: 0; z-index: 60; background: color-mix(in srgb, var(--ink) 18%, transparent); }
	.peek { position: fixed; top: 0; right: 0; bottom: 0; z-index: 61; width: 480px; max-width: 100vw; display: flex; flex-direction: column; background: var(--surface); border-left: 1px solid var(--line-strong); box-shadow: var(--shadow-2); outline: none; }
	.ph { display: flex; align-items: center; gap: 10px; padding: 12px 16px; border-bottom: 1px solid var(--line); flex: none; }
	.pk { font-family: var(--mono); font-size: var(--t-sm); color: var(--ink-2); }
	.sp { flex: 1; }
	.full { font-size: var(--t-sm); color: var(--ink-2); text-decoration: none; }
	.full:hover { color: var(--accent); }
	.close { display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: var(--r-sm); border: 1px solid var(--line); background: var(--surface); color: var(--ink-2); }
	.close:hover { color: var(--ink); border-color: var(--line-strong); }
	.pbody { flex: 1; overflow-y: auto; padding: 20px 20px 40px; }
	.ptitle { font-family: var(--serif); font-weight: 400; font-size: var(--t-xl); line-height: 1.2; letter-spacing: -0.01em; color: var(--ink); margin: 0 0 14px; }
	.props { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; }
	.facts { display: grid; grid-template-columns: 84px minmax(0, 1fr); gap: 8px 12px; margin: 0 0 20px; font-size: var(--t-sm); align-items: center; }
	.facts dt { color: var(--ink-3); font-family: var(--mono); font-size: var(--t-xs); text-transform: uppercase; letter-spacing: 0.05em; }
	.facts dd { margin: 0; color: var(--ink); }
	.lbls { display: flex; flex-wrap: wrap; gap: 6px; }
	.faint { color: var(--ink-3); }
	.small { font-size: var(--t-sm); margin: 0 0 20px; }
	.sh { font-family: var(--mono); font-size: var(--t-xs); letter-spacing: 0.06em; text-transform: uppercase; color: var(--ink-3); padding-top: 14px; border-top: 1px solid var(--line); margin-bottom: 10px; }
	.desc { font-size: var(--t-base); line-height: 1.6; color: var(--ink); margin-bottom: 22px; }
	.acts { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.acts li { display: flex; flex-wrap: wrap; gap: 6px; align-items: baseline; font-size: var(--t-sm); color: var(--ink-2); }
	.who { font-weight: 500; color: var(--ink); }
	.who.ai { color: var(--accent); }
	.when { margin-left: auto; font-size: var(--t-xs); color: var(--ink-3); }
	.perr { padding: 14px; background: var(--danger-soft); border: 1px solid var(--danger); border-radius: var(--r); color: var(--danger); font-size: var(--t-sm); }
	.sk { display: flex; flex-direction: column; gap: 12px; }
	.skb { border-radius: var(--r-sm); background: var(--line); display: block; }
	@media (prefers-reduced-motion: no-preference) { .skb { animation: skshim 1.6s ease-in-out infinite; } }
	@keyframes skshim { 0%, 100% { opacity: 0.55; } 50% { opacity: 1; } }

	@media (max-width: 720px) {
		.peek { width: 100vw; border-left: none; }
		.pbody { padding: 16px 16px 32px; }
	}
</style>
