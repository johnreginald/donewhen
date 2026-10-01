<script>
	// "C" — a one-line quick-add that lands in Triage. Epic, a single "type"
	// label and priority are optional extras, not a full issue form (that's the
	// issue detail page's job).
	import { api } from '$lib/api.js';
	import { projects, typeLabels, loadIssues, PRIORITIES } from '$lib/store.js';
	import { quickCapture, showToast } from '$lib/ui.js';
	import { X } from '@lucide/svelte';
	import EpicMenu from './EpicMenu.svelte';
	import PriorityIcon from './PriorityIcon.svelte';

	let title = $state('');
	let epicId = $state('');
	let labelId = $state('');
	let priority = $state(0);
	let saving = $state(false);
	let titleEl = $state(null);
	let modalEl = $state(null);
	let lastFocused = null;

	$effect(() => {
		if ($quickCapture) {
			lastFocused = document.activeElement;
			title = '';
			epicId = '';
			labelId = '';
			priority = 0;
			saving = false;
			queueMicrotask(() => titleEl && titleEl.focus());
		} else if (lastFocused) {
			lastFocused.focus?.();
			lastFocused = null;
		}
	});

	function close() {
		quickCapture.set(false);
	}

	async function create() {
		if (saving || !title.trim()) return;
		saving = true;
		try {
			const is = await api.createIssue({
				title: title.trim(),
				projectId: epicId || undefined,
				priority,
				labelIds: labelId ? [labelId] : []
			});
			showToast(`${is.key} created`);
			close();
			loadIssues();
		} catch (e) {
			showToast(e.message || 'Failed to create', 'error');
		} finally {
			saving = false;
		}
	}

	function pickLabel(id) {
		labelId = labelId === id ? '' : id;
	}

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
			close();
		} else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			create();
		} else {
			trapTab(e);
		}
	}
</script>

{#if $quickCapture}
	<div class="backdrop" role="presentation" onclick={close}></div>
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
			<button class="qc-close" onclick={close} aria-label="Close"><X size={15} strokeWidth={2} /></button>
		</div>
		<div class="qc-body">
			<input
				bind:this={titleEl}
				bind:value={title}
				class="qc-input"
				placeholder="Issue title"
				maxlength="200"
			/>
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
			{#if $typeLabels.length}
				<div class="qc-field wide">
					<span id="qc-label-label">Label</span>
					<div class="qc-chips" role="radiogroup" aria-labelledby="qc-label-label">
						{#each $typeLabels as l (l.id)}
							<button
								type="button"
								class="chip"
								class:on={labelId === l.id}
								role="radio"
								aria-checked={labelId === l.id}
								onclick={() => pickLabel(l.id)}
							>
								<span class="dot" style:background={l.color}></span>{l.name}
							</button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
		<div class="qc-foot">
			<span class="qc-hint">⌘↵ to save · Esc to cancel</span>
			<span class="spacer"></span>
			<button class="btn ghost" onclick={close}>Cancel</button>
			<button class="btn primary" onclick={create} disabled={saving || !title.trim()}>
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
		border-radius: 3px;
		background: var(--st-triage);
		flex: none;
	}
	.qc-title {
		font-weight: 600;
		font-size: 14px;
	}
	.qc-dest {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--ink-3);
	}
	.qc-close {
		margin-left: auto;
		background: none;
		border: none;
		color: var(--ink-3);
		width: 24px;
		height: 24px;
		border-radius: 6px;
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
		border: none;
		outline: none;
		background: none;
		font-size: 19px;
		font-weight: 500;
		color: var(--ink);
		padding: 4px 0;
		font-family: var(--font);
	}
	.qc-input::placeholder {
		color: var(--ink-3);
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
		font-size: 10px;
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
		font-size: 12px;
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
		font-size: 12.5px;
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
		font-size: 11px;
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
