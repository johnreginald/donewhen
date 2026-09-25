<script>
	// Agents, as Paperclip lists them: who they are, what they run on, whether
	// they can run right now, and when they last did.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { agents, priorityAgents, PRIORITIES } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import { HARNESSES, harnessName, harnessOn } from '$lib/harness.js';
	import PageHeader from '$components/PageHeader.svelte';
	import ModelPicker from '$components/ModelPicker.svelte';
	import EpicMenu from '$components/EpicMenu.svelte';
	import { Plus, Bot, X } from '@lucide/svelte';

	let hosts = $state([]);
	let lastRun = $state({}); // agent id -> latest run
	let tab = $state('all');

	// Who works a task nobody chose an agent for, by its priority — e.g. the
	// strongest model for Urgent, a cheap one for Low.
	const PRIORITY_ORDER = [1, 2, 3, 4, 0];
	async function setDefault(priority, agentId) {
		try {
			const list = await api.put('/priority-agents', { priority, agentId });
			priorityAgents.set(Object.fromEntries((list || []).map((p) => [p.priority, p.agentId])));
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	async function load() {
		hosts = (await api.hosts().catch(() => [])) || [];
		const runs = (await api.runs({ limit: 200 }).catch(() => [])) || [];
		const latest = {};
		for (const r of runs) if (r.agentId && !latest[r.agentId]) latest[r.agentId] = r;
		lastRun = latest;
	}
	onMount(load);
	onMount(() =>
		onLive((ev) => {
			if (ev?.type === 'host.updated' || ev?.type === 'run.finished' || ev?.type === 'run.started') load();
		})
	);

	const ready = (a) => harnessOn(hosts, a.harness)?.status?.ready && harnessOn(hosts, a.harness)?.online;
	const shown = $derived(
		$agents.filter((a) =>
			tab === 'all' ? true : tab === 'active' ? a.status === 'active' : tab === 'paused' ? a.status === 'paused' : !ready(a)
		)
	);
	const counts = $derived({
		all: $agents.length,
		active: $agents.filter((a) => a.status === 'active').length,
		paused: $agents.filter((a) => a.status === 'paused').length,
		setup: $agents.filter((a) => !ready(a)).length
	});

	// New agent
	let creating = $state(false);
	let draft = $state({ name: '', role: '', harness: 'claude', model: '' });
	async function create() {
		if (!draft.name.trim()) return;
		try {
			const a = await api.createAgent({ ...draft, name: draft.name.trim() });
			agents.update((l) => [...l.filter((x) => x.id !== a.id), a]);
			creating = false;
			draft = { name: '', role: '', harness: 'claude', model: '' };
			goto(`/agents/${a.slug}?tab=runtime`);
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Agents' }]}>
		<button class="btn primary sm" onclick={() => (creating = true)}><Plus size={14} strokeWidth={2.4} />New agent</button>
	</PageHeader>
	<div class="pg-body">
		<div class="tabs">
			{#each [['all', 'All'], ['active', 'Active'], ['paused', 'Paused'], ['setup', 'Needs setup']] as [k, l]}
				<button class="tab" class:on={tab === k} onclick={() => (tab = k)}>
					{l}{#if counts[k]}<span class="n">{counts[k]}</span>{/if}
				</button>
			{/each}
		</div>

		{#if !$agents.length}
			<div class="empty">
				<Bot size={22} strokeWidth={1.6} />
				<p>No agents yet. An agent is a harness — Claude Code, Codex or OpenCode — with a model and instructions, that you can hand tickets to.</p>
				<button class="btn primary" onclick={() => (creating = true)}><Plus size={14} strokeWidth={2.4} />New agent</button>
			</div>
		{:else}
			<div class="list">
				{#each shown as a (a.id)}
					{@const h = harnessOn(hosts, a.harness)}
					{@const r = lastRun[a.id]}
					<a class="row" href="/agents/{a.slug}">
						<span class="av"><Bot size={15} strokeWidth={2} /></span>
						<span class="who">
							<span class="nm">{a.name}</span>
							<span class="role">{a.role || 'No role'}</span>
						</span>
						<span class="dot" class:ok={h?.status?.ready && h?.online} class:paused={a.status === 'paused'}
							title={a.status === 'paused' ? 'Paused' : h?.status?.ready && h?.online ? 'Ready on ' + h.host.name : 'Needs setup'}></span>
						<span class="harness">{harnessName(a.harness)}</span>
						<span class="model mono">{a.model || 'default'}</span>
						<span class="last">{r ? rel(r.startedAt) : 'never ran'}</span>
					</a>
				{/each}
			</div>

			<section class="defaults">
				<h3>Default agent by priority</h3>
				<p class="hint">A task with no agent chosen goes to the agent for its priority — for Run, Start task and the conversation. Choosing an agent on the task overrides it.</p>
				<div class="dgrid">
					{#each PRIORITY_ORDER as pr (pr)}
						<span class="pl">{PRIORITIES.find((p) => p.value === pr)?.label}</span>
						<EpicMenu value={$priorityAgents[pr] || ''} options={$agents} icon={Bot} none="No default"
							onchange={(v) => setDefault(pr, v)} />
					{/each}
				</div>
			</section>
		{/if}
	</div>
</div>

{#if creating}
	<div class="backdrop" role="presentation" onclick={() => (creating = false)}></div>
	<div class="modal" role="dialog" aria-modal="true" aria-label="New agent">
		<div class="mh"><span>New agent</span><button class="x" onclick={() => (creating = false)} aria-label="Close"><X size={15} /></button></div>
		<div class="mb">
			<input class="big" bind:value={draft.name} placeholder="Agent name, e.g. Backend Engineer" />
			<input class="field" bind:value={draft.role} placeholder="Role (optional), e.g. Works tickets in api-server" />
			<div class="lbl">Harness</div>
			<div class="harnesses">
				{#each HARNESSES as hz (hz.id)}
					{@const h = harnessOn(hosts, hz.id)}
					<button class="hz" class:on={draft.harness === hz.id} onclick={() => ((draft.harness = hz.id), (draft.model = ''))}>
						<span class="hzn">{hz.name}</span>
						<span class="hzp">{harnessOn(hosts, hz.id)?.status?.auth || hz.plan}</span>
						<span class="hzs" class:ok={h?.status?.ready && h?.online}>
							{h?.status?.ready && h?.online ? 'Ready on ' + h.host.name : 'Not connected'}
						</span>
					</button>
				{/each}
			</div>
			<div class="lbl">Model</div>
			{#key draft.harness}
				<ModelPicker
					bind:value={draft.model}
					harness={draft.harness}
					models={harnessOn(hosts, draft.harness)?.status?.models || []}
				/>
			{/key}
		</div>
		<div class="mf">
			<button class="btn ghost" onclick={() => (creating = false)}>Cancel</button>
			<button class="btn primary" onclick={create} disabled={!draft.name.trim() || (draft.harness === 'opencode' && !draft.model)}>Create agent</button>
		</div>
	</div>
{/if}

<style>
	.tabs {
		display: flex;
		gap: 4px;
		padding: 12px 20px 6px;
	}
	.tab {
		background: none;
		border: none;
		color: var(--text-dim);
		font-size: 13px;
		padding: 5px 10px;
		border-radius: 7px;
		display: inline-flex;
		gap: 6px;
		align-items: center;
	}
	.tab:hover {
		color: var(--text);
	}
	.tab.on {
		background: var(--bg-hover);
		color: var(--text);
	}
	.n {
		font-size: 11px;
		color: var(--text-faint);
	}
	.list {
		padding: 4px 12px 16px;
	}
	.defaults {
		margin: 8px 20px 32px;
		padding: 14px 16px;
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--bg-elev);
		max-width: 620px;
	}
	.defaults h3 {
		margin: 0 0 4px;
		font-size: 13px;
		font-weight: 600;
	}
	.defaults .hint {
		margin: 0 0 12px;
		font-size: 12.5px;
		color: var(--text-faint);
		line-height: 1.5;
	}
	.dgrid {
		display: grid;
		grid-template-columns: 110px minmax(0, 1fr);
		align-items: center;
		gap: 6px 12px;
	}
	.pl {
		font-size: 13px;
		color: var(--text-dim);
	}
	.row {
		display: grid;
		grid-template-columns: 28px minmax(0, 1.6fr) 10px minmax(0, 1fr) minmax(0, 1fr) 90px;
		align-items: center;
		gap: 12px;
		padding: 9px 10px;
		border-radius: 8px;
		font-size: 13.5px;
	}
	.row:hover {
		background: var(--bg-hover);
	}
	.av {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		display: grid;
		place-items: center;
		color: var(--text-dim);
	}
	.who {
		display: flex;
		flex-direction: column;
		min-width: 0;
		line-height: 1.3;
	}
	.nm {
		font-weight: 500;
	}
	.role {
		font-size: 12px;
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #d03b3b;
	}
	.dot.ok {
		background: #0ca30c;
	}
	.dot.paused {
		background: var(--text-faint);
	}
	.harness {
		color: var(--text-dim);
	}
	.mono {
		font-family: var(--mono);
		font-size: 12px;
	}
	.model {
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.last {
		color: var(--text-faint);
		font-size: 12px;
		text-align: right;
	}
	.empty {
		margin: 40px auto;
		max-width: 420px;
		text-align: center;
		color: var(--text-dim);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 10px;
		font-size: 13.5px;
	}
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		z-index: 70;
	}
	.modal {
		position: fixed;
		top: 10vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(560px, 94vw);
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 14px;
		box-shadow: var(--shadow);
		z-index: 71;
		display: flex;
		flex-direction: column;
	}
	.mh {
		display: flex;
		align-items: center;
		padding: 14px 18px 6px;
		font-weight: 600;
	}
	.x {
		margin-left: auto;
		background: none;
		border: none;
		color: var(--text-faint);
		display: inline-flex;
		padding: 4px;
		border-radius: 6px;
	}
	.mb {
		padding: 8px 18px 14px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.big {
		background: transparent;
		border: none;
		outline: none;
		font-size: 18px;
		font-weight: 500;
		padding: 4px 0;
	}
	.field {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 8px 10px;
		font-size: 13px;
		outline: none;
	}
	.field:focus {
		border-color: var(--border-strong);
	}
	.lbl {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-faint);
		margin-top: 4px;
	}
	.harnesses {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 8px;
	}
	.hz {
		display: flex;
		flex-direction: column;
		gap: 2px;
		text-align: left;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 9px;
		padding: 10px;
		color: var(--text);
	}
	.hz.on {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.hzn {
		font-weight: 500;
		font-size: 13.5px;
	}
	.hzp {
		font-size: 11.5px;
		color: var(--text-dim);
	}
	.hzs {
		font-size: 11px;
		color: #fca5a5;
		margin-top: 4px;
	}
	.hzs.ok {
		color: #86efac;
	}
	.mf {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		padding: 12px 18px 14px;
		border-top: 1px solid var(--border);
	}
	@media (max-width: 720px) {
		.row {
			grid-template-columns: 28px minmax(0, 1fr) 10px 90px;
		}
		.harness,
		.model {
			display: none;
		}
		.harnesses {
			grid-template-columns: 1fr;
		}
	}
</style>
