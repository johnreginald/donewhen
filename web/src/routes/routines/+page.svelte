<script>
	// Routines: tickets that make themselves on a schedule — Paperclip's
	// routines. A routine can also queue its agent's run; that is the one
	// thing here that starts on its own, and only when you turn it on.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { agents, projects } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import PageHeader from '$components/PageHeader.svelte';
	import EpicMenu from '$components/EpicMenu.svelte';
	import { Plus, Repeat, Play, Bot, X, Trash2 } from '@lucide/svelte';

	let list = $state([]);
	let editing = $state(null); // the form being edited, or null
	let saving = $state(false);

	const PRESETS = [
		{ cron: '0 * * * *', label: 'Every hour' },
		{ cron: '0 9 * * *', label: 'Every day at 09:00' },
		{ cron: '0 9 * * 1-5', label: 'Weekdays at 09:00' },
		{ cron: '0 9 * * 1', label: 'Mondays at 09:00' },
		{ cron: '0 9 1 * *', label: 'The 1st of each month at 09:00' }
	];
	const describe = (r) => (PRESETS.find((p) => p.cron === r.schedule)?.label || `cron ${r.schedule}`) + ` · ${r.timezone}`;
	const agentName = (id) => $agents.find((a) => a.id === id)?.name;

	async function load() {
		list = (await api.get('/routines').catch(() => [])) || [];
	}
	onMount(load);

	function blank() {
		return {
			name: '',
			title: '',
			descriptionMd: '',
			schedule: '0 9 * * 1',
			timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
			agentId: '',
			projectId: '',
			criteria: [],
			autoRun: false,
			enabled: true
		};
	}
	function edit(r) {
		editing = r
			? { ...r, agentId: r.agentId || '', projectId: r.projectId || '', criteria: structuredClone(r.criteria || []) }
			: blank();
	}
	const custom = $derived(editing && !PRESETS.some((p) => p.cron === editing.schedule));

	function addCrit() {
		editing.criteria = [...editing.criteria, { text: '', kind: 'deterministic', check: { cmd: '', expect_exit: 0 } }];
	}
	function setKind(c, kind) {
		c.kind = kind;
		c.check = kind === 'deterministic' ? { cmd: '', expect_exit: 0 } : kind === 'policy' ? { policy: 'paths_within', args: [] } : undefined;
	}

	async function save() {
		saving = true;
		try {
			const body = { ...editing, agentId: editing.agentId || null, projectId: editing.projectId || null };
			delete body.id;
			const saved = editing.id ? await api.patch(`/routines/${editing.id}`, body) : await api.post('/routines', body);
			list = [...list.filter((x) => x.id !== saved.id), saved].sort((a, b) => a.name.localeCompare(b.name));
			editing = null;
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			saving = false;
		}
	}
	async function toggle(r) {
		try {
			const saved = await api.patch(`/routines/${r.id}`, { enabled: !r.enabled });
			list = list.map((x) => (x.id === saved.id ? saved : x));
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function runNow(r) {
		try {
			const is = await api.post(`/routines/${r.id}/run`, {});
			showToast(`${is.key} made`);
			load();
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function remove(r) {
		try {
			await api.del(`/routines/${r.id}`);
			list = list.filter((x) => x.id !== r.id);
			editing = null;
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Routines' }]}>
		<button class="btn primary sm" onclick={() => edit(null)}><Plus size={14} strokeWidth={2.4} />New routine</button>
	</PageHeader>
	<div class="pg-body">
		{#if !list.length}
			<div class="empty">
				<Repeat size={22} strokeWidth={1.6} />
				<p>A routine makes a ticket on a schedule — a weekly dependency bump, a daily triage pass — for an agent, with its own done-when.</p>
				<button class="btn primary" onclick={() => edit(null)}><Plus size={14} strokeWidth={2.4} />New routine</button>
			</div>
		{:else}
			<div class="list">
				{#each list as r (r.id)}
					<div class="row" class:off={!r.enabled}>
						<button class="main" onclick={() => edit(r)}>
							<span class="nm">{r.name}</span>
							<span class="sub">{describe(r)}{#if r.agentId} · {agentName(r.agentId)}{/if}</span>
						</button>
						{#if r.autoRun}<span class="badge">runs itself</span>{/if}
						<span class="next">{r.enabled ? (r.nextRunAt ? 'next ' + rel(r.nextRunAt).replace(' ago', '') : '—') : 'off'}</span>
						{#if r.lastIssueKey}<a class="last" href="/issue/{r.lastIssueKey}">{r.lastIssueKey}</a>{:else}<span class="last faint">—</span>{/if}
						<button class="icon" title="Run now" onclick={() => runNow(r)}><Play size={14} /></button>
						<button class="switch" class:on={r.enabled} onclick={() => toggle(r)} aria-label={r.enabled ? 'Turn off' : 'Turn on'}><span></span></button>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

{#if editing}
	<div class="backdrop" role="presentation" onclick={() => (editing = null)}></div>
	<div class="modal" role="dialog" aria-modal="true" aria-label="Routine">
		<div class="mh"><span>{editing.id ? 'Edit routine' : 'New routine'}</span><button class="x" onclick={() => (editing = null)} aria-label="Close"><X size={15} /></button></div>
		<div class="mb">
			<input class="big" bind:value={editing.name} placeholder="Routine name, e.g. Weekly dependency bump" />
			<div class="grid">
				<label>Schedule
					<select value={custom ? 'custom' : editing.schedule} onchange={(e) => (editing.schedule = e.currentTarget.value === 'custom' ? '*/30 * * * *' : e.currentTarget.value)}>
						{#each PRESETS as p}<option value={p.cron}>{p.label}</option>{/each}
						<option value="custom">Custom (cron)</option>
					</select>
				</label>
				{#if custom}<label>Cron<input class="mono" bind:value={editing.schedule} placeholder="m h dom mon dow" /></label>{/if}
				<label>Timezone<input bind:value={editing.timezone} /></label>
			</div>
			<div class="chips">
				<span class="faint">For</span>
				<EpicMenu value={editing.agentId} options={$agents} icon={Bot} none="No agent" onchange={(v) => (editing.agentId = v)} />
				<span class="faint">in</span>
				<EpicMenu value={editing.projectId} options={$projects} onchange={(v) => (editing.projectId = v)} />
			</div>
			<div class="lbl">The ticket it makes</div>
			<input class="field" bind:value={editing.title} placeholder="Title — {'{date}'} becomes the day, e.g. Bump dependencies {'{date}'}" />
			<textarea class="field" rows="3" bind:value={editing.descriptionMd} placeholder="What the ticket asks for (markdown)"></textarea>
			<div class="lbl">Done when</div>
			{#each editing.criteria as c, i}
				<div class="crit">
					<select value={c.kind} onchange={(e) => setKind(c, e.currentTarget.value)}>
						<option value="deterministic">command</option>
						<option value="policy">policy</option>
						<option value="manual">manual</option>
					</select>
					<input class="field grow" bind:value={c.text} placeholder="What must be true" />
					{#if c.kind === 'deterministic'}
						<input class="field grow mono" bind:value={c.check.cmd} placeholder="command that must exit 0, e.g. go test ./..." />
					{:else if c.kind === 'policy'}
						<input class="field grow mono" value={(c.check.args || []).join(', ')}
							oninput={(e) => (c.check.args = e.currentTarget.value.split(',').map((s) => s.trim()).filter(Boolean))}
							placeholder="paths it may change, e.g. go.mod, go.sum" />
					{/if}
					<button class="icon" onclick={() => (editing.criteria = editing.criteria.filter((_, j) => j !== i))} aria-label="Remove"><X size={13} /></button>
				</div>
			{/each}
			<button class="btn sm add" onclick={addCrit}><Plus size={13} />Add a check</button>
			<label class="check"><input type="checkbox" bind:checked={editing.autoRun} />
				Also run it — queue the agent as soon as the ticket is made. Its commands above run on your Mac.</label>
			<label class="check"><input type="checkbox" bind:checked={editing.enabled} />On</label>
		</div>
		<div class="mf">
			{#if editing.id}<button class="btn danger sm" onclick={() => remove(editing)}><Trash2 size={13} />Delete</button>{/if}
			<span class="spacer"></span>
			<button class="btn ghost" onclick={() => (editing = null)}>Cancel</button>
			<button class="btn primary" onclick={save} disabled={saving || !editing.name.trim() || !editing.title.trim()}>{saving ? 'Saving…' : 'Save routine'}</button>
		</div>
	</div>
{/if}

<style>
	.list {
		padding: 12px;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 8px 10px;
		border-radius: 8px;
		font-size: 13.5px;
	}
	.row:hover {
		background: var(--bg-hover);
	}
	.row.off .nm {
		color: var(--text-faint);
	}
	.main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		background: none;
		border: none;
		color: var(--text);
		text-align: left;
		padding: 0;
		line-height: 1.35;
	}
	.nm {
		font-weight: 500;
	}
	.sub {
		font-size: 12px;
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.badge {
		font-size: 11px;
		color: var(--st-progress);
		border: 1px solid color-mix(in srgb, var(--st-progress) 40%, transparent);
		border-radius: 999px;
		padding: 0 8px;
	}
	.next,
	.last {
		font-size: 12px;
		color: var(--text-faint);
		min-width: 70px;
		text-align: right;
	}
	.last {
		font-family: var(--mono);
		color: var(--accent2);
	}
	.faint {
		color: var(--text-faint);
	}
	.icon {
		background: none;
		border: none;
		color: var(--text-faint);
		display: inline-flex;
		padding: 4px;
		border-radius: 6px;
	}
	.icon:hover {
		color: var(--text);
		background: var(--bg-hover);
	}
	.switch {
		width: 30px;
		height: 17px;
		border-radius: 999px;
		border: none;
		background: var(--border-strong);
		position: relative;
		padding: 0;
		flex-shrink: 0;
	}
	.switch span {
		position: absolute;
		top: 2px;
		left: 2px;
		width: 13px;
		height: 13px;
		border-radius: 50%;
		background: #fff;
		transition: transform 0.15s;
	}
	.switch.on {
		background: var(--accent);
	}
	.switch.on span {
		transform: translateX(13px);
	}
	.empty {
		margin: 40px auto;
		max-width: 440px;
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
		top: 6vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(680px, 94vw);
		max-height: 88vh;
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
	}
	.mb {
		padding: 8px 18px 14px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		overflow-y: auto;
	}
	.big {
		background: transparent;
		border: none;
		outline: none;
		font-size: 18px;
		font-weight: 500;
		padding: 4px 0;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
		gap: 10px;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 5px;
		font-size: 12px;
		color: var(--text-faint);
	}
	label.check {
		flex-direction: row;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		color: var(--text-dim);
	}
	.chips {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		flex-wrap: wrap;
	}
	.lbl {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-faint);
		margin-top: 4px;
	}
	.field,
	select,
	input:not([type='checkbox']) {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 7px 9px;
		font-size: 13px;
		color: var(--text);
		outline: none;
		font-family: inherit;
	}
	.mono {
		font-family: var(--mono);
		font-size: 12px;
	}
	.big {
		border: none !important;
		background: transparent !important;
	}
	.crit {
		display: flex;
		gap: 6px;
		align-items: center;
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.add {
		width: fit-content;
	}
	.mf {
		display: flex;
		gap: 8px;
		align-items: center;
		padding: 12px 18px 14px;
		border-top: 1px solid var(--border);
	}
	.spacer {
		flex: 1;
	}
</style>
