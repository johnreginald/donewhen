<script>
	// By epic — Project (initiative) → Epic (project) → Issue, collapsible,
	// with a done/total progress bar and the epic's repo URL on its header.
	// (Moved out of /list, which is now the state-grouped row view.)
	import { visibleIssues, states, projects, initiatives, activeInitiative } from '$lib/store.js';
	import PageHeader from '$components/PageHeader.svelte';
	import IssuesToolbar from '$components/IssuesToolbar.svelte';
	import { openIssue } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import PriorityIcon from '$components/PriorityIcon.svelte';
	import LabelPill from '$components/LabelPill.svelte';
	import StateIcon from '$components/StateIcon.svelte';

	const stOf = (id) => $states.find((s) => s.id === id);

	const LS_KEY = 'raenil.byEpic.collapsed';
	function loadCollapsed() {
		try {
			return new Set(JSON.parse(localStorage.getItem(LS_KEY) || '[]'));
		} catch {
			return new Set();
		}
	}
	// Keyed by epic id ALONE (the old /list page keyed this g.id + e.id, which
	// broke the moment a project moved between initiatives).
	let collapsed = $state(loadCollapsed());
	function toggle(epicId) {
		const n = new Set(collapsed);
		n.has(epicId) ? n.delete(epicId) : n.add(epicId);
		collapsed = n;
		try {
			localStorage.setItem(LS_KEY, JSON.stringify([...n]));
		} catch {
			/* storage blocked: not remembered */
		}
	}

	function shortRepo(url) {
		if (!url) return '';
		try {
			const u = new URL(url);
			return (u.host + u.pathname).replace(/\/$/, '');
		} catch {
			return url;
		}
	}

	const groups = $derived(build($visibleIssues, $projects, $initiatives, $states, $activeInitiative));
	function build(iss, projs, inis, sts, activeIni) {
		const projById = new Map(projs.map((p) => [p.id, p]));
		const iniById = new Map(inis.map((i) => [i.id, i]));
		const stPos = (i) => sts.find((s) => s.id === i.stateId)?.position ?? 99;
		const isDone = (i) => {
			const cat = sts.find((s) => s.id === i.stateId)?.category;
			return cat === 'completed' || cat === 'canceled';
		};
		const sortIss = (a, b) => stPos(a) - stPos(b) || a.position - b.position;
		const last = (name, tag) => (name === tag ? 1 : 0);

		// Seed every epic (scoped to the active Project) so newly-created /
		// empty epics still show as headers, then drop issues into them.
		const shownEpics = activeIni ? projs.filter((p) => p.initiativeId === activeIni) : projs;
		const byEpic = new Map();
		for (const p of shownEpics) byEpic.set(p.id, []);
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
			const sorted = [...list].sort(sortIss);
			const done = sorted.filter(isDone).length;
			byProj.get(iniId).push({
				id: epicId,
				name: epic?.name || 'No epic',
				repoUrl: epic?.repoUrl || null,
				issues: sorted,
				done,
				total: sorted.length,
				pct: sorted.length ? Math.round((done / sorted.length) * 100) : 0
			});
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
	const hasAnyEpic = $derived(groups.some((g) => g.epics.length));
	const loading = $derived($states.length === 0);
</script>

<div class="page">
	<PageHeader crumbs={[{ label: 'Tasks', href: '/board' }, { label: 'By epic' }]} />
	<IssuesToolbar />

	{#if loading}
		<div class="tree">
			{#each Array(3) as _, i (i)}
				<div class="epic skel-epic">
					<div class="ehd">
						<span class="skel" style="width:14px;height:14px;border-radius:3px"></span>
						<span class="skel" style="width:{120 + i * 30}px;height:13px"></span>
						<span class="sp"></span>
						<span class="skel" style="width:36px;height:11px"></span>
					</div>
					<div class="eprog"><i style="width:{30 + i * 20}%"></i></div>
				</div>
			{/each}
		</div>
	{:else if !hasAnyEpic}
		<div class="empty">
			<div class="ic">▦</div>
			<p class="etitle">No epics yet</p>
			<p class="esub">Epics show up here once issues are grouped under one. Press ⌘K to create one.</p>
		</div>
	{:else}
		<div class="tree">
			{#each groups as g (g.id)}
				<div class="proj-group">
					<div class="proj-head">
						<span class="pn">{g.name}</span>
						<span class="c">{g.count}</span>
					</div>
					{#each g.epics as e (e.id)}
						<div class="epic">
							<button class="ehd" onclick={() => toggle(e.id)} aria-expanded={!collapsed.has(e.id)}>
								<span class="chv" class:open={!collapsed.has(e.id)}>›</span>
								<span class="nm">{e.name}</span>
								{#if e.repoUrl}
									<span class="repo"><span class="rg"></span>{shortRepo(e.repoUrl)}</span>
								{/if}
								<span class="sp"></span>
								<span class="frac">{e.done}/{e.total}</span>
							</button>
							{#if e.total}
								<div class="eprog"><i style="width:{e.pct}%"></i></div>
							{/if}
							{#if !collapsed.has(e.id)}
								{#if !e.total}
									<div class="eempty">No issues in this epic yet.</div>
								{:else}
									{#each e.issues as i (i.id)}
										{@const st = stOf(i.stateId)}
										<button class="erow" onclick={() => openIssue(i.key)}>
											<StateIcon category={st?.category} color={st?.color} size={13} />
											<span class="k">{i.key}</span>
											<span class="t">{i.title}</span>
											<span class="lbls">{#each i.labels as l (l.id)}<LabelPill label={l} />{/each}</span>
											{#if i.priority}<PriorityIcon priority={i.priority} />{/if}
											<span class="d">{rel(i.updatedAt)}</span>
										</button>
									{/each}
								{/if}
							{/if}
						</div>
					{:else}
						<div class="eempty">No epics in this project yet.</div>
					{/each}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page {
		height: 100%;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.tree {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 12px 0 30px;
	}
	.proj-group {
		margin-bottom: 4px;
	}
	.proj-head {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 4px 20px 10px;
		font: 600 11px var(--mono);
		color: var(--ink-3);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.proj-head .c {
		font-family: var(--mono);
	}
	.epic {
		margin: 0 16px 12px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
	}
	.ehd {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		padding: 10px 14px;
		background: none;
		border: none;
		text-align: left;
		color: var(--ink);
	}
	.ehd:hover {
		background: var(--hover);
	}
	.ehd .nm {
		font-weight: 600;
		font-size: 13.5px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ehd .repo {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--ink-3);
		display: flex;
		align-items: center;
		gap: 5px;
		flex: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 220px;
	}
	.ehd .rg {
		width: 8px;
		height: 8px;
		border-radius: 2px;
		border: 1.4px solid currentColor;
		flex: none;
	}
	.ehd .sp {
		flex: 1;
	}
	.ehd .frac {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--ink-3);
		flex: none;
	}
	.eprog {
		height: 4px;
		background: var(--sunken);
		margin: 0 14px 10px;
		border-radius: 2px;
		overflow: hidden;
	}
	.eprog i {
		display: block;
		height: 100%;
		background: var(--st-done);
	}
	.erow {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		height: 34px;
		padding: 0 14px 0 32px;
		border: none;
		border-top: 1px solid var(--line);
		background: none;
		color: var(--ink);
		text-align: left;
		font-size: 12.5px;
	}
	.erow:hover {
		background: var(--hover);
	}
	.erow:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.erow .k {
		font-family: var(--mono);
		color: var(--ink-3);
		font-size: 11px;
		width: 54px;
		flex: none;
	}
	.erow .t {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.erow .lbls {
		display: flex;
		gap: 4px;
		flex: none;
		max-width: 30%;
		overflow: hidden;
	}
	.erow .d {
		font-size: 11px;
		color: var(--ink-3);
		flex: none;
		width: 44px;
		text-align: right;
	}
	.eempty {
		padding: 14px 32px;
		color: var(--ink-3);
		font-size: 12.5px;
		border-top: 1px solid var(--line);
	}

	/* loading */
	.skel {
		background: linear-gradient(90deg, var(--sunken) 25%, var(--hover) 37%, var(--sunken) 63%);
		background-size: 400% 100%;
		animation: skshim 1.6s ease infinite;
		border-radius: 4px;
		display: inline-block;
	}
	@keyframes skshim {
		0% {
			background-position: 100% 0;
		}
		100% {
			background-position: 0 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.skel {
			animation: none;
		}
	}
	.skel-epic .ehd {
		cursor: default;
	}

	/* empty */
	.empty {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 9px;
		padding: 24px;
		text-align: center;
	}
	.ic {
		width: 38px;
		height: 38px;
		border-radius: 50%;
		border: 1.5px dashed var(--line-strong);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--ink-3);
		font-size: 14px;
	}
	.etitle {
		margin: 0;
		font-size: 13.5px;
		font-weight: 600;
		color: var(--ink);
	}
	.esub {
		margin: 0;
		font-size: 12px;
		color: var(--ink-3);
		max-width: 280px;
		line-height: 1.45;
	}

	.chv {
		display: inline-block;
		font-size: 9px;
		color: var(--ink-3);
		transition: transform var(--dur) var(--ease);
		flex: none;
	}
	.chv.open {
		transform: rotate(90deg);
	}

	/* mobile: stack repo under the name, drop labels, no horizontal scroll at 400px */
	@media (max-width: 720px) {
		.epic {
			margin: 0 12px 10px;
		}
		.ehd {
			flex-wrap: wrap;
			padding: 10px 12px;
		}
		.ehd .nm {
			flex: 1 1 calc(100% - 24px);
			max-width: calc(100% - 24px);
			font-size: 13px;
		}
		.ehd .repo {
			font-size: 10.5px;
			max-width: 60%;
		}
		.ehd .frac {
			margin-left: auto;
		}
		.eprog {
			margin: 0 12px 10px;
		}
		.erow {
			padding: 0 12px 0 14px;
			gap: 8px;
		}
		.erow .lbls {
			display: none;
		}
		.erow .k {
			width: 48px;
		}
	}
</style>
