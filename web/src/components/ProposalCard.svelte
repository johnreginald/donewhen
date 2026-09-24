<script>
	// An agent's proposal to split the work into tickets, each with done-when
	// criteria a program can check. Approving creates them in Ready; running
	// them is still a Run you press.
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';
	import { loadIssues } from '$lib/store.js';
	import { ListChecks, ChevronRight, Check, X } from '@lucide/svelte';

	let { interaction, agentName = 'The agent', ondone } = $props();

	const p = $derived(interaction.payload || {});
	const tickets = $derived(p.tickets || []);
	let open = $state(new Set());
	let note = $state('');
	let busy = $state(false);

	function toggle(i) {
		const n = new Set(open);
		n.has(i) ? n.delete(i) : n.add(i);
		open = n;
	}
	async function decide(decision) {
		busy = true;
		try {
			const res = await api.post(`/interactions/${interaction.id}/respond`, { decision, note: note.trim() });
			if (decision === 'approve') await loadIssues();
			ondone?.(res);
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			busy = false;
		}
	}
	const created = $derived(interaction.response?.created || []);
	const checkText = (c) =>
		c.kind === 'deterministic' ? c.check?.cmd : c.kind === 'policy' ? `${c.check?.policy} ${(c.check?.args || []).join(', ')}` : '';
</script>

<div class="pc" class:closed={interaction.status !== 'open'}>
	<div class="ph">
		<ListChecks size={15} strokeWidth={2} />
		<span class="pt">{agentName} proposes {tickets.length} ticket{tickets.length === 1 ? '' : 's'}</span>
		{#if interaction.status !== 'open'}<span class="st {interaction.status}">{interaction.status}</span>{/if}
	</div>
	{#if p.summary}<div class="sum">{p.summary}</div>{/if}

	<ol class="list">
		{#each tickets as t, i}
			<li>
				<button class="tr" onclick={() => toggle(i)} aria-expanded={open.has(i)}>
					<span class="chev" class:on={open.has(i)}><ChevronRight size={13} /></span>
					<span class="tt">{t.title}</span>
					{#if created[i]}<a class="key" href="/issue/{created[i]}">{created[i]}</a>{/if}
					<span class="cn">{(t.criteria || []).length} checks</span>
				</button>
				<div class="td">
					{#if open.has(i) && t.description}<p>{t.description}</p>{/if}
					<!-- Always shown: approving runs these commands on your machine. -->
					{#if t.criteria?.length}
						<ul class="crit">
							{#each t.criteria as c}
								<li>
									<span class="k {c.kind}">{c.kind || 'manual'}</span>{c.text}
									{#if checkText(c)}<code>{checkText(c)}</code>{/if}
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			</li>
		{/each}
	</ol>

	{#if interaction.status === 'open'}
		<div class="warn">Approving creates these tickets. When one is run, its commands above run on your machine.</div>
		<input class="note" bind:value={note} placeholder="Note for {agentName} (optional — say what to change if you reject)" />
		<div class="pf">
			<button class="btn sm" onclick={() => decide('reject')} disabled={busy}><X size={13} />Reject</button>
			<button class="btn primary sm" onclick={() => decide('approve')} disabled={busy}>
				<Check size={13} strokeWidth={2.4} />Create {tickets.length} ticket{tickets.length === 1 ? '' : 's'}
			</button>
		</div>
	{/if}
</div>

<style>
	.pc {
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 12px;
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.pc.closed {
		border-color: var(--border);
	}
	.ph {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		color: var(--text-dim);
	}
	.pt {
		color: var(--text);
	}
	.st {
		margin-left: auto;
		font-size: 11.5px;
		padding: 1px 8px;
		border-radius: 999px;
		border: 1px solid var(--border-strong);
	}
	.st.approved {
		color: #86efac;
	}
	.st.rejected {
		color: #fca5a5;
	}
	.sum {
		font-size: 13.5px;
		color: var(--text-dim);
	}
	.list {
		margin: 0;
		padding-left: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.tr {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text);
		text-align: left;
		padding: 6px 6px;
		border-radius: 7px;
		font-size: 13.5px;
	}
	.tr:hover {
		background: var(--bg-hover);
	}
	.chev {
		display: inline-flex;
		color: var(--text-faint);
		transition: transform 0.15s;
	}
	.chev.on {
		transform: rotate(90deg);
	}
	.tt {
		flex: 1;
		min-width: 0;
	}
	.key {
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--accent2);
	}
	.cn {
		font-size: 12px;
		color: var(--text-faint);
	}
	.td {
		padding: 2px 8px 8px 28px;
		font-size: 13px;
		color: var(--text-dim);
	}
	.td p {
		margin: 0 0 6px;
	}
	.crit {
		margin: 0;
		padding-left: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.k {
		font-size: 10.5px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		border: 1px solid var(--border-strong);
		border-radius: 4px;
		padding: 0 4px;
		margin-right: 6px;
		color: var(--text-faint);
	}
	.k.deterministic,
	.k.policy {
		color: #86efac;
	}
	code {
		font-family: var(--mono);
		font-size: 11.5px;
		margin-left: 6px;
		color: var(--text-faint);
	}
	.warn {
		font-size: 12px;
		color: var(--text-faint);
	}
	.note {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 7px 10px;
		font-size: 13px;
		color: var(--text);
		outline: none;
	}
	.pf {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}
</style>
