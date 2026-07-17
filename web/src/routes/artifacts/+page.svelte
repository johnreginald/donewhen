<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { api } from '$lib/api.js';
	import { projects, initiatives, issues, labels as allLabels } from '$lib/store.js';
	import { showToast, openIssue } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';
	import LabelPill from '$components/LabelPill.svelte';
	import { Trash2, ChevronDown } from '@lucide/svelte';

	const TYPES = {
		change: { label: 'Change', icon: '⟳', color: 'var(--accent)' },
		feature: { label: 'Feature', icon: '◈', color: 'var(--st-done)' },
		decision: { label: 'Decision', icon: '◆', color: 'var(--st-progress)' },
		overview: { label: 'Overview', icon: '◇', color: 'var(--st-ready)' },
		reference: { label: 'Reference', icon: '▤', color: 'var(--text-dim)' }
	};
	const typeMeta = (t) => TYPES[t] || TYPES.reference;

	let docs = $state([]);
	let sel = $state(null);
	let query = $state('');
	let typeFilter = $state('');
	let aiOnly = $state(false);
	let attachOpen = $state(false);
	let typeOpen = $state(false);
	let filterOpen = $state(false);
	let labelPickerOpen = $state(false);
	let contentEl = $state(null);

	let missing = $state([]);
	let showGaps = $state(false);
	onMount(async () => {
		await load();
		missing = (await api.missingDocs().catch(() => [])) || [];
		const wanted = $page.url.searchParams.get('doc');
		const d = (wanted && docs.find((x) => x.id === wanted)) || docs[0];
		if (d) open(d);
	});
	async function load() {
		docs = (await api.documents()) || [];
	}

	const filtered = $derived(
		docs.filter((d) => {
			if (query.trim() && !d.title.toLowerCase().includes(query.trim().toLowerCase())) return false;
			if (typeFilter && d.type !== typeFilter) return false;
			if (aiOnly && d.author !== 'ai') return false;
			return true;
		})
	);

	async function open(d) {
		sel = await api.document(d.id);
		attachOpen = typeOpen = labelPickerOpen = confirmDel = false;
	}

	const toc = $derived(extractToc(sel?.bodyMd || ''));
	function extractToc(md) {
		const out = [];
		for (const line of md.split('\n')) {
			const m = line.match(/^(#{2,3})\s+(.+)/);
			if (m) out.push({ level: m[1].length, text: m[2].replace(/[*`]/g, '').trim() });
		}
		return out;
	}
	function scrollToHeading(text) {
		if (!contentEl) return;
		for (const h of contentEl.querySelectorAll('h1,h2,h3')) {
			if (h.textContent.trim() === text) {
				h.scrollIntoView({ behavior: 'smooth', block: 'start' });
				return;
			}
		}
	}

	async function persist(extra) {
		if (!sel) return;
		sel = await api.saveDocument({ id: sel.id, title: sel.title, bodyMd: sel.bodyMd, type: sel.type, ...extra });
		await load();
	}
	async function setType(t) {
		typeOpen = false;
		await persist({ type: t });
	}
	async function setAttach(kind, id) {
		attachOpen = false;
		await persist({
			projectId: kind === 'project' ? id : '',
			initiativeId: kind === 'initiative' ? id : '',
			issueId: kind === 'issue' ? id : ''
		});
	}
	function toggleLabel(id) {
		const has = sel.labels.some((l) => l.id === id);
		const ids = has
			? sel.labels.filter((l) => l.id !== id).map((l) => l.id)
			: [...sel.labels.map((l) => l.id), id];
		persist({ labelIds: ids });
	}
	let confirmDel = $state(false);
	async function del() {
		if (!sel) return;
		if (!confirmDel) {
			confirmDel = true;
			return;
		}
		await api.deleteDocument(sel.id);
		sel = null;
		confirmDel = false;
		await load();
	}

	function attachOf(d) {
		if (d.initiativeId) return { icon: '◈', label: $initiatives.find((i) => i.id === d.initiativeId)?.name ?? 'Project', kind: 'initiative' };
		if (d.projectId) return { icon: '▢', label: $projects.find((p) => p.id === d.projectId)?.name ?? 'Epic', kind: 'project' };
		if (d.issueId) {
			const is = $issues.find((i) => i.id === d.issueId);
			return { icon: '◦', label: is ? is.key : 'Issue', kind: 'issue', issue: is };
		}
		return null;
	}
	function relTime(iso) {
		const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
		if (s < 60) return 'just now';
		if (s < 3600) return Math.floor(s / 60) + 'm ago';
		if (s < 86400) return Math.floor(s / 3600) + 'h ago';
		if (s < 604800) return Math.floor(s / 86400) + 'd ago';
		return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}
</script>

<div class="docs">
	<aside class="index">
		<div class="ihead"><span class="it">Artifacts</span><span class="isub">{docs.length}</span></div>
		{#if missing.length}
			<button class="cov" class:open={showGaps} onclick={() => (showGaps = !showGaps)}>
				<span class="cov-dot"></span>{missing.length} done {missing.length === 1 ? 'issue has' : 'issues have'} no artifact
			</button>
			{#if showGaps}
				<div class="cov-list">
					{#each missing as m (m.id)}
						<button class="cov-item" onclick={() => openIssue(m.key)}>
							<span class="mono">{m.key}</span><span class="cov-t">{m.title}</span>
						</button>
					{/each}
				</div>
			{/if}
		{/if}
		<input class="search" placeholder="Search…" bind:value={query} />
		<div class="filters">
			<div class="dd tflt">
				<button class="dd-btn" onclick={() => (filterOpen = !filterOpen)}>
					{#if typeFilter}<span style:color={typeMeta(typeFilter).color}>{typeMeta(typeFilter).icon}</span>{/if}
					<span class="dd-label">{typeFilter ? typeMeta(typeFilter).label : 'All types'}</span>
					<ChevronDown size={14} strokeWidth={2} class="dd-chev" />
				</button>
				{#if filterOpen}
					<div class="dd-bd" role="presentation" onclick={() => (filterOpen = false)}></div>
					<div class="dd-menu">
						<button class="dd-item" class:on={!typeFilter} onclick={() => { typeFilter = ''; filterOpen = false; }}>All types</button>
						{#each Object.entries(TYPES) as [k, t] (k)}
							<button class="dd-item" class:on={typeFilter === k} onclick={() => { typeFilter = k; filterOpen = false; }}>
								<span style:color={t.color}>{t.icon}</span>{t.label}
							</button>
						{/each}
					</div>
				{/if}
			</div>
			<button class="ai-toggle" class:on={aiOnly} onclick={() => (aiOnly = !aiOnly)} title="AI-written only">✦ AI</button>
		</div>
		<div class="list">
			{#each filtered as d (d.id)}
				<button class="item" class:active={sel && sel.id === d.id} onclick={() => open(d)}>
					<div class="i-top">
						<span class="i-ic" style:color={typeMeta(d.type).color}>{typeMeta(d.type).icon}</span>
						<span class="i-title">{d.title || 'Untitled'}</span>
						{#if d.author === 'ai'}<span class="ai-badge">✦</span>{/if}
					</div>
					<div class="i-sub">
						{#if attachOf(d)}<span class="i-at">{attachOf(d).icon} {attachOf(d).label}</span><span class="sep">·</span>{/if}
						<span>{relTime(d.updatedAt)}</span>
						{#each d.labels ?? [] as l (l.id)}<span class="i-dot" style:background={l.color}></span>{/each}
					</div>
				</button>
			{:else}
				<div class="empty faint">{query || typeFilter || aiOnly ? 'No matches.' : 'No artifacts yet — Claude writes them as it works.'}</div>
			{/each}
		</div>
	</aside>

	<section class="view">
		{#if sel}
			{@const at = attachOf(sel)}
			<div class="topbar">
				<div class="crumb">
					<span class="ci">▤</span>Artifacts
					{#if at}<span class="sepp">›</span><button class="crumb-lnk" onclick={() => at.kind === 'issue' && at.issue && openIssue(at.issue.key)}>{at.icon} {at.label}</button>{/if}
				</div>
				<div class="grow"></div>
				<div class="typewrap">
					<button class="chip" onclick={() => (typeOpen = !typeOpen)}>
						<span style:color={typeMeta(sel.type).color}>{typeMeta(sel.type).icon}</span>{typeMeta(sel.type).label}
					</button>
					{#if typeOpen}
						<button class="bd" aria-label="x" onclick={() => (typeOpen = false)}></button>
						<div class="pop">
							{#each Object.entries(TYPES) as [k, t] (k)}
								<button class="pi" class:on={sel.type === k} onclick={() => setType(k)}><span style:color={t.color}>{t.icon}</span>{t.label}</button>
							{/each}
						</div>
					{/if}
				</div>
				<button class="btn" class:danger={confirmDel} class:ghost={!confirmDel} onclick={del} title="Delete document">
					{#if confirmDel}Confirm delete{:else}<Trash2 size={15} strokeWidth={2} />{/if}
				</button>
			</div>

			<div class="body">
				<div class="scroll" bind:this={contentEl}>
					<div class="doc">
						<h1 class="doctitle">{sel.title}</h1>
						<div class="prov">
							{#if sel.author === 'ai'}<span class="by ai">✦ Written by AI</span>{:else}<span class="by">Written by you</span>{/if}
							{#if at && at.kind === 'issue' && at.issue}<span class="sep">·</span><button class="prov-lnk" onclick={() => openIssue(at.issue.key)}>from {at.issue.key}</button>{/if}
							<span class="sep">·</span><span>{relTime(sel.updatedAt)}</span>
						</div>
						{#if sel.labels.length}
							<div class="dlabels">
								{#each sel.labels as l (l.id)}
									<button class="lbtn" onclick={() => toggleLabel(l.id)}><LabelPill label={l} /><span class="x">✕</span></button>
								{/each}
							</div>
						{/if}
						<div class="rendered"><Markdown source={sel.bodyMd} /></div>
					</div>
				</div>

				<aside class="rail">
					{#if toc.length}
						<div><div class="rh">On this page</div>
							<div class="toc">
								{#each toc as h}<button class="ta" class:sub={h.level === 3} onclick={() => scrollToHeading(h.text)}>{h.text}</button>{/each}
							</div>
						</div>
					{/if}
					<div>
						<div class="rh">Attached to</div>
						<div class="attachwrap">
							<button class="chip full" onclick={() => (attachOpen = !attachOpen)}>
								{#if at}<span class="ci">{at.icon}</span>{at.label}{:else}＋ Attach{/if}
							</button>
							{#if attachOpen}
								<button class="bd" aria-label="x" onclick={() => (attachOpen = false)}></button>
								<div class="pop wide">
									<button class="pi" onclick={() => setAttach('none', '')}>No attachment</button>
									{#if $projects.length}<div class="ps">Epics</div>{/if}
									{#each $projects as p (p.id)}<button class="pi" onclick={() => setAttach('project', p.id)}><span class="dim">▢</span>{p.name}</button>{/each}
									{#if $issues.length}<div class="ps">Issues</div>{/if}
									{#each $issues.slice(0, 40) as is (is.id)}<button class="pi" onclick={() => setAttach('issue', is.id)}><span class="mono">{is.key}</span>{is.title}</button>{/each}
								</div>
							{/if}
						</div>
					</div>
					<div>
						<div class="rh">Labels</div>
						<div class="railwrap">
							{#each sel.labels as l (l.id)}<button class="lbtn" onclick={() => toggleLabel(l.id)}><LabelPill label={l} /><span class="x">✕</span></button>{/each}
							<button class="chip" onclick={() => (labelPickerOpen = !labelPickerOpen)}>＋</button>
							{#if labelPickerOpen}
								<button class="bd" aria-label="x" onclick={() => (labelPickerOpen = false)}></button>
								<div class="pop">
									{#each $allLabels as l (l.id)}<button class="pi" class:on={sel.labels.some((x) => x.id === l.id)} onclick={() => toggleLabel(l.id)}><span class="dot" style:background={l.color}></span>{l.name}</button>{/each}
								</div>
							{/if}
						</div>
					</div>
				</aside>
			</div>
		{:else}
			<div class="ph">
				<div class="ph-ic">✦</div>
				<div class="ph-t">The AI's engineering journal</div>
				<div class="faint">Claude writes a document when it implements or changes something — what it is, how it works, a mermaid diagram, key files. Pick one on the left to read.</div>
			</div>
		{/if}
	</section>
</div>

<style>
	.docs { display: flex; height: 100%; min-height: 0; }
	.index { width: 300px; flex: none; border-right: 1px solid var(--border); display: flex; flex-direction: column; min-height: 0; }
	.ihead { display: flex; align-items: baseline; gap: 8px; padding: 15px 14px 8px; }
	.it { font: 600 15px/1 var(--disp); }
	.isub { font-size: 12px; color: var(--text-faint); }
	.search { margin: 0 12px 8px; background: var(--bg); border: 1px solid var(--border); border-radius: 8px; padding: 7px 10px; font-size: 13px; outline: none; }
	.cov { display: flex; align-items: center; gap: 7px; margin: 0 12px 8px; background: color-mix(in srgb, var(--st-progress) 12%, var(--bg)); border: 1px solid color-mix(in srgb, var(--st-progress) 30%, var(--border)); border-radius: 8px; color: var(--text); padding: 7px 10px; font-size: 12.5px; text-align: left; }
	.cov:hover { border-color: color-mix(in srgb, var(--st-progress) 50%, var(--border)); }
	.cov-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--st-progress); flex: none; }
	.cov-list { margin: 0 12px 8px; display: flex; flex-direction: column; gap: 1px; max-height: 200px; overflow-y: auto; }
	.cov-item { display: flex; align-items: center; gap: 8px; background: none; border: none; color: var(--text-dim); text-align: left; padding: 5px 8px; border-radius: 6px; font-size: 12.5px; }
	.cov-item:hover { background: var(--bg-hover); color: var(--text); }
	.cov-item .mono { font-family: var(--mono); font-size: 11.5px; color: var(--text-faint); flex: none; }
	.cov-t { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.search:focus { border-color: var(--accent); }
	.filters { display: flex; gap: 6px; padding: 0 12px 10px; border-bottom: 1px solid var(--border); }
	.tflt { flex: 1; }
	.ai-toggle { display: inline-flex; align-items: center; gap: 4px; background: var(--bg); border: 1px solid var(--border); color: var(--text-dim); border-radius: 7px; padding: 6px 10px; font-size: 12.5px; flex: none; }
	.ai-toggle:hover { border-color: var(--border-strong); color: var(--text); }
	.ai-toggle.on { background: color-mix(in srgb, var(--accent2) 16%, var(--bg)); border-color: var(--accent2); color: var(--text); }
	.list { flex: 1; overflow-y: auto; padding: 6px 8px 12px; display: flex; flex-direction: column; gap: 2px; }
	.item { text-align: left; background: none; border: none; color: var(--text); padding: 8px 10px; border-radius: 8px; display: flex; flex-direction: column; gap: 4px; }
	.item:hover { background: var(--bg-elev); }
	.item.active { background: var(--bg-hover); }
	.i-top { display: flex; align-items: center; gap: 7px; }
	.i-ic { font-size: 13px; flex: none; }
	.i-title { font-size: 13.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
	.ai-badge { color: var(--accent2); font-size: 11px; flex: none; }
	.i-sub { display: flex; align-items: center; gap: 5px; font-size: 11px; color: var(--text-faint); padding-left: 20px; }
	.i-at { color: var(--text-dim); max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.i-dot { width: 7px; height: 7px; border-radius: 50%; }
	.empty { padding: 30px 14px; text-align: center; font-size: 13px; line-height: 1.5; }

	.view { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.topbar { display: flex; align-items: center; gap: 10px; padding: 10px 18px; border-bottom: 1px solid var(--border); }
	.crumb { display: flex; align-items: center; gap: 7px; font-size: 12.5px; color: var(--dim); }
	.crumb .ci { color: var(--text-faint); }
	.crumb .sepp { color: var(--text-faint); }
	.crumb-lnk { background: none; border: none; color: var(--text); font-size: 12.5px; padding: 0; }
	.crumb-lnk:hover { color: var(--accent); }
	.grow { flex: 1; }
	.body { flex: 1; display: flex; overflow: hidden; min-height: 0; }
	.scroll { flex: 1; overflow-y: auto; min-width: 0; }
	.doc { max-width: 720px; margin: 0 auto; padding: 30px 36px 80px; }
	.doctitle { font: 700 30px/1.2 var(--disp); letter-spacing: -0.015em; margin: 0 0 10px; }
	.prov { display: flex; align-items: center; gap: 8px; font-size: 12.5px; color: var(--text-faint); margin-bottom: 14px; }
	.prov .by.ai { color: var(--accent2); }
	.prov .sep, .i-sub .sep { color: var(--text-faint); }
	.prov-lnk { background: none; border: none; color: var(--text-dim); font-size: 12.5px; padding: 0; }
	.prov-lnk:hover { color: var(--accent); }
	.dlabels { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 18px; padding-bottom: 16px; border-bottom: 1px solid var(--border); }
	.rendered { font-size: 15px; }
	.rendered :global(h1) { font: 700 24px/1.3 var(--disp); margin: 22px 0 10px; }
	.rendered :global(h2) { font: 650 19px/1.3 var(--disp); margin: 26px 0 10px; }
	.rendered :global(h3) { font: 600 16px/1.3 var(--disp); margin: 20px 0 8px; }

	.rail { width: 232px; flex: none; border-left: 1px solid var(--border); padding: 26px 16px; display: flex; flex-direction: column; gap: 22px; overflow-y: auto; }
	.rh { font: 600 10.5px/1 var(--font); letter-spacing: 0.07em; text-transform: uppercase; color: var(--text-faint); margin-bottom: 9px; }
	.toc { display: flex; flex-direction: column; gap: 1px; }
	.ta { text-align: left; background: none; border: none; border-left: 2px solid transparent; color: var(--text-dim); font-size: 13px; padding: 5px 10px; }
	.ta.sub { padding-left: 22px; font-size: 12.5px; }
	.ta:hover { color: var(--text); border-left-color: var(--accent); }

	.chip { display: inline-flex; align-items: center; gap: 6px; background: var(--bg-elev); border: 1px solid var(--border); color: var(--text-dim); border-radius: 7px; padding: 5px 10px; font-size: 12.5px; }
	.chip:hover { background: var(--bg-hover); color: var(--text); }
	.chip.full { width: 100%; justify-content: flex-start; }
	.chip .ci { color: var(--text-faint); }
	.typewrap, .attachwrap, .railwrap { position: relative; display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
	.lbtn { background: none; border: none; padding: 0; display: inline-flex; align-items: center; }
	.lbtn .x { font-size: 9px; color: var(--text-faint); margin-left: 3px; }
	.bd { position: fixed; inset: 0; z-index: 30; background: none; border: none; }
	.pop { position: absolute; top: calc(100% + 6px); left: 0; z-index: 31; min-width: 170px; background: var(--bg-elev); border: 1px solid var(--border-strong); border-radius: 10px; box-shadow: var(--shadow); padding: 5px; }
	.pop.wide { width: 250px; max-height: 320px; overflow-y: auto; }
	.ps { font: 600 10px/1 var(--font); letter-spacing: 0.05em; text-transform: uppercase; color: var(--text-faint); padding: 8px 9px 3px; }
	.pi { display: flex; align-items: center; gap: 8px; width: 100%; text-align: left; background: none; border: none; color: var(--text); padding: 7px 9px; border-radius: 6px; font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.pi:hover { background: var(--bg-hover); }
	.pi.on { color: var(--accent); }
	.pi .mono { font-family: var(--mono); color: var(--text-faint); font-size: 11px; }
	.pi .dim { color: var(--text-faint); }

	.ph { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; gap: 8px; padding: 40px; }
	.ph-ic { font-size: 30px; color: var(--accent2); }
	.ph-t { font: 600 17px/1 var(--disp); }
	.ph .faint { max-width: 420px; line-height: 1.6; font-size: 13.5px; }

	@media (max-width: 720px) {
		.index { width: 150px; }
		.rail { display: none; }
		.doc { padding: 18px 16px 60px; }
	}
</style>
