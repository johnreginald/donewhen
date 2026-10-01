<script>
	import { onMount } from 'svelte';
	import { aiName } from '$lib/store.js';
	import PageHeader from '$components/PageHeader.svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { projects, initiatives, issues, labels as allLabels } from '$lib/store.js';
	import { showToast, openIssue, onLive } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';
	import LabelPill from '$components/LabelPill.svelte';
	import { rel } from '$lib/format.js';
	import { checkMermaid, mermaidBlocks } from '$lib/markdown.js';

	const TYPES = {
		change: { label: 'Change', icon: '⟳', color: 'var(--accent)' },
		feature: { label: 'Feature', icon: '◈', color: 'var(--st-done)' },
		decision: { label: 'Decision', icon: '◆', color: 'var(--st-progress)' },
		overview: { label: 'Overview', icon: '◇', color: 'var(--st-ready)' },
		reference: { label: 'Reference', icon: '▤', color: 'var(--ink-3)' }
	};
	const typeMeta = (t) => TYPES[t] || TYPES.reference;
	function fmtDate(iso) {
		return iso ? new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) : '';
	}

	// ---- the list ----
	let docs = $state([]);
	let loading = $state(false);
	let docsError = $state(null);
	let query = $state('');
	let typeFilter = $state('');
	let aiOnly = $state(false);
	let missing = $state([]);
	let showGaps = $state(true);

	// ---- diagram errors ----
	// Every mermaid block of every document, parsed in the browser.
	let diag = $state({ running: false, total: 0, broken: [], done: false });
	let diagSeq = 0;
	async function checkDiagrams(list) {
		const seq = ++diagSeq;
		diag = { running: true, total: 0, broken: [], done: false };
		let total = 0;
		const broken = [];
		for (const d of list) {
			const blocks = mermaidBlocks(d.bodyMd);
			for (let i = 0; i < blocks.length; i++) {
				const r = await checkMermaid(blocks[i]);
				if (seq !== diagSeq) return;
				total++;
				if (!r.ok) broken.push({ id: d.id, title: d.title, n: i + 1, of: blocks.length, error: r.error, detail: r.detail });
			}
		}
		if (seq === diagSeq) diag = { running: false, total, broken, done: true };
	}

	let loadSeq = 0;
	async function load() {
		const seq = ++loadSeq;
		loading = true;
		docsError = null;
		try {
			const list = await api.documents();
			if (seq !== loadSeq) return; // a newer load already landed — drop this stale response
			docs = list || [];
			return docs;
		} catch (e) {
			if (seq !== loadSeq) return;
			docsError = e?.message || 'Failed to load documents.';
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}
	async function refreshMissing() {
		try {
			missing = (await api.missingDocs()) || [];
		} catch {
			/* the coverage panel just stays as it was */
		}
	}

	const filtered = $derived(
		docs.filter((d) => {
			if (query.trim() && !d.title.toLowerCase().includes(query.trim().toLowerCase())) return false;
			if (typeFilter && d.type !== typeFilter) return false;
			if (aiOnly && d.author !== 'ai') return false;
			return true;
		})
	);

	// ---- the reader ----
	// selId drives which view shows and rides in the URL (?doc=) so the back
	// button and deep links work; sel is the fetched detail for that id.
	let selId = $state(null);
	let sel = $state(null);
	let selLoading = $state(false);
	let selError = $state(null);
	let attachOpen = $state(false);
	let typeOpen = $state(false);
	let labelPickerOpen = $state(false);
	let confirmDel = $state(false);
	let contentEl = $state(null);

	let openSeq = 0;
	async function selectDoc(id) {
		attachOpen = typeOpen = labelPickerOpen = confirmDel = false;
		sel = null;
		selError = null;
		selLoading = true;
		const seq = ++openSeq;
		try {
			const doc = await api.document(id);
			if (seq !== openSeq) return; // a newer open already landed
			sel = doc;
		} catch (e) {
			if (seq !== openSeq) return;
			selError = e?.message || 'Failed to load this document.';
		} finally {
			if (seq === openSeq) selLoading = false;
		}
	}
	function open(d) {
		goto(`/artifacts?doc=${d.id}`, { keepFocus: true, noScroll: true });
	}
	function back() {
		goto('/artifacts', { keepFocus: true, noScroll: true });
	}
	// Mirrors the URL into selId/sel — covers clicks, deep links, and the
	// browser's own back/forward.
	let urlDoc = undefined;
	$effect(() => {
		const id = $page.url.searchParams.get('doc') || null;
		if (id === urlDoc) return;
		urlDoc = id;
		selId = id;
		if (id) selectDoc(id);
		else {
			sel = null;
			selError = null;
		}
	});

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
		const prev = sel;
		try {
			sel = await api.saveDocument({ id: sel.id, title: sel.title, bodyMd: sel.bodyMd, type: sel.type, ...extra });
			load();
		} catch (e) {
			sel = prev;
			showToast('Could not save: ' + (e?.message || 'unknown error'), 'error');
		}
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
		const ids = has ? sel.labels.filter((l) => l.id !== id).map((l) => l.id) : [...sel.labels.map((l) => l.id), id];
		persist({ labelIds: ids });
	}
	async function del() {
		if (!sel) return;
		if (!confirmDel) {
			confirmDel = true;
			return;
		}
		try {
			await api.deleteDocument(sel.id);
			confirmDel = false;
			back();
			load();
		} catch (e) {
			confirmDel = false;
			showToast('Could not delete: ' + (e?.message || 'unknown error'), 'error');
		}
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
	function issueKeyOf(d) {
		const a = attachOf(d);
		return a && a.kind === 'issue' ? a.label : null;
	}
	function epicOf(d) {
		const a = attachOf(d);
		return a && a.kind !== 'issue' ? a : null;
	}

	onMount(() => {
		load().then((list) => list && checkDiagrams(list));
		refreshMissing();
		return onLive((ev) => {
			if (ev.type === 'document.saved' || ev.type === 'document.deleted') {
				load();
				if (ev.type === 'document.deleted' && ev.documentId === selId) back();
				else if (ev.type === 'document.saved' && ev.document?.id === selId) sel = ev.document;
			}
			if (ev.type === 'document.saved' || ev.type === 'document.deleted' || ev.type === 'issue.state_changed' || ev.type === 'issue.deleted') {
				refreshMissing();
			}
		});
	});
</script>

<div class="pg">
	<PageHeader
		crumbs={selId
			? [{ label: 'Artifacts', href: '/artifacts' }, { label: sel?.title || 'Document' }]
			: [{ label: 'Artifacts' }]}
	/>
	<div class="pg-body">
<div class="artifacts">
	{#if selId}
		<div class="rtop">
			<button class="mback" onclick={back} aria-label="Back to artifacts">←</button>
			<div class="crumb"><button class="crumb-lnk" onclick={back}>Artifacts</button>{#if sel}<span class="sep">›</span><b>{sel.title}</b>{/if}</div>
			<span class="rsp"></span>
			{#if sel}
				<div class="typewrap">
					<button class="tchip ghost" onclick={() => (typeOpen = !typeOpen)}>
						<span style:color={typeMeta(sel.type).color}>{typeMeta(sel.type).icon}</span>{typeMeta(sel.type).label}
					</button>
					{#if typeOpen}
						<button class="bd" aria-label="Close" onclick={() => (typeOpen = false)}></button>
						<div class="pop">
							{#each Object.entries(TYPES) as [k, t] (k)}
								<button class="pi" class:on={sel.type === k} onclick={() => setType(k)}><span style:color={t.color}>{t.icon}</span>{t.label}</button>
							{/each}
						</div>
					{/if}
				</div>
				<button class="delbtn" class:danger={confirmDel} onclick={del}>{confirmDel ? 'Confirm delete' : 'Delete'}</button>
			{/if}
		</div>
		<div class="rbody">
			<div class="rmain" bind:this={contentEl}>
				<div class="rwrap">
					{#if sel}
						<h1 class="rtitle">{sel.title}</h1>
						<div class="rmeta">
							{#if sel.author === 'ai'}<span class="ai">✦ Written by {$aiName}</span>{:else}<span>Written by you</span>{/if}
							{#if attachOf(sel) && attachOf(sel).kind === 'issue' && attachOf(sel).issue}
								<span class="sep">·</span>
								<span>Attached to <button class="lnk mono" onclick={() => openIssue(attachOf(sel).issue.key)}>{attachOf(sel).issue.key}</button></span>
							{/if}
							<span class="sep">·</span><span>Created {fmtDate(sel.createdAt)}</span>
							<span class="sep">·</span><span>Updated {rel(sel.updatedAt)}</span>
						</div>
						{#if sel.labels?.length}
							<div class="rlabels">
								{#each sel.labels as l (l.id)}
									<button class="lbtn" onclick={() => toggleLabel(l.id)}><LabelPill label={l} /><span class="x">✕</span></button>
								{/each}
							</div>
						{/if}
						<div class="rc"><Markdown source={sel.bodyMd} /></div>
					{:else if selError}
						<div class="rerr">
							<span>{selError}</span>
							<button class="btn ghost" onclick={() => selectDoc(selId)}>Try again</button>
						</div>
					{:else}
						<div class="rsk">
							<span class="skb" style="width:60%;height:30px;margin-bottom:14px"></span>
							<span class="skb" style="width:40%;height:13px;margin-bottom:26px"></span>
							<span class="skb" style="width:100%;height:11px;margin-bottom:8px"></span>
							<span class="skb" style="width:92%;height:11px;margin-bottom:8px"></span>
							<span class="skb" style="width:76%;height:11px"></span>
						</div>
					{/if}
				</div>
			</div>

			{#if sel}
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
							<button class="attach" onclick={() => (attachOpen = !attachOpen)}>
								{#if attachOf(sel)}<span class="mono">{attachOf(sel).icon} {attachOf(sel).label}</span>{:else}＋ Attach{/if}
							</button>
							{#if attachOpen}
								<button class="bd" aria-label="Close" onclick={() => (attachOpen = false)}></button>
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
								<button class="bd" aria-label="Close" onclick={() => (labelPickerOpen = false)}></button>
								<div class="pop">
									{#each $allLabels as l (l.id)}<button class="pi" class:on={sel.labels.some((x) => x.id === l.id)} onclick={() => toggleLabel(l.id)}><span class="dot" style:background={l.color}></span>{l.name}</button>{/each}
								</div>
							{/if}
						</div>
					</div>
				</aside>
			{/if}
		</div>
	{:else}
		<div class="main">
			<div class="mtop"><span class="ptitle">Artifacts</span><span class="msub">{docs.length} document{docs.length === 1 ? '' : 's'}</span></div>

			{#if docsError}
				<div class="err">
					<span>{docsError}</span>
					<button class="btn ghost" onclick={load}>Retry</button>
				</div>
			{/if}

			{#if missing.length}
				<div class="gaps">
					<button class="gaps-h" onclick={() => (showGaps = !showGaps)} aria-expanded={showGaps}>
						<span class="gdot">◐</span>Coverage gaps<span class="n">{missing.length}</span>
					</button>
					{#if showGaps}
						<div class="gaps-list">
							{#each missing as m (m.id)}
								<button class="gaps-item" onclick={() => openIssue(m.key)}>
									<span class="k mono">{m.key}</span><span class="gaps-t">{m.title}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			{#if diag.running || diag.done}
				<div class="dgs" class:bad={diag.broken.length}>
					<div class="dgs-h">
						<span class="dgs-t">Diagram errors{#if diag.done && diag.broken.length}<span class="n">{diag.broken.length}</span>{/if}</span>
						<button class="btn ghost" disabled={diag.running} onclick={() => checkDiagrams(docs)}>
							{diag.running ? 'Checking…' : 'Re-check'}
						</button>
					</div>
					{#if diag.done && !diag.broken.length}
						<div class="dgs-ok">All {diag.total} diagram{diag.total === 1 ? '' : 's'} render</div>
					{:else if diag.broken.length}
						<div class="gaps-list">
							{#each diag.broken as b (b.id + ':' + b.n)}
								<a class="dgs-item" href={`/artifacts?doc=${b.id}`}>
									<span class="gaps-t">{b.title}</span>
									<span class="dgs-n mono">diagram {b.n} of {b.of}</span>
									<span class="dgs-e" title={b.detail}>{b.error}</span>
								</a>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			<div class="toolsrow">
				<input class="input inp" placeholder="Search documents…" bind:value={query} />
				<div class="tchips">
					<button class="tchip" class:on={!typeFilter} onclick={() => (typeFilter = '')}>All</button>
					{#each Object.entries(TYPES) as [k, t] (k)}
						<button class="tchip" class:on={typeFilter === k} onclick={() => (typeFilter = k)}>
							<span class="gd" style:background={t.color}></span>{t.label}
						</button>
					{/each}
				</div>
				<span class="sp1"></span>
				<button class="tchip" class:on={aiOnly} onclick={() => (aiOnly = !aiOnly)}>✦ {$aiName} only</button>
			</div>

			{#if loading && !docs.length}
				<div class="dlist sk">
					{#each [72, 55, 64, 40, 60] as w, i (i)}
						<div class="skrow">
							<span class="skb" style="width:14px"></span>
							<span class="skb" style="width:{w}%"></span>
							<span class="skb" style="width:50px"></span>
							<span class="skb" style="width:80px"></span>
							<span class="skb" style="width:60px"></span>
							<span class="skb" style="width:30px"></span>
						</div>
					{/each}
				</div>
			{:else if docs.length === 0 && !docsError}
				<div class="hero">
					<span class="ic">✦</span>
					<span class="t">{$aiName}'s engineering journal</span>
					<span class="d faint">{$aiName} writes a document when it implements or changes something — what it is, how it works, a mermaid diagram, key files. Nothing recorded yet.</span>
				</div>
			{:else}
				<div class="dlist">
					<div class="dhead"><span></span><span>Title</span><span>Issue</span><span>Epic</span><span>Author</span><span>Updated</span></div>
					{#each filtered as d (d.id)}
						{@const key = issueKeyOf(d)}
						{@const epic = epicOf(d)}
						<button class="drow" onclick={() => open(d)}>
							<span class="d-top">
								<span class="d-ic" style:color={typeMeta(d.type).color}>{typeMeta(d.type).icon}</span>
								<span class="d-title">{d.title || 'Untitled'}{#if d.author === 'ai'}<span class="ai">✦</span>{/if}</span>
							</span>
							<span class="d-key">{key || '—'}</span>
							<span class="d-epic" class:d-dash={!epic}>{#if epic}<span class="ic">{epic.icon}</span>{epic.label}{:else}—{/if}</span>
							<span class="d-auth"><span class="av" class:ai={d.author === 'ai'}>{d.author === 'ai' ? $aiName.charAt(0).toUpperCase() : 'Y'}</span>{d.author === 'ai' ? $aiName : 'You'}</span>
							<span class="d-time">{rel(d.updatedAt)}</span>
							<span class="d-msub">
								{#if key}<span class="mono">{key}</span><span class="sep">·</span>{:else if epic}<span class="ic">{epic.icon}</span>{epic.label}<span class="sep">·</span>{/if}
								<span>{rel(d.updatedAt)}</span>
							</span>
						</button>
					{:else}
						<div class="empty faint">No matches.</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
	</div>
</div>

<style>
	.artifacts { display: flex; flex-direction: column; height: 100%; min-height: 0; }

	/* ---- index ---- */
	.main { flex: 1; min-height: 0; display: flex; flex-direction: column; padding: 18px clamp(16px, 4vw, 28px) 24px; overflow-y: auto; }
	.mtop { display: flex; align-items: baseline; gap: 10px; margin-bottom: 14px; flex: none; }
	.ptitle { font-family: var(--serif); font-weight: 400; font-size: var(--t-xl); letter-spacing: -0.01em; color: var(--ink); }
	.msub { font-family: var(--mono); font-size: var(--t-xs); color: var(--ink-3); }

	.err { display: flex; align-items: center; gap: 10px; justify-content: space-between; padding: 10px 14px; margin-bottom: 14px; background: var(--danger-soft); border: 1px solid var(--danger); border-radius: var(--r); color: var(--danger); font-size: var(--t-sm); flex: none; }

	.gaps { display: flex; flex-direction: column; gap: 6px; background: var(--surface); border: 1px solid var(--line); border-radius: var(--r); padding: 11px 14px; margin-bottom: 14px; flex: none; }
	.gaps-h { display: flex; align-items: center; gap: 8px; font-size: var(--t-sm); font-weight: 600; color: var(--ink); background: none; border: none; padding: 0; text-align: left; }
	.gaps-h .gdot { color: var(--st-progress); font-size: var(--t-sm); }
	.gaps-h .n { font-family: var(--mono); font-weight: 500; color: var(--st-progress); background: color-mix(in srgb, var(--st-progress) 16%, var(--surface)); border-radius: 999px; padding: 0 8px; height: 18px; display: inline-flex; align-items: center; font-size: var(--t-xs); }
	.gaps-list { display: flex; flex-direction: column; gap: 1px; max-height: 220px; overflow-y: auto; }
	.gaps-item { display: flex; align-items: center; gap: 10px; padding: 5px 8px; border-radius: var(--r-sm); font-size: var(--t-sm); color: var(--ink-2); background: none; border: none; text-align: left; }
	.gaps-item:hover { background: var(--hover); color: var(--ink); }
	.gaps-item .k { color: var(--ink-3); font-size: var(--t-xs); flex: none; width: 54px; }
	.gaps-t { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.dgs { display: flex; flex-direction: column; gap: 6px; background: var(--surface); border: 1px solid var(--line); border-radius: var(--r); padding: 11px 14px; margin-bottom: 14px; flex: none; }
	.dgs.bad { border-color: var(--danger); background: var(--danger-soft); }
	.dgs-h { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
	.dgs-t { display: flex; align-items: center; gap: 8px; font-size: var(--t-sm); font-weight: 600; color: var(--ink); }
	.dgs-t .n { font-family: var(--mono); font-weight: 500; color: var(--danger); font-size: var(--t-xs); }
	.dgs-ok { font-size: var(--t-sm); color: var(--ink-2); }
	.dgs-item { display: grid; grid-template-columns: minmax(0, 1fr) 110px minmax(0, 1.4fr); gap: 12px; align-items: center; padding: 5px 8px; border-radius: var(--r-sm); font-size: var(--t-sm); color: var(--ink-2); text-decoration: none; }
	.dgs-item:hover { background: var(--hover); color: var(--ink); }
	.dgs-n { color: var(--ink-3); font-size: var(--t-xs); }
	.dgs-e { color: var(--danger); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.toolsrow { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding-bottom: 13px; border-bottom: 1px solid var(--line); margin-bottom: 12px; flex: none; }
	.inp { height: 30px; padding: 0 10px; width: 220px; }
	.tchips { display: flex; gap: 6px; flex-wrap: wrap; }
	.tchip { display: inline-flex; align-items: center; gap: 6px; height: 27px; padding: 0 10px; border-radius: 999px; border: 1px solid var(--line); background: var(--surface); font-size: var(--t-sm); color: var(--ink-2); box-sizing: border-box; white-space: nowrap; }
	.tchip:hover { border-color: var(--line-strong); color: var(--ink); }
	.tchip.on { background: var(--accent-soft); border-color: var(--accent); color: var(--ink); }
	.tchip.ghost { background: var(--surface); }
	.tchip .gd { width: 7px; height: 7px; border-radius: 50%; flex: none; }
	.sp1 { flex: 1; }

	.dlist { border: 1px solid var(--line); border-radius: var(--r); background: var(--surface); overflow: hidden; flex: none; }
	.dhead, .drow { display: grid; grid-template-columns: 22px minmax(0, 1fr) 88px 150px 110px 56px; gap: 12px; align-items: center; padding: 0 14px; box-sizing: border-box; width: 100%; }
	.dhead { height: 28px; font-size: var(--t-xs); color: var(--ink-3); text-transform: uppercase; letter-spacing: 0.04em; border-bottom: 1px solid var(--line); background: var(--sunken); }
	.drow { height: 46px; border-bottom: 1px solid var(--line); background: none; border-left: none; border-right: none; border-top: none; text-align: left; color: var(--ink); }
	.drow:hover { background: var(--hover); }
	.drow:last-child { border-bottom: 0; }
	.d-top { display: contents; }
	.d-ic { font-size: var(--t-base); }
	.d-title { font-family: var(--serif); font-size: var(--t-base); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: flex; align-items: center; gap: 7px; }
	.d-title .ai { color: var(--accent); font-size: var(--t-xs); flex: none; }
	.d-key { font-family: var(--mono); font-size: var(--t-sm); color: var(--ink-3); }
	.d-epic { display: flex; align-items: center; gap: 6px; font-size: var(--t-sm); color: var(--ink-2); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
	.d-epic .ic { color: var(--ink-3); flex: none; }
	.d-epic.d-dash { color: var(--ink-3); }
	.d-auth { display: flex; align-items: center; gap: 6px; font-size: var(--t-sm); color: var(--ink-2); }
	.av { width: 18px; height: 18px; border-radius: 50%; background: var(--sunken); border: 1px solid var(--line); display: flex; align-items: center; justify-content: center; font: 600 var(--t-xs) var(--font); color: var(--ink-2); flex: none; }
	.av.ai { background: var(--accent-soft); color: var(--accent); border-color: transparent; }
	.d-time { font-size: var(--t-xs); color: var(--ink-3); text-align: right; }
	.d-msub { display: none; align-items: center; gap: 5px; font-size: var(--t-xs); color: var(--ink-3); padding-left: 21px; }
	.d-msub .sep { color: var(--ink-3); }
	.empty { padding: 30px 14px; text-align: center; font-size: var(--t-sm); line-height: 1.5; }

	.hero { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 9px; text-align: center; padding: 40px 20px; }
	.hero .ic { font-size: var(--t-xl); color: var(--accent); }
	.hero .t { font-family: var(--serif); font-size: var(--t-lg); color: var(--ink); }
	.hero .d { font-size: var(--t-sm); max-width: 340px; line-height: 1.6; }

	.skrow { display: grid; grid-template-columns: 22px minmax(0, 1fr) 88px 150px 110px 56px; gap: 12px; align-items: center; height: 46px; padding: 0 14px; border-bottom: 1px solid var(--line); box-sizing: border-box; }
	.skrow:last-child { border-bottom: 0; }
	.skb { height: 9px; border-radius: var(--r-sm); background: var(--line); display: inline-block; }
	@media (prefers-reduced-motion: no-preference) {
		.sk .skb, .rsk .skb { animation: skshim 1.6s ease-in-out infinite; }
	}
	@keyframes skshim { 0%, 100% { opacity: 0.55; } 50% { opacity: 1; } }

	/* ---- reader ---- */
	.rtop { display: flex; align-items: center; gap: 12px; padding: 14px clamp(16px, 4vw, 24px); border-bottom: 1px solid var(--line); flex: none; }
	.crumb { display: none; align-items: center; gap: 6px; font-size: var(--t-sm); color: var(--ink-3); min-width: 0; overflow: hidden; }
	.crumb b { color: var(--ink-2); font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.crumb .sep { flex: none; }
	.crumb-lnk { background: none; border: none; color: var(--ink-3); font-size: var(--t-sm); padding: 0; text-transform: none; }
	.crumb-lnk:hover { color: var(--ink); }
	.rsp { flex: 1; }
	.delbtn { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 12px; border-radius: var(--r-sm); border: 1px solid var(--line); background: var(--surface); color: var(--ink-2); font-size: var(--t-sm); box-sizing: border-box; }
	.delbtn:hover { border-color: var(--line-strong); color: var(--ink); }
	.delbtn.danger { background: var(--danger-soft); border-color: var(--danger); color: var(--danger); font-weight: 500; }

	.rbody { flex: 1; display: flex; overflow: hidden; min-height: 0; }
	.rmain { flex: 1; overflow-y: auto; min-width: 0; }
	.rwrap { max-width: 68ch; margin: 0 auto; padding: 36px 32px 100px; }
	.rtitle { font-family: var(--serif); font-weight: 400; font-size: var(--t-2xl); line-height: 1.15; margin: 0 0 14px; letter-spacing: -0.01em; color: var(--ink); }
	.rmeta { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding-bottom: 16px; margin-bottom: 6px; border-bottom: 1px solid var(--line); font-size: var(--t-sm); color: var(--ink-2); }
	.rmeta .sep { color: var(--ink-3); }
	.rmeta .ai { color: var(--accent); }
	.lnk { background: none; border: none; color: var(--ink-2); padding: 0; font-size: inherit; }
	.lnk:hover { color: var(--accent); }
	.rlabels { display: flex; flex-wrap: wrap; gap: 6px; margin: 14px 0 4px; }
	.lbtn { background: none; border: none; padding: 0; display: inline-flex; align-items: center; }
	.lbtn .x { font-size: var(--t-xs); color: var(--ink-3); margin-left: 3px; }

	.rc { font-family: var(--serif); font-size: var(--t-md); line-height: 1.7; color: var(--ink); margin-top: 22px; }
	.rwrap .rc :global(h1) { font-family: var(--serif); font-weight: 500; font-size: var(--t-xl); margin: 0 0 14px; }
	.rwrap .rc :global(h2) { font-family: var(--serif); font-weight: 500; font-size: var(--t-xl); margin: 30px 0 12px; }
	.rwrap .rc :global(h2:first-child) { margin-top: 0; }
	.rwrap .rc :global(h3) { font-family: var(--serif); font-weight: 500; font-size: var(--t-lg); margin: 24px 0 10px; }
	.rwrap .rc :global(p) { margin: 0 0 15px; }
	.rwrap .rc :global(ul), .rwrap .rc :global(ol) { margin: 0 0 16px; padding-left: 1.3em; }
	.rwrap .rc :global(li) { margin-bottom: 5px; }
	.rwrap .rc :global(code) { font-family: var(--mono); font-size: var(--t-sm); background: var(--sunken); padding: 1px 5px; border-radius: var(--r-sm); }
	.rwrap .rc :global(table) { border-collapse: collapse; width: 100%; margin: 0 0 20px; font-family: var(--font); font-size: var(--t-sm); }
	.rwrap .rc :global(th), .rwrap .rc :global(td) { border: 1px solid var(--line); padding: 7px 10px; text-align: left; }
	.rwrap .rc :global(th) { background: var(--sunken); font-weight: 600; color: var(--ink-2); font-size: var(--t-xs); text-transform: uppercase; letter-spacing: 0.03em; }
	.rwrap .rc :global(pre) { background: var(--sunken); border: 1px solid var(--line); border-radius: var(--r); padding: 14px 16px; overflow-x: auto; margin: 0 0 20px; }
	.rwrap .rc :global(pre code) { background: none; padding: 0; font-size: var(--t-sm); }
	.rwrap .rc :global(blockquote) { border-left: 3px solid var(--line-strong); margin: 0 0 16px; padding-left: 1em; color: var(--ink-2); }
	.rwrap .rc :global(.mermaid-container) { margin: 4px 0 20px; }

	.rerr { display: flex; align-items: center; gap: 12px; padding: 16px; background: var(--danger-soft); border: 1px solid var(--danger); border-radius: var(--r); color: var(--danger); font-size: var(--t-base); }
	.rsk { padding-top: 4px; }
	.rsk .skb { border-radius: var(--r-sm); background: var(--line); display: block; }
	@media (prefers-reduced-motion: no-preference) { .rsk .skb { animation: skshim 1.6s ease-in-out infinite; } }

	.rail { width: 232px; flex: none; border-left: 1px solid var(--line); padding: 24px 18px; overflow-y: auto; display: flex; flex-direction: column; gap: 22px; box-sizing: border-box; }
	.rh { font-family: var(--mono); font-size: var(--t-xs); letter-spacing: 0.06em; text-transform: uppercase; color: var(--ink-3); margin-bottom: 9px; }
	.toc { display: flex; flex-direction: column; gap: 1px; }
	.ta { text-align: left; background: none; border: none; border-left: 2px solid transparent; color: var(--ink-2); font-size: var(--t-sm); padding: 5px 8px; }
	.ta.sub { padding-left: 20px; font-size: var(--t-sm); }
	.ta:hover, .ta.on { color: var(--ink); border-left-color: var(--accent); font-weight: 500; }
	.attach { display: flex; align-items: center; gap: 8px; height: 30px; width: 100%; padding: 0 10px; border: 1px solid var(--line-strong); border-radius: var(--r-sm); background: var(--surface); font-size: var(--t-sm); color: var(--ink); text-align: left; box-sizing: border-box; }
	.attach:hover { border-color: var(--accent); }

	.chip { display: inline-flex; align-items: center; gap: 6px; background: var(--surface); border: 1px solid var(--line); color: var(--ink-2); border-radius: var(--r); padding: 5px 10px; font-size: var(--t-sm); }
	.chip:hover { background: var(--hover); color: var(--ink); }
	.typewrap, .attachwrap, .railwrap { position: relative; display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
	.bd { position: fixed; inset: 0; z-index: 30; background: none; border: none; }
	.pop { position: absolute; top: calc(100% + 6px); left: 0; z-index: 31; min-width: 170px; background: var(--surface); border: 1px solid var(--line-strong); border-radius: var(--r-lg); box-shadow: var(--shadow-2); padding: 5px; }
	.pop.wide { width: 250px; max-height: 320px; overflow-y: auto; }
	.ps { font: 600 var(--t-xs)/1 var(--font); letter-spacing: 0.05em; text-transform: uppercase; color: var(--ink-3); padding: 8px 9px 3px; }
	.pi { display: flex; align-items: center; gap: 8px; width: 100%; text-align: left; background: none; border: none; color: var(--ink); padding: 7px 9px; border-radius: var(--r-sm); font-size: var(--t-sm); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.pi:hover { background: var(--hover); }
	.pi.on { color: var(--accent); }
	.pi .mono { font-family: var(--mono); color: var(--ink-3); font-size: var(--t-xs); }
	.pi .dim { color: var(--ink-3); }

	.mback { display: none; }

	@media (max-width: 720px) {
		.main { padding: 14px 14px 20px; }
		.inp { width: 100%; }
		.dhead { display: none; }
		.drow { grid-template-columns: none; display: flex; flex-direction: column; align-items: flex-start; gap: 4px; height: auto; padding: 10px 14px; }
		.d-top { display: flex; align-items: center; gap: 7px; width: 100%; }
		.d-key, .d-epic, .d-auth, .d-time { display: none; }
		.d-msub { display: flex; }

		.rtop { padding: 10px 14px; }
		.rwrap { padding: 18px 16px 60px; }
		.rtitle { font-size: var(--t-xl); }
		.rail { display: none; }
		.mback { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; border-radius: var(--r); border: 1px solid var(--line); background: var(--surface); color: var(--ink-2); font-size: var(--t-lg); flex: none; }
		.mback:hover { color: var(--ink); }
		.crumb { display: none; }
	}
</style>
