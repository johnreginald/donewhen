<script>
	// The ticket's evidence, for the person reviewing it: what the ticket
	// reviewer said before it was built, what a second vendor found in the
	// change and its review guide, a preview link, and screenshots of each
	// screen beside the prototype screen they must match.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import { harnessName } from '$lib/harness.js';
	import Markdown from '$components/Markdown.svelte';
	import { ClipboardCheck, ScanSearch, ExternalLink, Image, Play } from '@lucide/svelte';

	let { issue, stateName } = $props();

	let reviews = $state([]);
	let evidence = $state([]);
	let loaded = $state(false);

	async function load() {
		const [r, e] = await Promise.all([
			api.issueReviews(issue.id).catch(() => []),
			api.issueEvidence(issue.id).catch(() => [])
		]);
		reviews = r || [];
		evidence = e || [];
		loaded = true;
	}
	onMount(() => {
		load();
		return onLive((ev) => {
			if (ev.issueId !== issue.id) return;
			if (ev.type === 'review.saved' || ev.type === 'evidence.saved' || ev.type === 'run.finished') load();
		});
	});

	const ticketReview = $derived(reviews.find((r) => r.kind === 'ticket'));
	const codeReview = $derived(reviews.find((r) => r.kind === 'code'));
	const preview = $derived(evidence.find((e) => e.name.toLowerCase().endsWith('.url'))?.text || '');
	const notes = $derived(evidence.filter((e) => /\.(md|txt)$/i.test(e.name)));

	// Screenshots pair up: home.png is what was built, home.prototype.png what
	// it must match.
	const shots = $derived.by(() => {
		const imgs = evidence.filter((e) => e.contentType.startsWith('image/'));
		const byBase = new Map();
		for (const e of imgs) {
			const m = e.name.match(/^(.*?)(\.prototype)?\.(png|jpe?g|webp|gif)$/i);
			const base = m ? m[1] : e.name;
			const slot = byBase.get(base) || { base };
			if (m && m[2]) slot.proto = e;
			else slot.built = e;
			byBase.set(base, slot);
		}
		return [...byBase.values()];
	});

	const blockingOf = (r) => (r?.findings || []).filter((f) => f.severity === 'blocking');
	const minorOf = (r) => (r?.findings || []).filter((f) => f.severity !== 'blocking');
	const loc = (f) => (f.file ? (f.line ? `${f.file}:${f.line}` : f.file) : '');

	async function runAnyway() {
		try {
			await api.post(`/issues/${issue.key}/run`, { skipQualify: true });
			showToast('Queued without the ticket review');
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
</script>

{#if loaded && (reviews.length || evidence.length)}
	<section class="ev">
		<header><span class="t">Evidence</span></header>

		{#if ticketReview}
			<div class="card">
				<div class="row">
					<ClipboardCheck size={14} />
					<span class="k">Ticket review</span>
					<span class="chip {ticketReview.verdict}">{ticketReview.verdict === 'pass' ? 'qualified' : 'needs changes'}</span>
					<span class="faint">by {harnessName(ticketReview.reviewer)} · {rel(ticketReview.createdAt)}</span>
					<span class="sp"></span>
					{#if ticketReview.verdict === 'changes' && stateName === 'Aligning'}
						<button class="btn sm" onclick={runAnyway}><Play size={12} />Run anyway</button>
					{/if}
				</div>
				{#if ticketReview.summary}<p class="sum">{ticketReview.summary}</p>{/if}
				{#if ticketReview.findings.length}
					<ul class="fs">
						{#each ticketReview.findings as f}
							<li class={f.severity}><b>{f.severity === 'blocking' ? 'Blocking' : 'Minor'}</b> {f.issue}{#if f.fix}<span class="fix"> — {f.fix}</span>{/if}</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/if}

		{#if codeReview}
			<div class="card">
				<div class="row">
					<ScanSearch size={14} />
					<span class="k">Code review</span>
					<span class="chip {codeReview.verdict}">{codeReview.verdict === 'pass' ? 'no blocking findings' : `${blockingOf(codeReview).length} blocking`}</span>
					<span class="faint">
						{harnessName(codeReview.reviewer)} reviewed {harnessName(codeReview.builder)}'s work{#if codeReview.round > 1} · round {codeReview.round}{/if} · {rel(codeReview.createdAt)}
					</span>
				</div>
				{#if codeReview.summary}<p class="sum">{codeReview.summary}</p>{/if}
				{#if codeReview.findings.length}
					<ul class="fs">
						{#each [...blockingOf(codeReview), ...minorOf(codeReview)] as f}
							<li class={f.severity}>
								<b>{f.severity === 'blocking' ? 'Blocking' : 'Minor'}</b>
								{#if loc(f)}<code>{loc(f)}</code>{/if}
								{f.issue}{#if f.fix}<span class="fix"> — {f.fix}</span>{/if}
							</li>
						{/each}
					</ul>
				{/if}
				{#if codeReview.guideMd}
					<details open>
						<summary>Review guide</summary>
						<div class="guide"><Markdown source={codeReview.guideMd} /></div>
					</details>
				{/if}
			</div>
		{/if}

		{#if preview}
			<a class="preview" href={preview} target="_blank" rel="noopener noreferrer"><ExternalLink size={13} />Open the preview</a>
		{/if}

		{#if shots.length}
			<div class="shots">
				{#each shots as s (s.base)}
					<figure>
						<figcaption><Image size={12} />{s.base}</figcaption>
						<div class="pair" class:single={!s.proto || !s.built}>
							{#if s.built}
								<a href={api.evidenceUrl(s.built.id)} target="_blank" rel="noopener"><img src={api.evidenceUrl(s.built.id)} alt="{s.base} as built" loading="lazy" /><span>Built</span></a>
							{/if}
							{#if s.proto}
								<a href={api.evidenceUrl(s.proto.id)} target="_blank" rel="noopener"><img src={api.evidenceUrl(s.proto.id)} alt="{s.base} prototype" loading="lazy" /><span>Prototype</span></a>
							{/if}
						</div>
					</figure>
				{/each}
			</div>
		{/if}

		{#each notes as n (n.id)}
			<div class="card"><div class="row"><span class="k">{n.name}</span></div><div class="guide"><Markdown source={n.text} /></div></div>
		{/each}
	</section>
{/if}

<style>
	.ev {
		border: 1px solid var(--border-strong);
		border-radius: 12px;
		background: var(--bg-elev);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.t {
		font-weight: 600;
		font-size: 13.5px;
	}
	.card {
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		background: var(--bg);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		color: var(--text-dim);
	}
	.k {
		font-weight: 600;
		font-size: 13px;
		color: var(--text);
	}
	.sp {
		flex: 1;
	}
	.faint {
		font-size: 12px;
		color: var(--text-faint);
	}
	.chip {
		font-size: 11.5px;
		padding: 1px 8px;
		border-radius: 999px;
		border: 1px solid var(--border);
	}
	.chip.pass {
		color: #86efac;
		border-color: color-mix(in srgb, #86efac 40%, transparent);
	}
	.chip.changes {
		color: #fcd34d;
		border-color: color-mix(in srgb, #fcd34d 40%, transparent);
	}
	.sum {
		margin: 0;
		font-size: 13px;
		color: var(--text);
		line-height: 1.5;
	}
	.fs {
		margin: 0;
		padding-left: 18px;
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12.5px;
		line-height: 1.5;
		color: var(--text-dim);
	}
	.fs li.blocking b {
		color: #fca5a5;
	}
	.fs li b {
		font-weight: 600;
		margin-right: 4px;
	}
	.fs code {
		font-family: var(--mono);
		font-size: 11.5px;
		margin-right: 4px;
		color: var(--text);
	}
	.fix {
		color: var(--text-faint);
	}
	details summary {
		cursor: pointer;
		font-size: 12.5px;
		color: var(--text-dim);
	}
	.guide {
		font-size: 13px;
		margin-top: 6px;
	}
	.preview {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 13px;
	}
	.shots {
		display: grid;
		gap: 12px;
	}
	figure {
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	figcaption {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: 12px;
		color: var(--text-dim);
		font-family: var(--mono);
	}
	.pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
	}
	.pair.single {
		grid-template-columns: minmax(0, 420px);
	}
	.pair a {
		position: relative;
		display: block;
		border: 1px solid var(--border);
		border-radius: 8px;
		overflow: hidden;
		background: var(--bg);
	}
	.pair img {
		display: block;
		width: 100%;
		height: auto;
	}
	.pair span {
		position: absolute;
		top: 6px;
		left: 6px;
		font-size: 11px;
		padding: 1px 6px;
		border-radius: 6px;
		background: color-mix(in srgb, var(--bg) 80%, transparent);
		color: var(--text-dim);
	}
	@media (max-width: 640px) {
		.pair {
			grid-template-columns: 1fr;
		}
	}
</style>
