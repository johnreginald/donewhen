<script>
	// Monthly caps per agent, as Paperclip's budgets: at 80% it is noted, at
	// 100% the agent pauses until the cap is raised or it is resumed.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';
	import { tokens, usd } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import AuditTabs from '$components/AuditTabs.svelte';

	let lines = $state([]);
	let drafts = $state({}); // agent id -> { tokens, usd }

	async function load() {
		lines = (await api.get('/budgets').catch(() => [])) || [];
		drafts = Object.fromEntries(
			lines.map((l) => [l.agent.id, { tokens: l.agent.budgetTokens || '', usd: l.agent.budgetUsd || '' }])
		);
	}
	onMount(load);

	async function save(l) {
		const d = drafts[l.agent.id];
		try {
			await api.updateAgent(l.agent.id, { budgetTokens: Number(d.tokens) || 0, budgetUsd: Number(d.usd) || 0 });
			showToast(`${l.agent.name}'s budget saved`);
			load();
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function resume(l) {
		try {
			await api.updateAgent(l.agent.id, { status: 'active' });
			load();
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	const tone = (s) => (s >= 1 ? 'over' : s >= 0.8 ? 'warn' : 'ok');
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Audit' }, { label: 'Budgets' }]} />
	<AuditTabs />
	<div class="pg-body">
		<div class="wrap">
			<p class="lead">
				A cap counts this calendar month (UTC). Tokens count every run, subscription included; dollars count
				metered (API-key) spend. At 80% the ticket notes it; at 100% the agent pauses and its queued work waits.
			</p>
			{#each lines as l (l.agent.id)}
				{@const d = drafts[l.agent.id]}
				<div class="row">
					<div class="who">
						<a href="/agents/{l.agent.slug}" class="nm">{l.agent.name}</a>
						{#if l.agent.status === 'paused'}<span class="paused">paused</span>{/if}
					</div>
					<div class="bar {tone(l.share)}" title="{Math.round(l.share * 100)}% of the tighter cap">
						<span style:width="{Math.min(100, l.share * 100)}%"></span>
						<i class="mark" style:left="80%"></i>
					</div>
					<div class="use">
						{tokens(l.usage.tokens)} tokens · {usd(l.usage.costUsd)} metered
						{#if l.agent.budgetTokens || l.agent.budgetUsd}<span class="faint">· {Math.round(l.share * 100)}%</span>{/if}
					</div>
					<label>Tokens / month<input type="number" min="0" bind:value={d.tokens} placeholder="no cap" /></label>
					<label>$ / month<input type="number" min="0" step="0.5" bind:value={d.usd} placeholder="no cap" /></label>
					<button class="btn sm" onclick={() => save(l)}>Save</button>
					{#if l.agent.status === 'paused'}<button class="btn sm" onclick={() => resume(l)}>Resume</button>{/if}
				</div>
			{:else}
				<div class="faint">No agents yet.</div>
			{/each}
		</div>
	</div>
</div>

<style>
	.wrap {
		padding: 16px clamp(16px, 3vw, 28px) 40px;
		max-width: 1080px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.lead {
		margin: 0 0 6px;
		font-size: 13px;
		color: var(--text-faint);
		max-width: 720px;
	}
	.row {
		display: grid;
		grid-template-columns: 160px 1fr 200px 130px 110px auto auto;
		align-items: center;
		gap: 12px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 10px 14px;
		font-size: 13px;
	}
	.nm {
		font-weight: 500;
	}
	.paused {
		margin-left: 6px;
		font-size: 11px;
		color: var(--st-review);
	}
	.bar {
		position: relative;
		height: 8px;
		border-radius: 999px;
		background: var(--bg-elev2);
		overflow: visible;
	}
	.bar span {
		display: block;
		height: 100%;
		border-radius: 999px;
		background: #0ca30c;
	}
	.bar.warn span {
		background: #c98500;
	}
	.bar.over span {
		background: #d03b3b;
	}
	.mark {
		position: absolute;
		top: -3px;
		width: 1px;
		height: 14px;
		background: var(--text-faint);
	}
	.use {
		font-size: 12px;
		color: var(--text-dim);
	}
	.faint {
		color: var(--text-faint);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 11px;
		color: var(--text-faint);
	}
	input {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 5px 8px;
		font-size: 12.5px;
		color: var(--text);
		width: 100%;
	}
	@media (max-width: 900px) {
		.row {
			grid-template-columns: 1fr 1fr;
		}
		.bar {
			grid-column: 1 / -1;
		}
	}
</style>
