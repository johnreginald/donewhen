<script>
	// "N" — a one-line quick-add that lands in Triage. The title is the point;
	// description, epic, labels and priority are optional extras, not a full
	// issue form (that's the issue detail page's job).
	import { api } from '$lib/api.js';
	import { projects, labels, labelGroups, activeProject, loadIssues, PRIORITIES } from '$lib/store.js';
	import { quickCapture, showToast, openIssue } from '$lib/ui.js';
	import { validateTitle, toggleLabel, hasDraft, TITLE_MAX } from '$lib/capture.js';
	import { X } from '@lucide/svelte';
	import { get } from 'svelte/store';
	import EpicMenu from './EpicMenu.svelte';
	import PriorityIcon from './PriorityIcon.svelte';

	let title = $state('');
	let description = $state('');
	let epicId = $state('');
	let labelIds = $state([]);
	let priority = $state(0);
	let saving = $state(false);
	let error = $state('');
	let confirmDiscard = $state(false);
	let titleEl = $state(null);
	let modalEl = $state(null);
	let lastFocused = null;

	$effect(() => {
		if ($quickCapture) {
			lastFocused = document.activeElement;
			title = '';
			description = '';
			// The epic the board is filtered to is where the user is working.
			epicId = get(activeProject) || '';
			labelIds = [];
			priority = 0;
			saving = false;
			error = '';
			confirmDiscard = false;
			queueMicrotask(() => titleEl && titleEl.focus());
		} else if (lastFocused) {
			lastFocused.focus?.();
			lastFocused = null;
		}
	});

	function close() {
		quickCapture.set(false);
	}

	// Closing with typed text asks first; an untouched dialog just closes.
	function tryClose() {
		if (hasDraft(title, description)) confirmDiscard = true;
		else close();
	}

	// keepOpen clears the draft for the next issue (epic, labels and priority
	// stay: a run of captures usually shares them).
	async function create(keepOpen) {
		if (saving) return;
		const v = validateTitle(title);
		if (v.error) {
			error = v.error;
			titleEl?.focus();
			return;
		}
		error = '';
		saving = true;
		try {
			const is = await api.createIssue({
				title: v.title,
				descriptionMd: description.trim(),
				stateName: 'Triage',
				projectId: epicId || undefined,
				priority,
				labelIds
			});
			showToast(`${is.key} created`, 'info', { actionLabel: 'Open', onAction: () => openIssue(is.key) });
			loadIssues();
			if (keepOpen) {
				title = '';
				description = '';
				titleEl?.focus();
			} else {
				close();
			}
		} catch (e) {
			error = e.message || 'Failed to create';
		} finally {
			saving = false;
		}
	}

	function pickLabel(id) {
		labelIds = toggleLabel(labelIds, id, $labels, $labelGroups);
	}

	// Labels shown group by group, so an exclusive group reads as one choice.
	const labelSections = $derived(
		$labelGroups
			.map((g) => ({ group: g, items: $labels.filter((l) => l.groupId === g.id) }))
			.filter((s) => s.items.length)
	);

	// Keep Tab cycling inside the dialog instead of leaking to the page behind it.
	function trapTab(e) {
		if (e.key !== 'Tab' || !modalEl) return;
		const nodes = modalEl.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
		const list = Array.from(nodes).filter((el) => !el.disabled);
		if (!list.length) return;
		const first = list[0];
		const last = list[list.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	function onKey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			if (confirmDiscard) confirmDiscard = false;
			else tryClose();
		} else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			create(true);
		} else if (e.key === 'Enter' && !e.shiftKey && e.target === titleEl) {
			e.preventDefault();
			create(false);
		} else {
			trapTab(e);
		}
	}
</script>

{#if $quickCapture}
	<div class="backdrop" role="presentation" onclick={tryClose}></div>
	<div
		class="qc-modal"
		bind:this={modalEl}
		role="dialog"
		aria-modal="true"
		aria-label="New issue"
		tabindex="-1"
		onkeydown={onKey}
	>
		<div class="qc-head">
			<span class="qc-dot"></span>
			<span class="qc-title">New issue</span>
			<span class="qc-dest">→ Triage</span>
			<button class="qc-close" onclick={tryClose} aria-label="Close"><X size={15} strokeWidth={2} /></button>
		</div>
		<div class="qc-body">
			<div class="qc-titlebox">
				<input
					bind:this={titleEl}
					bind:value={title}
					oninput={() => (error = '')}
					class="qc-input"
					class:bad={!!error}
					placeholder="Issue title"
					aria-label="Title"
					aria-invalid={!!error}
					aria-describedby={error ? 'qc-error' : undefined}
					maxlength={TITLE_MAX}
				/>
				{#if error}<div id="qc-error" class="qc-error" role="alert">{error}</div>{/if}
			</div>
			<textarea
				bind:value={description}
				class="textarea qc-desc"
				rows="3"
				placeholder="Description (Markdown, optional)"
				aria-label="Description"
			></textarea>
			<div class="qc-row">
				<label class="qc-field">
					<span>Epic</span>
					<EpicMenu value={epicId} options={$projects} onchange={(v) => (epicId = v)} none="No epic" />
				</label>
				<div class="qc-field">
					<span id="qc-pri-label">Priority</span>
					<div class="qc-pri-row" role="radiogroup" aria-labelledby="qc-pri-label">
						{#each PRIORITIES as p (p.value)}
							<button
								type="button"
								class="qc-pri-opt"
								class:on={priority === p.value}
								role="radio"
								aria-checked={priority === p.value}
								onclick={() => (priority = p.value)}
							>
								<PriorityIcon priority={p.value} />{p.label}
							</button>
						{/each}
					</div>
				</div>
			</div>
			{#each labelSections as sec (sec.group.id)}
				<div class="qc-field wide">
					<span id={`qc-label-${sec.group.id}`}>{sec.group.name}</span>
					<div class="qc-chips" role="group" aria-labelledby={`qc-label-${sec.group.id}`}>
						{#each sec.items as l (l.id)}
							<button
								type="button"
								class="chip"
								class:on={labelIds.includes(l.id)}
								aria-pressed={labelIds.includes(l.id)}
								onclick={() => pickLabel(l.id)}
							>
								<span class="dot" style:background={l.color}></span>{l.name}
							</button>
						{/each}
					</div>
				</div>
			{/each}
			{#if confirmDiscard}
				<div class="qc-confirm" role="alertdialog" aria-label="Discard this issue?">
					<span>Discard what you typed?</span>
					<span class="spacer"></span>
					<button class="btn ghost" onclick={() => (confirmDiscard = false)}>Keep editing</button>
					<button class="btn danger" onclick={close}>Discard</button>
				</div>
			{/if}
		</div>
		<div class="qc-foot">
			<span class="qc-hint">↵ create · ⌘↵ create and add another · Esc cancel</span>
			<span class="spacer"></span>
			<button class="btn ghost" onclick={tryClose}>Cancel</button>
			<button class="btn primary" onclick={() => create(false)} disabled={saving}>
				{saving ? 'Creating…' : 'Create'}
			</button>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: oklch(0.22 0.01 70 / 0.42);
		z-index: 70;
	}
	.qc-modal {
		position: fixed;
		left: 50%;
		top: 12vh;
		transform: translateX(-50%);
		width: min(640px, 92vw);
		max-height: 80vh;
		overflow-y: auto;
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		z-index: 71;
		display: flex;
		flex-direction: column;
	}
	.qc-head {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 15px 18px 11px;
	}
	.qc-dot {
		width: 9px;
		height: 9px;
		border-radius: var(--r-sm);
		background: var(--st-triage);
		flex: none;
	}
	.qc-title {
		font-weight: 600;
		font-size: var(--t-base);
	}
	.qc-dest {
		font-family: var(--mono);
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.qc-close {
		margin-left: auto;
		background: none;
		border: none;
		color: var(--ink-3);
		width: 24px;
		height: 24px;
		border-radius: var(--r-sm);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.qc-close:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.qc-body {
		padding: 4px 22px 6px;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.qc-input {
		width: 100%;
		box-sizing: border-box;
		background: transparent;
		border: none;
		border-bottom: 1px solid transparent;
		border-radius: 0;
		box-shadow: none;
		outline: none;
		color: var(--ink);
		font-family: inherit;
		font-size: var(--t-lg);
		font-weight: 500;
		padding: 4px 0;
	}
	.qc-input::placeholder {
		color: var(--ink-3);
	}
	.qc-input:focus-visible {
		border-bottom-color: var(--accent);
	}
	.qc-titlebox {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.qc-input.bad {
		border-bottom-color: var(--danger);
	}
	.qc-error {
		font-size: var(--t-sm);
		color: var(--danger);
	}
	.qc-desc {
		font-size: var(--t-base);
	}
	.qc-confirm {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 9px 12px;
		border: 1px solid var(--line-strong);
		border-radius: var(--r);
		background: var(--hover);
		font-size: var(--t-sm);
	}
	.qc-row {
		display: flex;
		gap: 16px;
		flex-wrap: wrap;
	}
	.qc-field {
		display: flex;
		flex-direction: column;
		gap: 6px;
		flex: 1;
		min-width: 160px;
	}
	.qc-field.wide {
		min-width: 100%;
	}
	.qc-field > span {
		font-family: var(--mono);
		font-size: var(--t-xs);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ink-3);
	}
	.qc-pri-row {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.qc-pri-opt {
		display: flex;
		align-items: center;
		gap: 6px;
		height: 28px;
		padding: 0 9px;
		border-radius: var(--r-sm);
		border: 1px solid var(--line);
		background: var(--surface);
		font-size: var(--t-sm);
		color: var(--ink-2);
	}
	.qc-pri-opt.on {
		border-color: var(--accent);
		color: var(--ink);
		background: var(--accent-soft);
	}
	.qc-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 7px;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 26px;
		padding: 0 10px;
		border-radius: 999px;
		border: 1px solid var(--line);
		background: var(--surface);
		font-size: var(--t-sm);
		color: var(--ink-2);
	}
	.chip .dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		flex: none;
	}
	.chip.on {
		border-color: var(--accent);
		color: var(--ink);
		background: var(--accent-soft);
	}
	.qc-foot {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 13px 18px 15px;
		border-top: 1px solid var(--line);
		margin-top: 6px;
	}
	.qc-hint {
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.spacer {
		flex: 1;
	}
	@media (max-width: 720px) {
		.qc-modal {
			left: 0;
			right: 0;
			top: auto;
			bottom: 0;
			transform: none;
			width: 100%;
			max-width: 100%;
			border-radius: var(--r-lg) var(--r-lg) 0 0;
			max-height: 88vh;
		}
	}
</style>
