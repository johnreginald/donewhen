<script>
	import { onMount } from 'svelte';
	// One agent: Paperclip's agent page, trimmed to what Raenil uses — who it
	// is, what it runs on (and whether that works right now), its
	// instructions, its runs, and what they cost.
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { agents, issues, states } from '$lib/store.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel, tokens, usd, duration } from '$lib/format.js';
	import { HARNESSES, harnessName, harnessOn } from '$lib/harness.js';
	import PageHeader from '$components/PageHeader.svelte';
	import RunBlock from '$components/RunBlock.svelte';
	import StateIcon from '$components/StateIcon.svelte';
	import Markdown from '$components/Markdown.svelte';
	import ModelPicker from '$components/ModelPicker.svelte';
	import { Bot, Pause, Play, FlaskConical, Check, X, LoaderCircle, Copy } from '@lucide/svelte';

	let agent = $state(null);
	let form = $state(null); // editable copy on the runtime / instructions tabs
	let hosts = $state([]);
	let runs = $state([]);
	let test = $state(null); // latest environment test job
	let saving = $state(false);
	let previewInstr = $state(false);

	// Rules every agent in the workspace gets, edited here too.
	let sharedText = $state('');
	let sharedSaved = $state('');
	let savingShared = $state(false);
	onMount(async () => {
		const r = await api.get('/allowed-tools').catch(() => null);
		sharedText = sharedSaved = (r?.allowedTools || []).join('\n');
	});
	async function saveShared() {
		savingShared = true;
		try {
			const r = await api.put('/allowed-tools', {
				allowedTools: sharedText.split('\n').map((x) => x.trim()).filter(Boolean)
			});
			sharedText = sharedSaved = (r?.allowedTools || []).join('\n');
			showToast('Saved for every agent');
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			savingShared = false;
		}
	}

	// A starting point for an agent's instructions: how this workspace works,
	// so a new agent does not have to learn it from its first refusal.
	const DEFAULT_INSTRUCTIONS = `# How we work

## Before building
- A ticket in Aligning is not ready. Read it, its done-when and the code, then ask what you need to know, propose how to split it, or say it is ready to run.
- Don't guess at scope. If the ticket is unclear, ask.

## While building
- Work only inside the ticket's worktree, on what the ticket asks. Leave unrelated code alone.
- The done-when list is the gate. Make every item true; don't change the list to fit the work.
- Run the project's tests and build before you finish.
- Don't commit, push or open pull requests: Raenil commits your work and a person reviews it.

## When you are stuck
- If you cannot finish without a decision or an answer, stop and ask. Say what you tried and what you need.

## Handing back
- Say what you changed, how you checked it, and anything left for the reviewer.
`;

	const TABS = [
		{ id: 'overview', label: 'Overview', group: 'Agent' },
		{ id: 'instructions', label: 'Instructions', group: 'Agent' },
		{ id: 'runtime', label: 'Harness / Runtime', group: 'Runtime' },
		{ id: 'runs', label: 'Runs', group: 'Audit' },
		{ id: 'costs', label: 'Costs', group: 'Audit' }
	];
	const tab = $derived($page.url.searchParams.get('tab') || 'overview');
	const setTab = (t) => goto(`?tab=${t}`, { replaceState: true, noScroll: true, keepFocus: true });

	$effect(() => {
		const slug = $page.params.slug;
		if (slug) load(slug);
	});

	async function load(slug) {
		try {
			agent = await api.agent(slug);
			form = toForm(agent);
			[hosts, runs] = await Promise.all([api.hosts().catch(() => []), api.agentRuns(agent.id).catch(() => [])]);
			const tests = await api.jobs({ agent: agent.id, kind: 'test_env', limit: 1 }).catch(() => []);
			test = tests?.[0] || null;
		} catch (e) {
			showToast(e.status === 404 ? 'No such agent' : e.message, 'error');
			goto('/agents');
		}
	}
	const toForm = (a) => ({
		name: a.name,
		role: a.role,
		harness: a.harness,
		model: a.model,
		effort: a.effort,
		maxTurns: a.maxTurns,
		heartbeatMinutes: a.heartbeatMinutes || 0,
		instructionsMd: a.instructionsMd,
		allowedTools: (a.allowedTools || []).join('\n')
	});
	const dirty = $derived(agent && form && JSON.stringify(form) !== JSON.stringify(toForm(agent)));

	async function save() {
		saving = true;
		try {
			agent = await api.updateAgent(agent.id, {
				...form,
				maxTurns: Number(form.maxTurns) || 0,
				heartbeatMinutes: Number(form.heartbeatMinutes) || 0,
				allowedTools: form.allowedTools
					.split('\n')
					.map((s) => s.trim())
					.filter(Boolean)
			});
			form = toForm(agent);
			showToast('Saved');
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			saving = false;
		}
	}
	async function setStatus(status) {
		try {
			agent = await api.updateAgent(agent.id, { status });
			form = toForm(agent);
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function runTest() {
		try {
			test = await api.testAgent(agent.id);
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	// Live: the test job moving, this agent's runs, hosts reporting in.
	onMount(() =>
		onLive((ev) => {
			if (!ev || !agent) return;
			if (ev.job && ev.job.agentId === agent.id && ev.job.kind === 'test_env') test = ev.job;
			if (ev.run && ev.run.agentId === agent.id) {
				const i = runs.findIndex((r) => r.id === ev.run.id);
				runs = i >= 0 ? runs.map((r) => (r.id === ev.run.id ? ev.run : r)) : [ev.run, ...runs];
			}
			if (ev.type === 'host.updated') api.hosts().then((h) => (hosts = h || []));
			if (ev.agent && ev.agent.id === agent.id && !dirty) {
				agent = ev.agent;
				form = toForm(ev.agent);
			}
		})
	);

	const conn = $derived(agent ? harnessOn(hosts, form?.harness || agent.harness) : null);
	const hz = $derived(HARNESSES.find((h) => h.id === (form?.harness || agent?.harness)));
	const assigned = $derived(agent ? $issues.filter((i) => i.agentId === agent.id) : []);
	const stOf = (id) => $states.find((s) => s.id === id);
	const cost = $derived(
		runs.reduce(
			(t, r) => ({
				runs: t.runs + 1,
				tokens: t.tokens + (r.tokens?.total || 0),
				cacheRead: t.cacheRead + (r.tokens?.cacheRead || 0),
				api: t.api + (r.costUsd || 0),
				notional: t.notional + (r.notionalCostUsd || 0),
				sub: t.sub + (r.billing === 'subscription' ? 1 : 0)
			}),
			{ runs: 0, tokens: 0, cacheRead: 0, api: 0, notional: 0, sub: 0 }
		)
	);
	function copy(text) {
		navigator.clipboard?.writeText(text).then(() => showToast('Copied'));
	}
</script>

{#if agent}
	<div class="pg">
		<PageHeader crumbs={[{ label: 'Agents', href: '/agents' }, { label: agent.name, upper: false }]} />
		<div class="pg-body">
			<div class="head">
				<span class="av"><Bot size={20} strokeWidth={1.8} /></span>
				<div class="hn">
					<h1>{agent.name}</h1>
					<div class="hs">
						<span>{harnessName(agent.harness)}</span>·<span>{agent.role || 'No role'}</span>
						{#if agent.status === 'paused'}<span class="paused">Paused</span>{/if}
					</div>
				</div>
				<div class="ha">
					<button class="btn sm" onclick={runTest}><FlaskConical size={14} />Test environment</button>
					{#if agent.status === 'paused'}
						<button class="btn sm" onclick={() => setStatus('active')}><Play size={14} />Resume</button>
					{:else}
						<button class="btn sm" onclick={() => setStatus('paused')}><Pause size={14} />Pause</button>
					{/if}
				</div>
			</div>

			<div class="split">
				<nav class="subnav">
					{#each ['Agent', 'Runtime', 'Audit'] as g}
						<div class="sg">{g}</div>
						{#each TABS.filter((t) => t.group === g) as t (t.id)}
							<button class="si" class:on={tab === t.id} onclick={() => setTab(t.id)}>{t.label}</button>
						{/each}
					{/each}
				</nav>

				<div class="panel">
					{#if tab === 'overview'}
						<h2>Overview</h2>
						<div class="cards">
							<section class="card">
								<h3>Identity</h3>
								<dl>
									<dt>Role</dt><dd>{agent.role || '—'}</dd>
									<dt>Status</dt><dd>{agent.status}</dd>
									<dt>Handle</dt><dd class="mono">{agent.slug}</dd>
								</dl>
							</section>
							<section class="card">
								<h3>Harness / Runtime</h3>
								<dl>
									<dt>Harness</dt><dd>{harnessName(agent.harness)}</dd>
									<dt>Model</dt><dd class="mono">{agent.model || 'default'}</dd>
									<dt>Connection</dt>
									<dd>
										{#if conn?.status?.ready && conn.online}<span class="ok">Ready on {conn.host.name}</span>
										{:else}<span class="bad">Not connected</span>{/if}
									</dd>
								</dl>
							</section>
						</div>
						<h3 class="sec">Latest run</h3>
						{#if runs[0]}<RunBlock run={runs[0]} />{:else}<div class="faint">No runs yet.</div>{/if}
						<h3 class="sec">Tasks for this agent <span class="faint">{assigned.length}</span></h3>
						{#each assigned as is (is.id)}
							{@const st = stOf(is.stateId)}
							<a class="task" href="/issue/{is.key}">
								<StateIcon category={st?.category} color={st?.color} />
								<span class="tt">{is.title}</span>
								<span class="tk">{is.key}</span>
							</a>
						{:else}
							<div class="faint">No tickets are assigned to this agent.</div>
						{/each}
					{:else if tab === 'instructions'}
						<h2>Instructions</h2>
						<p class="hint">What this agent should know about how you work, given to it at the start of each fresh session. It also reads the repository's own <code>CLAUDE.md</code> / <code>AGENTS.md</code>, and Raenil tells it the ticket, its done-when and what the turn is for — no need to repeat those here.</p>
						<div class="instr-bar">
							<div class="seg">
								<button class:on={!previewInstr} onclick={() => (previewInstr = false)}>Write</button>
								<button class:on={previewInstr} onclick={() => (previewInstr = true)}>Preview</button>
							</div>
							{#if !form.instructionsMd?.trim()}
								<button class="btn sm" onclick={() => (form.instructionsMd = DEFAULT_INSTRUCTIONS)}>Start from the default</button>
							{/if}
						</div>
						{#if previewInstr}
							<div class="preview">{#if form.instructionsMd}<Markdown source={form.instructionsMd} />{:else}<span class="faint">Nothing yet.</span>{/if}</div>
						{:else}
							<textarea class="instr" bind:value={form.instructionsMd} placeholder="e.g. You work in api-server. Run go test ./... before finishing. Never change migrations."></textarea>
						{/if}
					{:else if tab === 'runtime'}
						<h2>Harness / Runtime</h2>
						<section class="block">
							<h3>Agent identity</h3>
							<div class="grid2">
								<label>Name<input bind:value={form.name} /></label>
								<label>Role<input bind:value={form.role} placeholder="e.g. Backend engineer" /></label>
							</div>
						</section>
						<section class="block">
							<h3>Adapter</h3>
							<label>Harness
								<select bind:value={form.harness}>
									{#each HARNESSES as h (h.id)}<option value={h.id}>{h.name}</option>{/each}
								</select>
							</label>
							<div class="conn" class:ready={conn?.status?.ready && conn?.online}>
								<div class="ct">
									<span class="cn">{conn?.status?.auth || hz?.plan}</span>
									{#if conn?.status?.ready && conn.online}
										<span class="ok"><Check size={13} strokeWidth={2.4} /> Ready on {conn.host.name}</span>
									{:else if conn}
										<span class="bad">{conn.online ? 'Not ready' : 'Host offline'} on {conn.host.name}</span>
									{:else}
										<span class="bad">No host has reported</span>
									{/if}
								</div>
								{#if conn?.status?.detail}<div class="cd">{conn.status.detail}</div>{/if}
								{#if !(conn?.status?.ready && conn?.online)}
									<div class="fix">
										<code>{conn?.status?.fix || hz?.connect}</code>
										<button class="icon" onclick={() => copy(conn?.status?.fix || hz?.connect)} aria-label="Copy"><Copy size={13} /></button>
									</div>
									{#if !conn}<div class="cd">Start the runner host on your Mac: <code>orchestrator host</code></div>{/if}
								{/if}
								<div class="cd">
									{#if hz?.id === 'opencode'}Uses the keys OpenCode keeps on the host (opencode auth login); Raenil never sees them.
									{:else}Uses your {hz?.plan} login on the host — no API key.{/if}
								</div>
							</div>
							<div class="grid2">
								<div class="lab">Model
									{#key form.harness}
										<ModelPicker bind:value={form.model} harness={form.harness} models={conn?.status?.models || []} />
									{/key}
								</div>
								{#if form.harness === 'claude'}
									<label>Thinking effort
										<select bind:value={form.effort}>
											{#each ['', 'low', 'medium', 'high', 'xhigh', 'max'] as e}<option value={e}>{e || 'Auto'}</option>{/each}
										</select>
									</label>
								{/if}
								<label>Max turns per run<input type="number" min="0" bind:value={form.maxTurns} placeholder="0 = no limit" /></label>
								<label>Heartbeat
									<select bind:value={form.heartbeatMinutes}>
										{#each [[0, 'Off'], [5, 'Every 5 minutes'], [15, 'Every 15 minutes'], [30, 'Every 30 minutes'], [60, 'Every hour']] as [v, l]}
											<option value={v}>{l}</option>
										{/each}
									</select>
								</label>
							</div>
						</section>
						<section class="block">
							<h3>Allowed commands</h3>
							{#if form.harness === 'claude'}
								<p class="hint">Claude runs in auto mode: ordinary commands run without asking, and risky ones are blocked. Commands listed here skip that check.</p>
								{#if conn?.status?.alwaysAllowed?.length}
									<div class="defaults">
										{#each conn.status.alwaysAllowed as c (c)}<code>{c.replace(/^Bash\((.*)\)$/, '$1')}</code>{/each}
									</div>
								{/if}
								<div class="lab">Also allow for every agent in this workspace
									<textarea class="rules mono short" bind:value={sharedText} placeholder={'Bash(curl -sI *)'}></textarea>
								</div>
								{#if sharedText !== sharedSaved}
									<div class="shared-save">
										<button class="btn ghost sm" onclick={() => (sharedText = sharedSaved)}>Discard</button>
										<button class="btn primary sm" onclick={saveShared} disabled={savingShared}>{savingShared ? 'Saving…' : 'Save for every agent'}</button>
									</div>
								{/if}
								<div class="lab">Also allow for this agent
									<textarea class="rules mono" bind:value={form.allowedTools} placeholder={'Bash(docker compose ps*)\nBash(psql -c *)'}></textarea>
								</div>
								<p class="hint">One rule per line, e.g. <code>Bash(cargo test *)</code>. Bare <code>Bash</code> is not allowed: it would let the agent write anywhere.</p>
							{:else if form.harness === 'codex'}
								<p class="hint">Codex runs any command inside its own sandbox without asking: it can write only in the ticket's worktree and has no network. Nothing to list here.</p>
							{:else}
								<p class="hint">OpenCode approves every tool call it makes. It works in a throwaway worktree, and the ticket's checks decide whether the work passes.</p>
							{/if}
						</section>
						<section class="block">
							<h3>Test your agent</h3>
							<p class="hint">Asks the harness a question with this model, on the host, end to end.</p>
							<div class="test">
								<button class="btn sm" onclick={runTest} disabled={test && (test.status === 'queued' || test.status === 'claimed')}>
									<FlaskConical size={14} />Run test
								</button>
								{#if test}
									<span class="tst {test.status}">
										{#if test.status === 'queued'}<LoaderCircle size={13} class="spin" /> Waiting for a host…
										{:else if test.status === 'claimed'}<LoaderCircle size={13} class="spin" /> Testing on {test.host}…
										{:else if test.status === 'succeeded'}<Check size={13} strokeWidth={2.4} /> Answered on {test.host} with {test.result?.model} in {((test.result?.durationMs || 0) / 1000).toFixed(1)}s
										{:else}<X size={13} strokeWidth={2.4} /> {test.error || 'Failed'}{/if}
									</span>
									<span class="faint">{rel(test.finishedAt || test.createdAt)}</span>
								{/if}
							</div>
						</section>
					{:else if tab === 'runs'}
						<h2>Runs <span class="faint">{runs.length}</span></h2>
						{#each runs as r (r.id)}
							<RunBlock run={r} />
						{:else}
							<div class="faint">No runs yet.</div>
						{/each}
					{:else if tab === 'costs'}
						<h2>Costs</h2>
						<div class="cards">
							<section class="card stat"><div class="v">{usd(cost.api)}</div><div class="l">Metered spend</div><div class="s">API-key runs</div></section>
							<section class="card stat"><div class="v">{usd(cost.notional)}</div><div class="l">Subscription usage</div><div class="s">at list price · {cost.sub} runs</div></section>
							<section class="card stat"><div class="v">{tokens(cost.tokens)}</div><div class="l">Tokens</div><div class="s">{tokens(cost.cacheRead)} of them cache reads</div></section>
							<section class="card stat"><div class="v">{cost.runs}</div><div class="l">Runs</div><div class="s">all time</div></section>
						</div>
					{/if}

					{#if (tab === 'runtime' || tab === 'instructions') && dirty}
						<div class="savebar">
							<span class="faint">Unsaved changes</span>
							<button class="btn ghost sm" onclick={() => (form = toForm(agent))}>Discard</button>
							<button class="btn primary sm" onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save changes'}</button>
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	.head {
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 20px 24px 16px;
		flex-wrap: wrap;
	}
	.av {
		width: 42px;
		height: 42px;
		border-radius: 12px;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		display: grid;
		place-items: center;
		color: var(--text-dim);
	}
	h1 {
		margin: 0;
		font-size: 20px;
		font-weight: 600;
		letter-spacing: -0.01em;
	}
	.hs {
		display: flex;
		gap: 6px;
		font-size: 13px;
		color: var(--text-faint);
	}
	.paused {
		color: var(--st-review);
	}
	.ha {
		margin-left: auto;
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.split {
		display: flex;
		gap: 8px;
		padding: 0 16px 40px;
	}
	.subnav {
		width: 190px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.sg {
		font-size: 11px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-faint);
		padding: 12px 10px 4px;
	}
	.si {
		background: none;
		border: none;
		text-align: left;
		color: var(--text-dim);
		font-size: 13.5px;
		padding: 6px 10px;
		border-radius: 7px;
	}
	.si:hover {
		color: var(--text);
		background: var(--bg-hover);
	}
	.si.on {
		color: var(--text);
		background: var(--bg-hover);
	}
	.panel {
		flex: 1;
		min-width: 0;
		max-width: 820px;
		padding: 6px 8px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	h2 {
		margin: 6px 0 4px;
		font-size: 16px;
		font-weight: 600;
	}
	h3 {
		margin: 0 0 8px;
		font-size: 12px;
		font-weight: 500;
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	h3.sec {
		margin-top: 8px;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 10px;
	}
	.card,
	.block {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	dl {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 6px 16px;
		margin: 0;
		font-size: 13px;
	}
	dt {
		color: var(--text-faint);
	}
	dd {
		margin: 0;
		color: var(--text-dim);
		min-width: 0;
	}
	.mono {
		font-family: var(--mono);
		font-size: 12px;
	}
	.ok {
		color: #86efac;
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.bad {
		color: #fca5a5;
	}
	.faint {
		color: var(--text-faint);
		font-size: 13px;
		font-weight: 400;
	}
	.task {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 7px 8px;
		border-radius: 7px;
		font-size: 13px;
	}
	.task:hover {
		background: var(--bg-hover);
	}
	.tt {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tk {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--text-faint);
	}
	.hint {
		margin: 0;
		font-size: 12.5px;
		color: var(--text-faint);
		line-height: 1.5;
	}
	code {
		font-family: var(--mono);
		font-size: 11.5px;
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 4px;
		padding: 0 4px;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 5px;
		font-size: 12px;
		color: var(--text-faint);
	}
	input,
	select,
	textarea {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 7px 9px;
		font-size: 13px;
		color: var(--text);
		outline: none;
		font-family: inherit;
	}
	textarea.mono {
		font-family: var(--mono);
	}
	input:focus,
	select:focus,
	textarea:focus {
		border-color: var(--border-strong);
	}
	.rules.short {
		min-height: 64px;
	}
	.shared-save {
		display: flex;
		justify-content: flex-end;
		gap: 6px;
		margin: -4px 0 10px;
	}
	.instr-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
	}
	.defaults {
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
		margin-bottom: 12px;
	}
	.defaults code {
		font-size: 11.5px;
		font-family: var(--mono);
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 5px;
		padding: 1px 6px;
		color: var(--text-dim);
	}
	.lab {
		display: flex;
		flex-direction: column;
		gap: 5px;
		font-size: 12px;
		color: var(--text-faint);
	}
	.grid2 {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 10px;
	}
	.conn {
		border: 1px solid color-mix(in srgb, #d03b3b 35%, var(--border));
		border-radius: 9px;
		padding: 10px 12px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		background: var(--bg);
	}
	.conn.ready {
		border-color: color-mix(in srgb, #0ca30c 35%, var(--border));
	}
	.ct {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		font-size: 13px;
	}
	.cn {
		font-weight: 500;
	}
	.cd {
		font-size: 12px;
		color: var(--text-faint);
	}
	.fix {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.icon {
		background: none;
		border: none;
		color: var(--text-faint);
		display: inline-flex;
		padding: 3px;
		border-radius: 5px;
	}
	.icon:hover {
		color: var(--text);
		background: var(--bg-hover);
	}
	.rules,
	.instr {
		min-height: 110px;
		resize: vertical;
		line-height: 1.55;
	}
	.instr {
		min-height: 320px;
	}
	.preview {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 14px 16px;
		min-height: 200px;
	}
	.seg {
		display: flex;
		gap: 2px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2px;
		width: fit-content;
	}
	.seg button {
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 3px 10px;
		border-radius: 6px;
		font-size: 12.5px;
	}
	.seg button.on {
		background: var(--bg-hover);
		color: var(--text);
	}
	.test {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
		font-size: 13px;
	}
	.tst {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		color: var(--text-dim);
	}
	.tst.succeeded {
		color: #86efac;
	}
	.tst.failed {
		color: #fca5a5;
	}
	.tst :global(.spin) {
		animation: spin 1s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.stat .v {
		font-size: 24px;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
	.stat .l {
		font-size: 13px;
	}
	.stat .s {
		font-size: 12px;
		color: var(--text-faint);
	}
	.stat {
		gap: 2px;
	}
	.savebar {
		position: sticky;
		bottom: 12px;
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 8px;
		background: var(--bg-elev2);
		border: 1px solid var(--border-strong);
		border-radius: 10px;
		padding: 8px 10px;
		box-shadow: var(--shadow);
	}
	.savebar .faint {
		margin-right: auto;
	}
	@media (max-width: 720px) {
		.split {
			flex-direction: column;
		}
		.subnav {
			width: auto;
			flex-direction: row;
			overflow-x: auto;
		}
		.sg {
			display: none;
		}
		.head {
			padding: 16px;
		}
	}
</style>
