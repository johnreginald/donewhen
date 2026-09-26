<script>
	// Running: what the agents are doing right now, what is waiting, and what
	// they finished — the page to leave open while work happens. Everything is
	// kept live from job, run and criterion events; nothing here starts work.
	import { onMount } from 'svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import { api } from '$lib/api.js';
	import { activeJobs, agents, issues } from '$lib/store.js';
	import { onLive } from '$lib/ui.js';
	import { rel, duration, tokens } from '$lib/format.js';
	import { hostOnline, harnessName } from '$lib/harness.js';
	import { Bot, Check, X, LoaderCircle, Clock, Terminal, MessageSquare, ShieldCheck, Flag, ArrowRight, Server, Wrench } from '@lucide/svelte';

	const KIND = {
		run_ticket: { label: 'Run', doing: 'Working', icon: Terminal },
		chat: { label: 'Conversation', doing: 'Thinking', icon: MessageSquare },
		verify: { label: 'Verify', doing: 'Verifying', icon: ShieldCheck },
		finish: { label: 'Finish', doing: 'Finishing', icon: Flag }
	};

	let hosts = $state([]);
	let runs = $state([]); // recent finished runs, newest first
	let failedStarts = $state([]); // jobs that failed before a run existed
	let criteria = $state({}); // issue id -> criteria
	let feed = $state({}); // issue id -> the running run's last few readable lines
	let runIssue = {}; // run id -> issue id
	let filter = $state('all');
	let now = $state(Date.now());

	const agentOf = (id) => $agents.find((a) => a.id === id);
	const issueOf = (id) => $issues.find((i) => i.id === id);
	const since = (iso) => duration(iso, new Date(now).toISOString());

	const working = $derived($activeJobs.filter((j) => j.status === 'claimed'));
	// Oldest first: the order a host will take them in.
	const queued = $derived([...$activeJobs.filter((j) => j.status === 'queued')].reverse());

	// ── live transcript preview ──────────────────────────────────────────
	// "→ tool input" is a tool line, plain text is the agent speaking;
	// results and meta lines stay out of the preview.
	function item(l) {
		if (l.startsWith('→ ')) {
			const b = l.slice(2);
			const sp = b.indexOf(' ');
			return { tool: sp > 0 ? b.slice(0, sp) : b, text: sp > 0 ? b.slice(sp + 1) : '' };
		}
		if (l.startsWith('· ') || l.startsWith('← ') || l.startsWith('✗ ') || !l.trim()) return null;
		return { text: l.replace(/\s+/g, ' ') };
	}
	function push(issueId, lines) {
		const items = lines.map(item).filter(Boolean);
		if (!items.length) return;
		feed = { ...feed, [issueId]: [...(feed[issueId] || []), ...items].slice(-4) };
	}
	async function loadCriteria(issueId, key) {
		const c = await api.criteria(key || issueId).catch(() => null);
		if (c) criteria = { ...criteria, [issueId]: c };
	}
	async function watch(j) {
		if (j.kind !== 'chat') loadCriteria(j.issueId, j.issueKey);
		const rs = (await api.issueRuns(j.issueId).catch(() => [])) || [];
		const r = rs.find((x) => x.status === 'running');
		if (!r || runIssue[r.id]) return;
		runIssue[r.id] = j.issueId;
		const evs = (await api.get(`/runs/${r.id}/events`).catch(() => [])) || [];
		feed = { ...feed, [j.issueId]: [] };
		push(j.issueId, evs.map((e) => e.text));
	}
	const watched = new Set();
	$effect(() => {
		for (const j of working) {
			if (watched.has(j.id)) continue;
			watched.add(j.id);
			watch(j);
		}
	});

	// ── history ──────────────────────────────────────────────────────────
	async function loadHistory() {
		const [rs, js, hs] = await Promise.all([
			api.runs({ limit: 80 }).catch(() => []),
			api.jobs({ limit: 80 }).catch(() => []),
			api.hosts().catch(() => [])
		]);
		runs = (rs || []).filter((r) => r.status !== 'running' && r.status !== 'queued');
		// A job that failed before any run of its ticket began: no run
		// record says why, so the job does.
		const runsList = rs || [];
		failedStarts = (js || []).filter(
			(j) =>
				j.status === 'failed' && j.issueId && j.kind !== 'test_env' &&
				!runsList.some((r) => r.issueId === j.issueId && new Date(r.startedAt) >= new Date(j.claimedAt || j.createdAt) &&
					new Date(r.startedAt) <= new Date(j.finishedAt || Date.now()))
		);
		hosts = hs || [];
	}
	onMount(loadHistory);
	onMount(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});
	onMount(() =>
		onLive((ev) => {
			if (!ev) return;
			if (ev.type === 'run.events' && runIssue[ev.runId]) push(runIssue[ev.runId], ev.lines || []);
			if (ev.run) {
				if (ev.run.status === 'running') {
					const j = $activeJobs.find((x) => x.issueId === ev.run.issueId);
					if (j) watch(j);
				} else loadHistory();
			}
			if (ev.job && (ev.job.status === 'succeeded' || ev.job.status === 'failed')) loadHistory();
			if (ev.type === 'host.updated') api.hosts().then((h) => (hosts = h || [])).catch(() => {});
			if (ev.type?.startsWith('criteri') && ev.issueId && criteria[ev.issueId]) loadCriteria(ev.issueId, issueOf(ev.issueId)?.key);
		})
	);

	// A run's outcome, as the person reading the list cares about it.
	function outcome(r) {
		if (r.kind === 'chat') return r.status === 'succeeded' ? { label: 'Replied', tone: 'ok' } : { label: 'No reply', tone: 'bad' };
		if (r.status === 'aborted') return { label: 'Stopped', tone: 'mute' };
		if (r.verdict === 'passed') return { label: 'Passed', tone: 'ok' };
		if (r.verdict === 'failed') return { label: 'Checks failed', tone: 'bad' };
		if (r.verdict === 'blocked') return { label: 'Blocked', tone: 'warn' };
		return r.status === 'succeeded' ? { label: 'Done', tone: 'ok' } : { label: 'Failed', tone: 'bad' };
	}

	const finished = $derived.by(() => {
		const rows = [
			...runs.map((r) => ({ id: r.id, at: r.finishedAt || r.startedAt, run: r, o: outcome(r) })),
			...failedStarts.map((j) => ({ id: j.id, at: j.finishedAt || j.createdAt, job: j, o: { label: "Couldn't start", tone: 'bad' } }))
		].sort((a, b) => new Date(b.at) - new Date(a.at));
		return rows.filter((x) =>
			filter === 'failed' ? x.o.tone === 'bad' || x.o.tone === 'warn'
			: filter === 'passed' ? x.o.label === 'Passed'
			: filter === 'chat' ? x.run?.kind === 'chat'
			: true
		);
	});
	// Grouped by day: Today, Yesterday, then the date.
	const groups = $derived.by(() => {
		const out = [];
		const today = new Date(now).toDateString();
		const yest = new Date(now - 86400000).toDateString();
		for (const x of finished.slice(0, 60)) {
			const d = new Date(x.at).toDateString();
			const label = d === today ? 'Today' : d === yest ? 'Yesterday'
				: new Date(x.at).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
			if (!out.length || out[out.length - 1].label !== label) out.push({ label, rows: [] });
			out[out.length - 1].rows.push(x);
		}
		return out;
	});

	const isToday = (iso) => iso && new Date(iso).toDateString() === new Date(now).toDateString();
	const passedToday = $derived(runs.filter((r) => r.kind === 'work' && r.verdict === 'passed' && isToday(r.finishedAt)).length);
	const failedToday = $derived(
		runs.filter((r) => outcome(r).tone === 'bad' && isToday(r.finishedAt || r.startedAt)).length +
			failedStarts.filter((j) => isToday(j.finishedAt || j.createdAt)).length
	);
	const onlineHosts = $derived(hosts.filter(hostOnline));
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Running' }]}>
		<span class="hosts">
			{#each hosts as h (h.id)}
				<span class="host" class:on={hostOnline(h)} title={hostOnline(h) ? `${h.name} is online` : `${h.name} last seen ${rel(h.lastSeenAt)}`}>
					<Server size={13} strokeWidth={2} /><span class="hd"></span>{h.name}
				</span>
			{:else}
				<span class="host">No runner host has reported</span>
			{/each}
		</span>
	</PageHeader>

	<div class="body">
		<div class="stats">
			<div class="stat live" class:zero={!working.length}><span class="v">{working.length}</span><span class="l">Working</span></div>
			<div class="stat" class:zero={!queued.length}><span class="v">{queued.length}</span><span class="l">Up next</span></div>
			<div class="stat ok" class:zero={!passedToday}><span class="v">{passedToday}</span><span class="l">Passed today</span></div>
			<div class="stat bad" class:zero={!failedToday}><span class="v">{failedToday}</span><span class="l">Failed today</span></div>
		</div>

		<section>
			<h2>Working now</h2>
			{#if working.length}
				<div class="cards">
					{#each working as j (j.id)}
						{@const a = agentOf(j.agentId)}
						{@const k = KIND[j.kind] || KIND.run_ticket}
						{@const crit = criteria[j.issueId] || []}
						{@const done = crit.filter((c) => c.done).length}
						<a class="card" href="/issue/{j.issueKey}">
							<div class="ch">
								<span class="av"><Bot size={15} strokeWidth={2} /></span>
								<span class="who">
									<span class="an">{a?.name || 'Agent'}</span>
									<span class="am">{harnessName(a?.harness || '')}{a?.model ? ` · ${a.model}` : ''}</span>
								</span>
								<span class="kind"><k.icon size={12} strokeWidth={2.2} />{k.label}</span>
								<span class="timer" title={j.host ? `On ${j.host}` : ''}><Clock size={12} strokeWidth={2.2} />{since(j.claimedAt || j.createdAt)}</span>
							</div>
							<div class="tt"><span class="key">{j.issueKey}</span>{issueOf(j.issueId)?.title || ''}</div>
							{#if crit.length}
								<div class="prog" title="{done} of {crit.length} done-when items met">
									<div class="bar"><span style:width="{(done / crit.length) * 100}%"></span></div>
									<span class="pn">{done}/{crit.length} done-when</span>
								</div>
							{/if}
							<div class="feed">
								{#each feed[j.issueId] || [] as f, i (i)}
									{#if f.tool}
										<div class="fl tool"><Wrench size={11} strokeWidth={2.2} /><span class="fn">{f.tool}</span><span class="fx">{f.text}</span></div>
									{:else}
										<div class="fl say">{f.text}</div>
									{/if}
								{:else}
									<div class="fl wait"><LoaderCircle size={12} strokeWidth={2.2} class="spin" />{j.kind === 'run_ticket' ? 'Setting up the worktree and checking the harness…' : `${k.doing}…`}</div>
								{/each}
							</div>
							<div class="cf"><span class="doing"><span class="pulse"></span>{k.doing}</span><span class="open">Open task <ArrowRight size={12} strokeWidth={2.2} /></span></div>
						</a>
					{/each}
				</div>
			{:else}
				<div class="idle">
					<span class="idle-ic"><Bot size={20} strokeWidth={1.8} /></span>
					<div>
						<div class="it">No agent is working right now</div>
						<div class="is">
							{#if !onlineHosts.length}No runner host is online — start it on your Mac: <code>orchestrator service restart</code>
							{:else}Run a task, or press Start task on one, and it shows up here as it works.{/if}
						</div>
					</div>
				</div>
			{/if}
		</section>

		{#if queued.length}
			<section>
				<h2>Up next</h2>
				<div class="list">
					{#each queued as j, i (j.id)}
						{@const k = KIND[j.kind] || KIND.run_ticket}
						<a class="qrow" href="/issue/{j.issueKey}">
							<span class="pos">{i + 1}</span>
							<span class="qt"><span class="key">{j.issueKey}</span>{issueOf(j.issueId)?.title || ''}</span>
							<span class="kind"><k.icon size={12} strokeWidth={2.2} />{k.label}</span>
							<span class="qa"><Bot size={12} strokeWidth={2} />{agentOf(j.agentId)?.name || 'Agent'}</span>
							<span class="qw">waiting {since(j.createdAt)}</span>
						</a>
					{/each}
				</div>
			</section>
		{/if}

		<section>
			<div class="fh">
				<h2>Finished</h2>
				<div class="seg" role="tablist">
					{#each [['all', 'All'], ['passed', 'Passed'], ['failed', 'Failed'], ['chat', 'Conversations']] as [key, label]}
						<button role="tab" aria-selected={filter === key} class:on={filter === key} onclick={() => (filter = key)}>{label}</button>
					{/each}
				</div>
			</div>
			{#each groups as g (g.label)}
				<div class="day">{g.label}</div>
				<div class="list">
					{#each g.rows as x (x.id)}
						{@const src = x.run || x.job}
						<a class="frow" href="/issue/{src.issueKey}">
							<span class="oc {x.o.tone}">
								{#if x.o.tone === 'ok'}<Check size={13} strokeWidth={2.6} />{:else}<X size={13} strokeWidth={2.6} />{/if}
							</span>
							<span class="ft">
								<span class="ftt"><span class="key">{src.issueKey}</span>{x.run?.issueTitle || issueOf(src.issueId)?.title || ''}</span>
								{#if x.job?.error}<span class="fe">{x.job.error.replace(/^not starting [A-Z0-9]+-\d+: /, '')}</span>{/if}
							</span>
							<span class="chip {x.o.tone}">{x.o.label}</span>
							<span class="fa"><Bot size={12} strokeWidth={2} />{agentOf(src.agentId)?.name || 'Agent'}</span>
							<span class="fm">
								{#if x.run}{duration(x.run.startedAt, x.run.finishedAt)}{#if x.run.tokens?.total} · {tokens(x.run.tokens.total)} tok{/if}{/if}
							</span>
							<span class="fw">{new Date(x.at).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}</span>
						</a>
					{/each}
				</div>
			{:else}
				<div class="none">Nothing {filter === 'all' ? '' : 'like that '}yet.</div>
			{/each}
		</section>
	</div>
</div>

<style>
	.pg {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.body {
		flex: 1;
		overflow-y: auto;
		padding: 18px clamp(16px, 3vw, 32px) 56px;
		display: flex;
		flex-direction: column;
		gap: 26px;
	}
	h2 {
		margin: 0 0 10px;
		font-size: 13px;
		font-weight: 600;
		color: var(--text);
	}
	.hosts {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.host {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--text-faint);
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 3px 10px;
	}
	.host.on {
		color: var(--text-dim);
	}
	.hd {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--text-faint);
	}
	.host.on .hd {
		background: #4ade80;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 10px;
	}
	.stat {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.stat .v {
		font-size: 24px;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		line-height: 1.1;
	}
	.stat .l {
		font-size: 12px;
		color: var(--text-faint);
	}
	.stat.live .v {
		color: var(--st-progress);
	}
	.stat.ok .v {
		color: #4ade80;
	}
	.stat.bad .v {
		color: #f87171;
	}
	.stat.zero .v {
		color: var(--text-faint);
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
		gap: 12px;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 10px;
		background: var(--bg-elev);
		border: 1px solid color-mix(in srgb, var(--st-progress) 40%, var(--border));
		border-radius: 12px;
		padding: 14px;
		color: var(--text);
		text-decoration: none;
		transition: border-color 0.15s;
	}
	.card:hover {
		border-color: var(--st-progress);
	}
	.ch {
		display: flex;
		align-items: center;
		gap: 9px;
	}
	.av {
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border-radius: 50%;
		background: color-mix(in srgb, var(--st-progress) 18%, var(--bg-elev2));
		color: var(--st-progress);
		flex: none;
	}
	.who {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
		line-height: 1.25;
	}
	.an {
		font-size: 13px;
		font-weight: 600;
	}
	.am {
		font-size: 11.5px;
		color: var(--text-faint);
		font-family: var(--mono);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.kind {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: 11.5px;
		color: var(--text-dim);
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 1px 7px;
		white-space: nowrap;
		justify-self: start;
	}
	.timer {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: 12px;
		font-variant-numeric: tabular-nums;
		color: var(--text-dim);
		white-space: nowrap;
	}
	.tt {
		font-size: 14px;
		line-height: 1.4;
		font-weight: 500;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.key {
		font-family: var(--mono);
		font-size: 11.5px;
		font-weight: 400;
		color: var(--text-faint);
		margin-right: 7px;
	}
	.prog {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.bar {
		flex: 1;
		height: 4px;
		border-radius: 2px;
		background: var(--bg-elev2);
		overflow: hidden;
	}
	.bar span {
		display: block;
		height: 100%;
		background: #4ade80;
		border-radius: 2px;
		transition: width 0.3s;
	}
	.pn {
		font-size: 11.5px;
		color: var(--text-faint);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
	.feed {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-height: 88px;
	}
	.fl {
		font-size: 12px;
		line-height: 1.45;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--text-faint);
	}
	.fl.tool {
		display: flex;
		align-items: center;
		gap: 6px;
		font-family: var(--mono);
		font-size: 11.5px;
	}
	.fl.tool :global(svg) {
		flex: none;
	}
	.fn {
		color: var(--text-dim);
	}
	.fx {
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.fl.wait {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.fl.wait :global(.spin) {
		animation: rp-spin 1.2s linear infinite;
	}
	.fl:last-child:not(.wait) {
		color: var(--text);
	}
	.cf {
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 12px;
	}
	.doing {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		color: var(--st-progress);
		font-weight: 500;
	}
	.pulse {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--st-progress);
		animation: rp-pulse 1.4s ease-in-out infinite;
	}
	.open {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		color: var(--text-faint);
	}
	.card:hover .open {
		color: var(--text);
	}
	@keyframes rp-spin {
		to {
			transform: rotate(360deg);
		}
	}
	@keyframes rp-pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.3;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.pulse,
		.fl.wait :global(.spin) {
			animation: none;
		}
	}
	.idle {
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 18px;
		border: 1px dashed var(--border-strong);
		border-radius: 12px;
	}
	.idle-ic {
		display: grid;
		place-items: center;
		width: 40px;
		height: 40px;
		border-radius: 50%;
		background: var(--bg-elev2);
		color: var(--text-faint);
		flex: none;
	}
	.it {
		font-size: 13.5px;
		color: var(--text);
	}
	.is {
		font-size: 12.5px;
		color: var(--text-faint);
		margin-top: 2px;
	}
	.is code {
		font-family: var(--mono);
		font-size: 11.5px;
	}
	.list {
		border: 1px solid var(--border);
		border-radius: 10px;
		overflow: hidden;
		background: var(--bg-elev);
	}
	.qrow,
	.frow {
		display: grid;
		align-items: center;
		gap: 12px;
		padding: 9px 12px;
		color: var(--text);
		text-decoration: none;
		border-top: 1px solid var(--border);
		font-size: 13px;
	}
	.list > :first-child {
		border-top: none;
	}
	.qrow:hover,
	.frow:hover {
		background: var(--bg-hover);
	}
	.qrow {
		grid-template-columns: 22px minmax(0, 1fr) auto minmax(0, 160px) 110px;
	}
	.pos {
		display: grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--bg-elev2);
		font-size: 11px;
		color: var(--text-dim);
		font-variant-numeric: tabular-nums;
	}
	.qt,
	.ftt {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.qa,
	.fa {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12.5px;
		color: var(--text-dim);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	.qw,
	.fm,
	.fw {
		font-size: 12px;
		color: var(--text-faint);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
		text-align: right;
	}
	.fh {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 4px;
	}
	.fh h2 {
		margin: 0;
	}
	.seg {
		display: inline-flex;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2px;
	}
	.seg button {
		background: none;
		border: none;
		color: var(--text-faint);
		font-size: 12px;
		padding: 4px 10px;
		border-radius: 6px;
	}
	.seg button.on {
		background: var(--bg-hover);
		color: var(--text);
	}
	.day {
		font-size: 11.5px;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--text-faint);
		margin: 14px 2px 6px;
	}
	.frow {
		grid-template-columns: 22px minmax(0, 1fr) 104px minmax(0, 150px) 120px 64px;
	}
	.oc {
		display: grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 50%;
	}
	.oc.ok {
		background: color-mix(in srgb, #4ade80 18%, transparent);
		color: #4ade80;
	}
	.oc.bad {
		background: color-mix(in srgb, #f87171 18%, transparent);
		color: #f87171;
	}
	.oc.warn {
		background: color-mix(in srgb, #fbbf24 18%, transparent);
		color: #fbbf24;
	}
	.oc.mute {
		background: var(--bg-elev2);
		color: var(--text-faint);
	}
	.ft {
		display: flex;
		flex-direction: column;
		min-width: 0;
		gap: 2px;
	}
	.fe {
		font-size: 12px;
		color: #fca5a5;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.chip {
		justify-self: start;
		font-size: 11.5px;
		border-radius: 999px;
		padding: 1px 8px;
		border: 1px solid var(--border);
		color: var(--text-dim);
		white-space: nowrap;
	}
	.chip.ok {
		color: #4ade80;
		border-color: color-mix(in srgb, #4ade80 35%, var(--border));
	}
	.chip.bad {
		color: #f87171;
		border-color: color-mix(in srgb, #f87171 35%, var(--border));
	}
	.chip.warn {
		color: #fbbf24;
		border-color: color-mix(in srgb, #fbbf24 35%, var(--border));
	}
	.none {
		font-size: 13px;
		color: var(--text-faint);
		padding: 12px 2px;
	}
	@media (max-width: 760px) {
		.stats {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.cards {
			grid-template-columns: minmax(0, 1fr);
		}
		.qrow,
		.frow {
			grid-template-columns: 22px minmax(0, 1fr) auto;
		}
		.qrow .kind,
		.qrow .qw,
		.frow .fa,
		.frow .fm,
		.frow .fw {
			display: none;
		}
		.hosts {
			display: none;
		}
	}
</style>
