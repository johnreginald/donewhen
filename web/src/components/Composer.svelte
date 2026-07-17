<script>
	import { get } from 'svelte/store';
	import { api } from '$lib/api.js';
	import {
		states,
		projects,
		initiatives,
		labels,
		activeProject,
		activeInitiative,
		loadIssues,
		loadMeta,
		PRIORITIES
	} from '$lib/store.js';
	import { composer, closeComposer, showToast, openIssue } from '$lib/ui.js';
	import PriorityMenu from './PriorityMenu.svelte';
	import StatusMenu from './StatusMenu.svelte';
	import EpicMenu from './EpicMenu.svelte';

	// Epics shown when creating an issue: scoped to the active Project if one is
	// selected, otherwise all.
	const epicOptions = $derived(
		$activeInitiative ? $projects.filter((p) => p.initiativeId === $activeInitiative) : $projects
	);

	let kind = $state('issue');
	let title = $state('');
	let name = $state('');
	let desc = $state('');
	let stateId = $state('');
	let projectId = $state('');
	let initiativeId = $state('');
	let priority = $state(0);
	let selLabels = $state(new Set());
	let saving = $state(false);
	let firstInput = $state(null);
	let editId = $state(''); // set when editing an existing project/epic
	let confirmDel = $state(false);

	// Raenil's Project entity is shown as "Epic"; its Initiative entity as "Project".
	const NOUN = { issue: 'issue', project: 'epic', initiative: 'project' };
	const heading = $derived((editId ? 'Edit ' : 'New ') + NOUN[kind]);

	function defaultStateId() {
		const st = get(states);
		return st.find((s) => s.name === 'Backlog')?.id || st[0]?.id || '';
	}

	// Reset the form each time the composer opens. Reads stores via get() so the
	// effect only re-runs when the composer itself changes, not on data updates.
	$effect(() => {
		const c = $composer;
		if (!c) return;
		const pf = c.prefill || {};
		kind = c.kind;
		editId = pf.id || '';
		confirmDel = false;
		title = '';
		name = pf.name || '';
		desc = pf.description || pf.descriptionMd || '';
		priority = 0;
		selLabels = new Set();
		initiativeId = pf.initiativeId || '';
		stateId = pf.stateId || defaultStateId();
		projectId = pf.projectId || get(activeProject) || '';
		queueMicrotask(() => firstInput && firstInput.focus());
	});

	function toggleLabel(id) {
		const next = new Set(selLabels);
		next.has(id) ? next.delete(id) : next.add(id);
		selLabels = next;
	}

	async function save() {
		if (saving) return;
		saving = true;
		try {
			if (kind === 'issue') {
				if (!title.trim()) {
					showToast('Title required', 'error');
					return;
				}
				const is = await api.createIssue({
					title: title.trim(),
					descriptionMd: desc,
					stateId,
					projectId: projectId || undefined,
					priority,
					labelIds: [...selLabels]
				});
				await loadIssues();
				showToast(`${is.key} created`);
				closeComposer();
				openIssue(is.key);
			} else if (kind === 'project') {
				if (!name.trim()) {
					showToast('Name required', 'error');
					return;
				}
				const body = { name: name.trim(), descriptionMd: desc, initiativeId: initiativeId || null };
				if (editId) await api.updateProject(editId, body);
				else await api.saveProject(body);
				await refreshAfterMeta();
				showToast(editId ? 'Epic saved' : 'Epic created');
				closeComposer();
			} else if (kind === 'initiative') {
				if (!name.trim()) {
					showToast('Name required', 'error');
					return;
				}
				const body = { name: name.trim(), descriptionMd: desc };
				if (editId) await api.updateInitiative(editId, body);
				else await api.saveInitiative(body);
				await refreshAfterMeta();
				showToast(editId ? 'Project saved' : 'Project created');
				closeComposer();
			}
		} catch (e) {
			showToast(e.message || 'Failed to save', 'error');
		} finally {
			saving = false;
		}
	}

	// Reload metadata + the board (a rename/delete can change what's displayed).
	async function refreshAfterMeta() {
		await loadMeta();
		await loadIssues();
	}

	async function del() {
		if (!editId || saving) return;
		if (!confirmDel) {
			confirmDel = true;
			return;
		}
		saving = true;
		try {
			if (kind === 'project') await api.deleteProject(editId);
			else if (kind === 'initiative') await api.deleteInitiative(editId);
			await refreshAfterMeta();
			showToast(kind === 'project' ? 'Epic deleted' : 'Project deleted');
			closeComposer();
		} catch (e) {
			showToast(e.message || 'Delete failed', 'error');
		} finally {
			saving = false;
		}
	}

	function onKey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			closeComposer();
		} else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			save();
		}
	}
</script>

{#if $composer}
	<div class="backdrop" role="presentation" onclick={closeComposer}></div>
	<div class="modal" role="dialog" aria-modal="true" onkeydown={onKey}>
		<div class="head">
			<span class="dot" class:issue={kind === 'issue'} class:project={kind === 'project'}></span>
			<span class="htitle">{heading}</span>
		</div>

		<div class="body">
			{#if kind === 'issue'}
				<input
					bind:this={firstInput}
					bind:value={title}
					class="big"
					placeholder="Issue title"
				/>
				<textarea bind:value={desc} class="desc" placeholder="Description (markdown, mermaid…)"></textarea>
				<div class="meta">
					<div class="field">
						<span>Status</span>
						<StatusMenu value={stateId} onchange={(v) => (stateId = v)} />
					</div>
					<div class="field">
						<span>Epic</span>
						<EpicMenu value={projectId} options={epicOptions} onchange={(v) => (projectId = v)} />
					</div>
					<div class="field">
						<span>Priority</span>
						<PriorityMenu value={priority} onchange={(v) => (priority = v)} />
					</div>
				</div>
				{#if $labels.length}
					<div class="labels">
						{#each $labels as l (l.id)}
							<button
								type="button"
								class="chip"
								class:on={selLabels.has(l.id)}
								style="--lc:{l.color}"
								onclick={() => toggleLabel(l.id)}>{l.name}</button
							>
						{/each}
					</div>
				{/if}
			{:else}
				<input bind:this={firstInput} bind:value={name} class="big" placeholder="{kind === 'project' ? 'Epic' : 'Project'} name" />
				<textarea bind:value={desc} class="desc" placeholder="Description (optional)"></textarea>
				{#if kind === 'project'}
					<label class="field wide">
						<span>Project</span>
						<select bind:value={initiativeId}>
							<option value="">— None —</option>
							{#each $initiatives as i (i.id)}
								<option value={i.id}>{i.name}</option>
							{/each}
						</select>
					</label>
				{/if}
			{/if}
		</div>

		<div class="foot">
			{#if editId}
				<button class="btn danger" onclick={del} disabled={saving}>
					{confirmDel ? 'Confirm delete' : 'Delete'}
				</button>
			{/if}
			<span class="hint faint">⌘↵ to save · Esc to cancel</span>
			<span class="spacer"></span>
			<button class="btn ghost" onclick={closeComposer}>Cancel</button>
			<button class="btn primary" onclick={save} disabled={saving}>
				{saving ? 'Saving…' : editId ? 'Save' : 'Create'}
			</button>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		z-index: 70;
	}
	.modal {
		position: fixed;
		top: 12vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(600px, 94vw);
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 14px;
		box-shadow: var(--shadow);
		z-index: 71;
		display: flex;
		flex-direction: column;
		max-height: 80vh;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 14px 18px 10px;
	}
	.dot {
		width: 9px;
		height: 9px;
		border-radius: 3px;
		background: var(--text-faint);
	}
	.dot.issue {
		background: var(--accent);
	}
	.dot.project {
		background: var(--accent2);
	}
	.htitle {
		font-weight: 600;
		font-size: 14px;
	}
	.body {
		padding: 4px 18px 8px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.big {
		width: 100%;
		background: transparent;
		border: none;
		outline: none;
		color: var(--text);
		font-size: 19px;
		font-weight: 500;
		padding: 4px 0;
	}
	.big::placeholder {
		color: var(--text-faint);
		font-weight: 500;
	}
	.desc {
		width: 100%;
		min-height: 72px;
		resize: none;
		background: transparent;
		border: none;
		color: var(--text);
		padding: 2px 0;
		font-size: 14.5px;
		line-height: 1.55;
		font-family: inherit;
		outline: none;
		field-sizing: content;
	}
	.desc::placeholder {
		color: var(--text-faint);
	}
	.meta {
		display: flex;
		gap: 10px;
		flex-wrap: wrap;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1;
		min-width: 140px;
	}
	.field.wide {
		min-width: 100%;
	}
	.field span {
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-faint);
	}
	.field select {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		color: var(--text);
		padding: 7px 9px;
		font-size: 13px;
		outline: none;
	}
	.labels {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		max-height: 108px;
		overflow-y: auto;
		padding: 2px;
	}
	.chip {
		font-size: 13px;
		padding: 4px 11px;
		border-radius: 7px;
		border: 1px solid var(--border);
		background: var(--bg-elev);
		color: var(--text);
		font-weight: 500;
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.chip::before {
		content: '';
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--lc, var(--text-faint));
	}
	.chip.on {
		border-color: var(--lc, var(--accent));
		color: var(--text);
		background: color-mix(in srgb, var(--lc, var(--accent)) 16%, var(--bg));
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 12px 18px 14px;
		border-top: 1px solid var(--border);
	}
	.hint {
		font-size: 11px;
	}
	.spacer {
		flex: 1;
	}
</style>
