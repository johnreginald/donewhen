<script>
	import { issues, states, projects, initiatives } from '$lib/store.js';
	import { openIssue } from '$lib/ui.js';
	import PriorityIcon from '$components/PriorityIcon.svelte';
	import LabelPill from '$components/LabelPill.svelte';
	import StateIcon from '$components/StateIcon.svelte';

	const stOf = (id) => $states.find((s) => s.id === id);

	function loadCollapsed() {
		try {
			return new Set(JSON.parse(localStorage.getItem('raenil.list.collapsed') || '[]'));
		} catch {
			return new Set();
		}
	}
	let collapsed = $state(loadCollapsed());
	function toggle(id) {
		const n = new Set(collapsed);
		n.has(id) ? n.delete(id) : n.add(id);
		collapsed = n;
		try {
			localStorage.setItem('raenil.list.collapsed', JSON.stringify([...n]));
		} catch {
			/* ignore */
		}
	}

	// Group issues by Epic (project), then Epics by Project (initiative).
	const groups = $derived(build($issues, $projects, $initiatives, $states));
	function build(iss, projs, inis, sts) {
		const projById = new Map(projs.map((p) => [p.id, p]));
		const iniById = new Map(inis.map((i) => [i.id, i]));
		const stPos = (i) => sts.find((s) => s.id === i.stateId)?.position ?? 99;
		const sortIss = (a, b) => stPos(a) - stPos(b) || a.position - b.position;
		const last = (name, tag) => (name === tag ? 1 : 0);

		const byEpic = new Map();
		for (const i of iss) {
			const k = i.projectId || '__none__';
			if (!byEpic.has(k)) byEpic.set(k, []);
			byEpic.get(k).push(i);
		}
		const byProj = new Map();
		for (const [epicId, list] of byEpic) {
			const epic = epicId === '__none__' ? null : projById.get(epicId);
			const iniId = epic?.initiativeId || '__none__';
			if (!byProj.has(iniId)) byProj.set(iniId, []);
			byProj.get(iniId).push({ id: epicId, name: epic?.name || 'No epic', issues: [...list].sort(sortIss) });
		}
		const res = [];
		for (const [iniId, epics] of byProj) {
			const ini = iniId === '__none__' ? null : iniById.get(iniId);
			res.push({
				id: iniId,
				name: ini?.name || 'No project',
				count: epics.reduce((n, e) => n + e.issues.length, 0),
				epics: epics.sort((a, b) => last(a.name, 'No epic') - last(b.name, 'No epic') || a.name.localeCompare(b.name))
			});
		}
		return res.sort((a, b) => last(a.name, 'No project') - last(b.name, 'No project') || a.name.localeCompare(b.name));
	}
</script>

<div class="list">
	{#each groups as g (g.id)}
		<div class="proj-group">
			<div class="proj-head">
				<span class="pn">{g.name}</span>
				<span class="c">{g.count}</span>
			</div>
			{#each g.epics as e (e.id)}
				<div class="epic-group">
					<button class="epic-head" onclick={() => toggle(g.id + e.id)}>
						<span class="chev" class:open={!collapsed.has(g.id + e.id)}>▸</span>
						<span class="en">{e.name}</span>
						<span class="c">{e.issues.length}</span>
					</button>
					{#if !collapsed.has(g.id + e.id)}
						{#each e.issues as i (i.id)}
							<button class="row" onclick={() => openIssue(i.key)}>
								<StateIcon category={stOf(i.stateId)?.category} color={stOf(i.stateId)?.color} />
								<span class="rkey">{i.key}</span>
								{#if i.childCount > 0}<span class="epic-badge">↳{i.childCount}</span>{/if}
								<span class="rtitle">{i.title}</span>
								<span class="rlabels">{#each i.labels as l (l.id)}<LabelPill label={l} />{/each}</span>
								<PriorityIcon priority={i.priority} />
							</button>
						{/each}
					{/if}
				</div>
			{/each}
		</div>
	{/each}
	{#if !groups.length}
		<div class="empty faint">No issues yet. Press ⌘K to create one.</div>
	{/if}
</div>

<style>
	.list {
		height: 100%;
		overflow-y: auto;
		padding: 0 0 40px;
	}
	.proj-group {
		margin-bottom: 6px;
	}
	.proj-head {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 38px;
		padding: 0 20px;
		font-size: 13px;
		font-weight: 600;
		color: var(--text);
		position: sticky;
		top: 0;
		background: var(--bg);
		border-bottom: 1px solid var(--border);
		z-index: 4;
	}
	.epic-head {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		height: 32px;
		background: var(--bg);
		border: none;
		text-align: left;
		padding: 0 20px;
		color: var(--text-dim);
		font-size: 12.5px;
		position: sticky;
		top: 38px;
		z-index: 3;
	}
	.epic-head:hover {
		color: var(--text);
	}
	.chev {
		font-size: 9px;
		color: var(--text-faint);
		transition: transform 0.15s ease;
	}
	.chev.open {
		transform: rotate(90deg);
	}
	.en {
		font-weight: 500;
	}
	.c {
		color: var(--text-faint);
		font-size: 11.5px;
		font-family: var(--mono);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		background: none;
		border: none;
		border-top: 1px solid var(--border);
		text-align: left;
		padding: 8px 20px 8px 40px;
		color: var(--text);
		font-size: 13.5px;
	}
	.row:hover {
		background: var(--bg-elev);
	}
	.rkey {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--text-faint);
		flex: none;
		width: 62px;
	}
	.epic-badge {
		font-size: 10.5px;
		font-family: var(--mono);
		color: var(--accent2);
		background: color-mix(in srgb, var(--accent2) 15%, transparent);
		padding: 1px 6px;
		border-radius: 10px;
		flex: none;
	}
	.rtitle {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rlabels {
		display: flex;
		gap: 4px;
		flex: none;
		max-width: 40%;
		overflow: hidden;
	}
	.empty {
		padding: 40px;
		text-align: center;
	}
</style>
