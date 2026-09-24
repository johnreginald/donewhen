<script>
	// What agents cost, Paperclip's Costs view: metered spend kept apart from
	// subscription usage, which costs nothing extra but still has a size.
	import { api } from '$lib/api.js';
	import { tokens, usd } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import AuditTabs from '$components/AuditTabs.svelte';

	let range = $state('mtd');
	let c = $state(null);
	$effect(() => {
		const r = range;
		api.get(`/costs?range=${r}`).then((x) => (c = x)).catch(() => (c = null));
	});
	const RANGES = [
		['mtd', 'Month to date'],
		['7d', 'Last 7 days'],
		['30d', 'Last 30 days'],
		['all', 'All time']
	];
	const pct = (a, b) => (b ? Math.round((a / b) * 100) : 0);
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Audit' }, { label: 'Costs' }]} />
	<AuditTabs />
	<div class="pg-body">
		<div class="wrap">
			<div class="ranges">
				{#each RANGES as [k, l]}<button class:on={range === k} onclick={() => (range = k)}>{l}</button>{/each}
			</div>
			{#if c}
				<div class="cards">
					<div class="card"><div class="v">{usd(c.total.costUsd)}</div><div class="l">Metered spend</div><div class="s">{c.total.apiRuns} API-key runs</div></div>
					<div class="card"><div class="v">{usd(c.total.notionalCostUsd)}</div><div class="l">Subscription usage</div><div class="s">at list price · {c.total.subscriptionRuns} runs · not billed</div></div>
					<div class="card"><div class="v">{tokens(c.total.tokens)}</div><div class="l">Tokens</div><div class="s">{pct(c.total.cacheRead, c.total.tokens)}% cache reads</div></div>
					<div class="card"><div class="v">{c.total.runs}</div><div class="l">Runs</div><div class="s">work and chat</div></div>
				</div>

				<h3>By agent</h3>
				<table>
					<thead><tr><th>Agent</th><th>Runs</th><th>Tokens</th><th>Cache read</th><th>Output</th><th>Metered</th><th>Subscription</th></tr></thead>
					<tbody>
						{#each c.byAgent as l (l.id + l.name)}
							<tr>
								<td>{#if l.id}<a href="/agents/{l.id}">{l.name}</a>{:else}{l.name} <span class="faint">(no agent)</span>{/if}</td>
								<td>{l.usage.runs}</td><td>{tokens(l.usage.tokens)}</td><td>{tokens(l.usage.cacheRead)}</td>
								<td>{tokens(l.usage.output)}</td><td>{usd(l.usage.costUsd)}</td><td>{usd(l.usage.notionalCostUsd)}</td>
							</tr>
						{:else}
							<tr><td colspan="7" class="faint">No runs in this period.</td></tr>
						{/each}
					</tbody>
				</table>

				<h3>Tickets that used the most</h3>
				<table>
					<thead><tr><th>Ticket</th><th>Runs</th><th>Tokens</th><th>Metered</th><th>Subscription</th></tr></thead>
					<tbody>
						{#each c.byTicket as l (l.id)}
							<tr>
								<td><a href="/issue/{l.key}"><span class="mono">{l.key}</span> {l.name}</a></td>
								<td>{l.usage.runs}</td><td>{tokens(l.usage.tokens)}</td><td>{usd(l.usage.costUsd)}</td><td>{usd(l.usage.notionalCostUsd)}</td>
							</tr>
						{:else}
							<tr><td colspan="5" class="faint">No runs in this period.</td></tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	</div>
</div>

<style>
	.wrap {
		padding: 16px clamp(16px, 3vw, 28px) 40px;
		max-width: 1080px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.ranges {
		display: flex;
		gap: 2px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2px;
		width: fit-content;
	}
	.ranges button {
		background: none;
		border: none;
		color: var(--text-dim);
		font-size: 12.5px;
		padding: 4px 10px;
		border-radius: 6px;
	}
	.ranges button.on {
		background: var(--bg-hover);
		color: var(--text);
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 10px;
	}
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 14px 16px;
	}
	.v {
		font-size: 24px;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
	.l {
		font-size: 13px;
	}
	.s,
	.faint {
		font-size: 12px;
		color: var(--text-faint);
	}
	h3 {
		margin: 10px 0 0;
		font-size: 12px;
		font-weight: 500;
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	th,
	td {
		text-align: left;
		padding: 8px 12px;
		border-bottom: 1px solid var(--border);
		font-variant-numeric: tabular-nums;
	}
	th {
		font-weight: 500;
		color: var(--text-faint);
		font-size: 12px;
	}
	tr:last-child td {
		border-bottom: none;
	}
	td a:hover {
		text-decoration: underline;
	}
	.mono {
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--text-faint);
	}
</style>
