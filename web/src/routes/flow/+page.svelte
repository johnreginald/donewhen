<script>
	// Flow: whether the factory is getting faster. Attempts per passed ticket,
	// pass rate and time per harness, what the reviewers find, and what is
	// waiting for the person who reviews. Read-only.
	import { onMount } from 'svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import { api } from '$lib/api.js';
	import { tokens } from '$lib/format.js';
	import { harnessName } from '$lib/harness.js';

	let days = $state(7);
	let f = $state(null);
	let err = $state('');

	async function load() {
		try {
			f = await api.get(`/flow?days=${days}`);
			err = '';
		} catch (e) {
			err = e.message;
		}
	}
	onMount(load);

	const pct = (x) => `${Math.round((x || 0) * 100)}%`;
	const num = (x, d = 1) => (x || 0).toFixed(d);
	const mins = (m) => (m >= 60 ? `${num(m / 60)} h` : `${num(m)} min`);
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Flow' }]}>
		<select bind:value={days} onchange={load} aria-label="Window">
			{#each [[1, 'Today'], [7, 'Last 7 days'], [30, 'Last 30 days']] as [v, l]}<option value={v}>{l}</option>{/each}
		</select>
	</PageHeader>

	<div class="body">
		{#if err}<p class="err">{err}</p>{/if}
		{#if f}
			<div class="stats">
				<div class="stat"><span class="v">{f.ticketsPassed}</span><span class="l">Tickets passed</span></div>
				<div class="stat" class:warn={f.attemptsPerPassed > 1.5}><span class="v">{num(f.attemptsPerPassed)}</span><span class="l">Attempts per passed ticket</span></div>
				<div class="stat"><span class="v">{f.medianTicketHours >= 1 ? `${num(f.medianTicketHours)} h` : `${Math.round(f.medianTicketHours * 60)} min`}</span><span class="l">Median time to pass</span></div>
				<div class="stat" class:warn={f.waitingForReview > 3}><span class="v">{f.waitingForReview}</span><span class="l">Waiting for your review{#if f.waitingForReview} · median {num(f.waitingHoursMedian)} h{/if}</span></div>
			</div>

			<section>
				<h2>By harness</h2>
				<table>
					<thead><tr><th>Harness</th><th>Runs</th><th>Passed</th><th>Pass rate</th><th>Median run</th><th>Tokens</th><th>Metered</th></tr></thead>
					<tbody>
						{#each f.runners as r (r.runner)}
							<tr>
								<td>{harnessName(r.runner)}</td><td>{r.runs}</td><td>{r.passed}</td>
								<td class:good={r.passRate >= 0.7} class:bad={r.passRate < 0.4}>{pct(r.passRate)}</td>
								<td>{mins(r.medianMinutes)}</td><td>{tokens(r.tokens)}</td><td>{r.costUsd ? `$${num(r.costUsd, 2)}` : '—'}</td>
							</tr>
						{:else}
							<tr><td colspan="7" class="faint">No runs in this window.</td></tr>
						{/each}
					</tbody>
				</table>
			</section>

			<section>
				<h2>Reviews</h2>
				<div class="stats">
					<div class="stat"><span class="v">{f.ticketReviews ? pct(f.ticketQualifyRate) : '—'}</span><span class="l">Tickets qualified first time ({f.ticketReviews} reviewed)</span></div>
					<div class="stat"><span class="v">{f.codeReviews ? pct(f.codeFirstPassRate) : '—'}</span><span class="l">Changes with nothing blocking at first review ({f.codeReviews} reviews)</span></div>
					<div class="stat"><span class="v">{f.blockingFound}</span><span class="l">Blocking findings caught before you</span></div>
				</div>
			</section>
			<p class="faint">Targets: 1.5 attempts or fewer per ticket, 70% pass rate or better. Subscription runs show no metered cost.</p>
		{/if}
	</div>
</div>

<style>
	.pg {
		display: flex;
		flex-direction: column;
		min-height: 100%;
	}
	.body {
		padding: 16px 24px 40px;
		display: flex;
		flex-direction: column;
		gap: 20px;
		max-width: 1000px;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 10px;
	}
	.stat {
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 4px;
		background: var(--bg-elev);
	}
	.stat .v {
		font-size: 24px;
		font-weight: 600;
	}
	.stat .l {
		font-size: 12px;
		color: var(--text-dim);
	}
	.stat.warn .v {
		color: #fcd34d;
	}
	h2 {
		font-size: 13px;
		font-weight: 600;
		margin: 0 0 8px;
		color: var(--text-dim);
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	th,
	td {
		text-align: left;
		padding: 8px 10px;
		border-bottom: 1px solid var(--border);
	}
	th {
		font-weight: 500;
		color: var(--text-faint);
		font-size: 12px;
	}
	td.good {
		color: #86efac;
	}
	td.bad {
		color: #fca5a5;
	}
	.faint {
		color: var(--text-faint);
		font-size: 12px;
	}
	.err {
		color: #fca5a5;
	}
	select {
		font-size: 12.5px;
	}
	@media (max-width: 640px) {
		.body {
			padding: 12px 16px 32px;
		}
		table {
			display: block;
			overflow-x: auto;
		}
	}
</style>
