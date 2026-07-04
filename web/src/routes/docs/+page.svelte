<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { projects, initiatives, issues, labels as allLabels } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import Markdown from '$components/Markdown.svelte';
	import LabelPill from '$components/LabelPill.svelte';

	let docs = $state([]);
	let sel = $state(null);
	let query = $state('');
	let titleDraft = $state('');
	let bodyDraft = $state('');
	let previewSource = $state('');
	let mode = $state('preview'); // edit | split | preview
	let saveStatus = $state('idle'); // idle | saving | saved
	let attachOpen = $state(false);
	let labelPickerOpen = $state(false);

	let saveTimer, previewTimer;
	let dirty = false;

	onMount(load);
	async function load() {
		docs = (await api.documents()) || [];
	}

	const filtered = $derived(
		query.trim()
			? docs.filter((d) => d.title.toLowerCase().includes(query.trim().toLowerCase()))
			: docs
	);

	async function open(d) {
		await flushSave();
		sel = await api.document(d.id);
		titleDraft = sel.title;
		bodyDraft = sel.bodyMd || '';
		previewSource = bodyDraft;
		mode = 'preview';
		saveStatus = 'idle';
		dirty = false;
		attachOpen = false;
		labelPickerOpen = false;
	}

	async function create() {
		await flushSave();
		sel = await api.saveDocument({
			title: 'Untitled',
			bodyMd: ''
		});
		titleDraft = 'Untitled';
		bodyDraft = '';
		previewSource = '';
		mode = 'split';
		dirty = false;
		saveStatus = 'saved';
		await load();
	}

	// ---- autosave ----
	function scheduleSave() {
		dirty = true;
		saveStatus = 'saving';
		clearTimeout(saveTimer);
		saveTimer = setTimeout(() => persist({ title: titleDraft, bodyMd: bodyDraft }), 700);
	}
	async function flushSave() {
		clearTimeout(saveTimer);
		if (sel && dirty) await persist({ title: titleDraft, bodyMd: bodyDraft });
	}
	async function persist(extra) {
		if (!sel) return;
		saveStatus = 'saving';
		try {
			const updated = await api.saveDocument({ id: sel.id, ...extra });
			sel = updated;
			// keep drafts in sync only for fields we did not just type
			if (extra.title === undefined) titleDraft = sel.title;
			// update the list entry in place (no reorder while editing)
			docs = docs.map((d) => (d.id === sel.id ? sel : d));
			dirty = false;
			saveStatus = 'saved';
		} catch (e) {
			saveStatus = 'idle';
			showToast('Save failed: ' + e.message, 'error');
		}
	}

	function onBodyInput() {
		scheduleSave();
		clearTimeout(previewTimer);
		previewTimer = setTimeout(() => (previewSource = bodyDraft), 350);
	}
	function onTitleInput() {
		scheduleSave();
	}
	function onKey(e) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			flushSave();
		}
	}

	// ---- attach ----
	function currentAttach() {
		if (!sel) return null;
		if (sel.initiativeId)
			return { icon: '◈', label: $initiatives.find((i) => i.id === sel.initiativeId)?.name ?? 'Initiative' };
		if (sel.projectId)
			return { icon: '▢', label: $projects.find((p) => p.id === sel.projectId)?.name ?? 'Project' };
		if (sel.issueId) {
			const is = $issues.find((i) => i.id === sel.issueId);
			return { icon: '◦', label: is ? `${is.key} ${is.title}` : 'Issue' };
		}
		return null;
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

	async function del() {
		if (!sel || !confirm('Delete this document?')) return;
		clearTimeout(saveTimer);
		dirty = false;
		await api.deleteDocument(sel.id);
		sel = null;
		await load();
	}

	function attachLabel(d) {
		if (d.initiativeId) return '◈ ' + ($initiatives.find((i) => i.id === d.initiativeId)?.name ?? 'Initiative');
		if (d.projectId) return '▢ ' + ($projects.find((p) => p.id === d.projectId)?.name ?? 'Project');
		if (d.issueId) return ($issues.find((i) => i.id === d.issueId)?.key ?? 'Issue');
		return '';
	}
	function relTime(iso) {
		const t = new Date(iso).getTime();
		const s = Math.floor((Date.now() - t) / 1000);
		if (s < 60) return 'now';
		if (s < 3600) return Math.floor(s / 60) + 'm';
		if (s < 86400) return Math.floor(s / 3600) + 'h';
		if (s < 604800) return Math.floor(s / 86400) + 'd';
		return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}
</script>

<svelte:window onkeydown={onKey} />

<div class="docs">
	<!-- index -->
	<aside class="index">
		<div class="index-head">
			<span class="ih-title">Documents</span>
			<button class="new-btn" title="New document" onclick={create}>+</button>
		</div>
		<input class="search" placeholder="Search documents…" bind:value={query} />
		<div class="doc-list">
			{#each filtered as d (d.id)}
				<button class="doc-item" class:active={sel && sel.id === d.id} onclick={() => open(d)}>
					<div class="di-title">{d.title || 'Untitled'}</div>
					<div class="di-sub">
						{#if attachLabel(d)}<span class="di-attach">{attachLabel(d)}</span><span class="di-sep">·</span>{/if}
						<span class="di-time">{relTime(d.updatedAt)}</span>
						{#each d.labels ?? [] as l (l.id)}<span class="di-dot" style:background={l.color}></span>{/each}
					</div>
				</button>
			{:else}
				<div class="empty faint">{query ? 'No matches.' : 'No documents yet.'}</div>
			{/each}
		</div>
	</aside>

	<!-- view / editor -->
	<section class="view">
		{#if sel}
			<div class="toolbar">
				<div class="seg">
					<button class:on={mode === 'edit'} onclick={() => (mode = 'edit')}>Edit</button>
					<button class:on={mode === 'split'} onclick={() => (mode = 'split')}>Split</button>
					<button class:on={mode === 'preview'} onclick={() => (mode = 'preview')}>Preview</button>
				</div>
				<span class="status {saveStatus}">
					{saveStatus === 'saving' ? 'Saving…' : saveStatus === 'saved' ? 'Saved' : ''}
				</span>
				<div class="grow"></div>
				<button class="btn ghost" onclick={del} title="Delete">🗑</button>
			</div>

			<div class="scroll">
				<div class="doc">
					<input
						class="title"
						bind:value={titleDraft}
						oninput={onTitleInput}
						onblur={flushSave}
						placeholder="Untitled"
					/>

					<div class="meta">
						<!-- attach chip -->
						<div class="attach-wrap">
							<button class="chip" class:set={currentAttach()} onclick={() => (attachOpen = !attachOpen)}>
								{#if currentAttach()}
									<span class="ci">{currentAttach().icon}</span>{currentAttach().label}
								{:else}
									＋ Attach
								{/if}
							</button>
							{#if attachOpen}
								<button class="pop-backdrop" aria-label="close" onclick={() => (attachOpen = false)}></button>
								<div class="popover">
									<button class="pop-item" onclick={() => setAttach('none', '')}>No attachment</button>
									{#if $initiatives.length}<div class="pop-sec">Initiatives</div>{/if}
									{#each $initiatives as i (i.id)}
										<button class="pop-item" onclick={() => setAttach('initiative', i.id)}><span class="ci">◈</span>{i.name}</button>
									{/each}
									{#if $projects.length}<div class="pop-sec">Projects</div>{/if}
									{#each $projects as p (p.id)}
										<button class="pop-item" onclick={() => setAttach('project', p.id)}><span class="ci">▢</span>{p.name}</button>
									{/each}
									{#if $issues.length}<div class="pop-sec">Issues</div>{/if}
									{#each $issues.slice(0, 40) as is (is.id)}
										<button class="pop-item" onclick={() => setAttach('issue', is.id)}><span class="ci mono">{is.key}</span>{is.title}</button>
									{/each}
								</div>
							{/if}
						</div>

						<!-- labels -->
						{#each sel.labels as l (l.id)}
							<button class="label-chip" onclick={() => toggleLabel(l.id)}>
								<LabelPill label={l} /><span class="x">✕</span>
							</button>
						{/each}
						<div class="attach-wrap">
							<button class="chip" onclick={() => (labelPickerOpen = !labelPickerOpen)}>＋ Label</button>
							{#if labelPickerOpen}
								<button class="pop-backdrop" aria-label="close" onclick={() => (labelPickerOpen = false)}></button>
								<div class="popover">
									{#each $allLabels as l (l.id)}
										<button class="pop-item" class:on={sel.labels.some((x) => x.id === l.id)} onclick={() => toggleLabel(l.id)}>
											<span class="dot" style:background={l.color}></span>{l.name}
										</button>
									{/each}
								</div>
							{/if}
						</div>
					</div>

					<div class="body {mode}">
						{#if mode === 'edit' || mode === 'split'}
							<textarea
								class="editor"
								bind:value={bodyDraft}
								oninput={onBodyInput}
								onblur={flushSave}
								placeholder="Write markdown…  ```mermaid diagrams render in preview."
							></textarea>
						{/if}
						{#if mode === 'preview' || mode === 'split'}
							<div class="rendered">
								{#if previewSource.trim()}
									<Markdown source={previewSource} />
								{:else}
									<span class="faint">Nothing to preview yet.</span>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			</div>
		{:else}
			<div class="placeholder">
				<div class="ph-icon">📄</div>
				<div class="ph-title">No document selected</div>
				<div class="faint">Pick one on the left, or create a new document.</div>
				<button class="btn primary" onclick={create}>+ New document</button>
			</div>
		{/if}
	</section>
</div>

<style>
	.docs {
		display: flex;
		height: 100%;
		min-height: 0;
	}
	/* index */
	.index {
		width: 288px;
		flex: none;
		border-right: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.index-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 14px 14px 8px;
	}
	.ih-title {
		font: 600 15px/1 var(--disp);
	}
	.new-btn {
		width: 26px;
		height: 26px;
		border-radius: 7px;
		background: var(--accent-grad);
		color: #fff;
		border: none;
		font-size: 17px;
		line-height: 1;
		box-shadow: 0 3px 10px var(--accent-soft);
	}
	.search {
		margin: 0 12px 8px;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 7px 10px;
		font-size: 13px;
		outline: none;
	}
	.search:focus {
		border-color: var(--accent);
	}
	.doc-list {
		flex: 1;
		overflow-y: auto;
		padding: 4px 8px 12px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.doc-item {
		text-align: left;
		background: none;
		border: none;
		color: var(--text);
		padding: 8px 10px;
		border-radius: 8px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.doc-item:hover {
		background: var(--bg-elev);
	}
	.doc-item.active {
		background: var(--bg-hover);
	}
	.di-title {
		font-size: 13.5px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.di-sub {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: 11px;
		color: var(--text-faint);
	}
	.di-attach {
		color: var(--text-dim);
		max-width: 130px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.di-dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
	}
	.empty {
		padding: 30px 12px;
		text-align: center;
		font-size: 13px;
	}

	/* view */
	.view {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
	}
	.toolbar {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 9px 16px;
		border-bottom: 1px solid var(--border);
	}
	.seg {
		display: flex;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2px;
		gap: 2px;
	}
	.seg button {
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 4px 12px;
		border-radius: 6px;
		font-size: 12.5px;
	}
	.seg button.on {
		background: var(--bg-elev2);
		color: var(--text);
	}
	.status {
		font-size: 12px;
		color: var(--text-faint);
		min-width: 52px;
	}
	.status.saved {
		color: var(--st-done);
	}
	.grow {
		flex: 1;
	}
	.scroll {
		flex: 1;
		overflow-y: auto;
		min-height: 0;
	}
	.doc {
		max-width: 780px;
		margin: 0 auto;
		padding: 28px 32px 80px;
		display: flex;
		flex-direction: column;
		min-height: 100%;
	}
	.title {
		background: none;
		border: none;
		outline: none;
		font: 700 30px/1.2 var(--disp);
		letter-spacing: -0.01em;
		color: var(--text);
		padding: 0;
		margin-bottom: 12px;
	}
	.title::placeholder {
		color: var(--text-faint);
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 7px;
		margin-bottom: 20px;
		padding-bottom: 16px;
		border-bottom: 1px solid var(--border);
	}
	.attach-wrap {
		position: relative;
		display: inline-flex;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		color: var(--text-dim);
		border-radius: 7px;
		padding: 4px 10px;
		font-size: 12.5px;
	}
	.chip:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
	.chip.set {
		color: var(--text);
	}
	.ci {
		color: var(--text-faint);
		font-size: 11px;
	}
	.ci.mono {
		font-family: var(--mono);
	}
	.label-chip {
		background: none;
		border: none;
		padding: 0;
		display: inline-flex;
		align-items: center;
	}
	.label-chip .x {
		font-size: 9px;
		color: var(--text-faint);
		margin-left: 3px;
	}
	.pop-backdrop {
		position: fixed;
		inset: 0;
		z-index: 30;
		background: none;
		border: none;
	}
	.popover {
		position: absolute;
		top: calc(100% + 6px);
		left: 0;
		z-index: 31;
		width: 280px;
		max-height: 340px;
		overflow-y: auto;
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 10px;
		box-shadow: var(--shadow);
		padding: 5px;
	}
	.pop-sec {
		font-size: 10.5px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-faint);
		padding: 8px 9px 3px;
	}
	.pop-item {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		text-align: left;
		background: none;
		border: none;
		color: var(--text);
		padding: 7px 9px;
		border-radius: 6px;
		font-size: 13px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.pop-item:hover {
		background: var(--bg-hover);
	}
	.pop-item.on {
		color: var(--accent);
	}

	.body {
		flex: 1;
		min-height: 340px;
	}
	.body.split {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 20px;
	}
	.editor {
		width: 100%;
		height: 100%;
		min-height: 340px;
		background: none;
		border: none;
		outline: none;
		resize: none;
		color: var(--text);
		font-family: var(--mono);
		font-size: 14px;
		line-height: 1.7;
	}
	.body.split .editor {
		border-right: 1px solid var(--border);
		padding-right: 18px;
	}
	.rendered {
		min-height: 200px;
		font-size: 15px;
	}

	/* empty */
	.placeholder {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 8px;
	}
	.ph-icon {
		font-size: 34px;
		opacity: 0.5;
	}
	.ph-title {
		font: 600 16px/1 var(--disp);
	}
	.placeholder .btn {
		margin-top: 10px;
	}

	@media (max-width: 720px) {
		.index {
			width: 150px;
		}
		.doc {
			padding: 18px 16px 60px;
		}
		.body.split {
			grid-template-columns: 1fr;
		}
	}
</style>
