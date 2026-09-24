<script>
	// The workspace at a glance — Paperclip's dashboard: who is working on
	// what, four headline numbers, fourteen days of activity, what happened last.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { liveEvent } from '$lib/ui.js';
	import { states } from '$lib/store.js';
	import { rel, tokens, usd } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import StackedBars from '$components/StackedBars.svelte';
	import ActivityFeed from '$components/ActivityFeed.svelte';
	import StateIcon from '$components/StateIcon.svelte';
	import { Bot, CircleDot, Coins, Eye, LoaderCircle } from '@lucide/svelte';

	let d = $state(null);
	let failed = $state('');

	// Series colours: validated with the dataviz palette validator on the dark
	// card surface (#191a1c). Run outcomes use the fixed status palette with the
	// neutral between green and red, so the two never sit adjacent.
	const RUN_SERIES = [
		{ key: 'runsSucceeded', label: 'Succeeded', color: '#0ca30c' },
		{ key: 'runsOther', label: 'Other', color: '#6b7280' },
		{ key: 'runsFailed', label: 'Failed', color: '#d03b3b' }
	];
	const MOVE_SERIES = [
		{ key: 'movedReady', label: 'To Ready', color: '#3987e5' },
		{ key: 'movedStarted', label: 'To In Progress / Review', color: '#c98500' },
		{ key: 'movedDone', label: 'To Done', color: '#199e70' }
	];
	const RATE_SERIES = [{ key: 'rate', label: 'Criteria passed', color: '#3987e5' }];

	const rateDays = $derived(
		(d?.days || []).map((x) => ({ ...x, rate: x.finished ? Math.round((x.passed / x.finished) * 100) : 0 }))
	);
	const stOf = (id) => $states.find((s) => s.id === id);

	async function load() {
		try {
			const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
			d = await api.dashboard(tz);
			failed = '';
		} catch (e) {
			failed = e.message;
		}
	}
	onMount(load);

	// Anything that moves the numbers refreshes them, at most every few seconds.
	let pending;
	$effect(() => {
		const ev = $liveEvent;
		if (!ev) return;
		clearTimeout(pending);
		pending = setTimeout(load, 1500);
	});
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Dashboard' }]} />
	<div class="pg-body">
		{#if failed}
			<div class="err">Could not load the dashboard: {failed}</div>
		{:else if d}
			<div class="dash">
				<section>
					<div class="sh">
						<span>Agents</span>
					</div>
					{#if d.agents.length}
						<div class="agents">
							{#each d.agents as a (a.runner)}
								<div class="agent">
									<div class="ah">
										<span class="av"><Bot size={14} strokeWidth={2} /></span>
										<span class="an">{a.runner}</span>
										{#if a.model}<span class="am">{a.model}</span>{/if}
									</div>
									{#if a.issueKey}
										<button class="at" onclick={() => goto('/issue/' + a.issueKey)}>
											{#if a.status === 'running'}<LoaderCircle size={13} strokeWidth={2.2} class="spin" />{/if}
											<span class="att">{a.issueTitle}</span>
											<span class="atk">{a.issueKey}</span>
										</button>
									{/if}
									<div class="af">
										{#if a.status === 'running'}Working · started {rel(a.startedAt)}
										{:else}Finished {rel(a.finishedAt || a.startedAt)} · {a.status}{/if}
									</div>
								</div>
							{/each}
						</div>
					{:else}
						<div class="none">No agent has run yet. Runs appear here as soon as one starts.</div>
					{/if}
				</section>

				<section class="kpis">
					<div class="kpi">
						<div class="kv">{d.kpis.runnersActive}<Bot size={15} strokeWidth={2} /></div>
						<div class="kl">Agents active</div>
						<div class="ks">{d.kpis.runsRunning} running now</div>
					</div>
					<div class="kpi">
						<div class="kv">{d.kpis.inProgress}<CircleDot size={15} strokeWidth={2} /></div>
						<div class="kl">Tasks in progress</div>
						<div class="ks">{d.kpis.open} open, {d.kpis.blocked} blocked</div>
					</div>
					<div class="kpi">
						<div class="kv">{usd(d.kpis.monthCostUsd)}<Coins size={15} strokeWidth={2} /></div>
						<div class="kl">Month spend</div>
						<div class="ks">
							{tokens(d.kpis.monthTokens)} tokens{#if d.kpis.monthNotionalUsd} · {usd(d.kpis.monthNotionalUsd)} of subscription{/if}
						</div>
					</div>
					<a class="kpi link" href="/inbox">
						<div class="kv">{d.kpis.inReview}<Eye size={15} strokeWidth={2} /></div>
						<div class="kl">Awaiting your review</div>
						<div class="ks">In Review — the gate agents cannot pass</div>
					</a>
				</section>

				<section class="charts">
					<StackedBars title="Run activity" series={RUN_SERIES} days={d.days} />
					<StackedBars title="Tasks by status" series={MOVE_SERIES} days={d.days} />
					<StackedBars
						title="Success rate"
						subtitle="Runs whose criteria passed · last 14 days"
						series={RATE_SERIES}
						days={rateDays}
						max={100}
						format={(v, day) => (day.finished ? `${v}% (${day.passed}/${day.finished})` : '—')}
					/>
				</section>

				<section class="two">
					<div>
						<div class="sh"><span>Recent activity</span><a href="/log">View all</a></div>
						<div class="card"><ActivityFeed items={d.activity} /></div>
					</div>
					<div>
						<div class="sh"><span>Recent tasks</span><a href="/tasks">View all</a></div>
						<div class="card">
							{#each d.recentTasks as t (t.key)}
								{@const st = stOf(t.stateId)}
								<button class="rt" onclick={() => goto('/issue/' + t.key)}>
									<StateIcon category={st?.category} color={st?.color} />
									<span class="rtt">{t.title}</span>
									<span class="rtk">{t.key}</span>
									<span class="rtw">{rel(t.updatedAt)}</span>
								</button>
							{/each}
						</div>
					</div>
				</section>
			</div>
		{/if}
	</div>
</div>

<style>
	.dash {
		padding: 20px clamp(16px, 3vw, 28px) 40px;
		display: flex;
		flex-direction: column;
		gap: 22px;
		max-width: 1280px;
	}
	.sh {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		font-size: 11.5px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-faint);
		margin-bottom: 8px;
	}
	.sh a {
		text-transform: none;
		letter-spacing: 0;
		font-size: 12px;
		color: var(--text-dim);
	}
	.sh a:hover {
		color: var(--text);
	}
	.agents {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
		gap: 10px;
	}
	.agent {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}
	.ah {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.av {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		display: grid;
		place-items: center;
		color: var(--text-dim);
	}
	.an {
		font-weight: 500;
		text-transform: capitalize;
	}
	.am {
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--text-faint);
	}
	.at {
		display: flex;
		align-items: center;
		gap: 7px;
		background: none;
		border: none;
		padding: 0;
		color: var(--text-dim);
		text-align: left;
		font-size: 13px;
		min-width: 0;
	}
	.at:hover {
		color: var(--text);
	}
	.at :global(.spin) {
		animation: spin 1s linear infinite;
		color: var(--st-progress);
		flex-shrink: 0;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.att {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.atk {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--text-faint);
		flex-shrink: 0;
	}
	.af {
		font-size: 12px;
		color: var(--text-faint);
		text-align: right;
	}
	.kpis {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 10px;
	}
	.kpi {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.kpi.link:hover {
		border-color: var(--border-strong);
	}
	.kv {
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 26px;
		font-weight: 600;
		letter-spacing: -0.02em;
		font-variant-numeric: tabular-nums;
	}
	.kv :global(svg) {
		color: var(--text-faint);
	}
	.kl {
		font-size: 13px;
		color: var(--text);
	}
	.ks {
		font-size: 12px;
		color: var(--text-faint);
	}
	.charts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
		gap: 10px;
	}
	.two {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
		gap: 16px;
	}
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 6px;
	}
	.rt {
		display: flex;
		align-items: center;
		gap: 9px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text);
		padding: 7px 8px;
		border-radius: 7px;
		text-align: left;
		font-size: 13px;
	}
	.rt:hover {
		background: var(--bg-hover);
	}
	.rtt {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rtk {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--text-faint);
	}
	.rtw {
		font-size: 12px;
		color: var(--text-faint);
		min-width: 52px;
		text-align: right;
	}
	.none,
	.err {
		color: var(--text-faint);
		font-size: 13px;
		padding: 14px;
		border: 1px dashed var(--border-strong);
		border-radius: var(--radius-lg);
	}
	.err {
		margin: 20px;
		color: #fca5a5;
	}
</style>
