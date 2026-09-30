<script>
	import { api } from '$lib/api.js';
	import {
		initiatives,
		loadIssues,
		loadMeta
	} from '$lib/store.js';
	import { X } from '@lucide/svelte';
	import { composer, closeComposer, showToast } from '$lib/ui.js';

	// Creates and edits Projects and Epics. Tasks are not created here: they
	// arrive from planning (RePPIT) through the MCP tools.
	let kind = $state('project');
	let name = $state('');
	let desc = $state('');
	let initiativeId = $state('');
	let repoUrl = $state('');
	let saving = $state(false);
	let firstInput = $state(null);
	let editId = $state(''); // set when editing an existing project/epic
	let confirmDel = $state(false);

	// Raenil's Project entity is shown as "Epic"; its Initiative entity as "Project".
	const NOUN = { project: 'epic', initiative: 'project' };
	const heading = $derived((editId ? 'Edit ' : 'New ') + NOUN[kind]);

	// Reset the form each time the composer opens.
	$effect(() => {
		const c = $composer;
		if (!c) return;
		const pf = c.prefill || {};
		kind = c.kind;
		editId = pf.id || '';
		confirmDel = false;
		name = pf.name || '';
		desc = pf.description || pf.descriptionMd || '';
		initiativeId = pf.initiativeId || '';
		repoUrl = pf.repoUrl || '';
		queueMicrotask(() => firstInput && firstInput.focus());
	});

	async function save() {
		if (saving) return;
		saving = true;
		try {
			if (kind === 'project') {
				if (!name.trim()) {
					showToast('Name required', 'error');
					return;
				}
				const body = {
					name: name.trim(),
					descriptionMd: desc,
					initiativeId: initiativeId || null,
					repoUrl: repoUrl.trim() || null
				};
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
				const body = { name: name.trim(), descriptionMd: desc, repoUrl: repoUrl.trim() || null };
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
			<span class="dot" class:project={kind === 'project'}></span>
			<span class="htitle">{heading}</span>
			<button class="hclose" onclick={closeComposer} aria-label="Close"><X size={15} strokeWidth={2} /></button>
		</div>

		<div class="body">
				<input bind:this={firstInput} bind:value={name} class="big" placeholder="{kind === 'project' ? 'Epic' : 'Project'} name" />
				<textarea bind:value={desc} class="desc" placeholder="Description (optional)"></textarea>
				<label class="field wide">
					<span>Repository {kind === 'project' ? '(overrides Project)' : '(default for its Epics)'}</span>
					<input class="rin" bind:value={repoUrl} placeholder="https://github.com/org/repo" />
				</label>
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
		top: 8vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(760px, 94vw);
		max-height: min(680px, 84vh);
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: 14px;
		box-shadow: var(--shadow-2);
		z-index: 71;
		display: flex;
		flex-direction: column;
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
		background: var(--ink-3);
	}
	.dot.project {
		background: var(--accent);
	}
	.htitle {
		font-weight: 600;
		font-size: 14px;
	}
	.head .htitle:not(:first-child) {
		font-weight: 500;
		color: var(--ink-2);
	}
	.hclose {
		margin-left: auto;
		background: none;
		border: none;
		color: var(--ink-3);
		display: inline-flex;
		padding: 4px;
		border-radius: 6px;
	}
	.hclose:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.faint {
		color: var(--ink-3);
	}
	.body {
		flex: 1;
		padding: 6px 22px 10px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.big {
		width: 100%;
		background: transparent;
		border: none;
		outline: none;
		color: var(--ink);
		font-size: 19px;
		font-weight: 500;
		padding: 4px 0;
	}
	.big::placeholder {
		color: var(--ink-3);
		font-weight: 500;
	}
	.desc {
		width: 100%;
		flex: 1;
		min-height: 150px;
		resize: none;
		background: transparent;
		border: none;
		color: var(--ink);
		padding: 2px 0;
		font-size: 14.5px;
		line-height: 1.55;
		font-family: inherit;
		outline: none;
	}
	.desc::placeholder {
		color: var(--ink-3);
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
		color: var(--ink-3);
	}
	.field select,
	.rin {
		background: var(--paper);
		border: 1px solid var(--line);
		border-radius: 7px;
		color: var(--ink);
		padding: 7px 9px;
		font-size: 13px;
		outline: none;
		font-family: inherit;
	}
	.rin:focus {
		border-color: var(--line-strong);
	}
	.rin::placeholder {
		color: var(--ink-3);
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
		border: 1px solid var(--line);
		background: var(--surface);
		color: var(--ink);
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
		background: var(--lc, var(--ink-3));
	}
	.chip.on {
		border-color: var(--lc, var(--accent));
		color: var(--ink);
		background: color-mix(in srgb, var(--lc, var(--accent)) 16%, var(--paper));
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 12px 18px 14px;
		border-top: 1px solid var(--line);
	}
	.hint {
		font-size: 11px;
	}
	.spacer {
		flex: 1;
	}
</style>
