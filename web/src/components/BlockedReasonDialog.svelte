<script>
	// Asks why an issue is being moved to Blocked. Opened through
	// askBlockedReason() in ui.js, which resolves with the reason, or null when
	// the user backs out.
	import { blockPrompt } from '$lib/ui.js';
	import { checkReason, REASON_MAX } from '$lib/blocked.js';

	let reason = $state('');
	let error = $state('');
	let inputEl = $state(null);
	let modalEl = $state(null);
	let lastFocused = null;

	$effect(() => {
		if ($blockPrompt) {
			lastFocused = document.activeElement;
			reason = '';
			error = '';
			queueMicrotask(() => inputEl && inputEl.focus());
		} else if (lastFocused) {
			lastFocused.focus?.();
			lastFocused = null;
		}
	});

	function finish(value) {
		const p = $blockPrompt;
		blockPrompt.set(null);
		p?.resolve(value);
	}

	function submit() {
		const v = checkReason(reason);
		if (v.error) {
			error = v.error;
			inputEl?.focus();
			return;
		}
		finish(v.reason);
	}

	// Keep Tab cycling inside the dialog instead of leaking to the page behind it.
	function trapTab(e) {
		if (e.key !== 'Tab' || !modalEl) return;
		const list = Array.from(modalEl.querySelectorAll('button, textarea')).filter((el) => !el.disabled);
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
			finish(null);
		} else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey || e.target === inputEl) && !e.shiftKey) {
			e.preventDefault();
			submit();
		} else {
			trapTab(e);
		}
	}
</script>

{#if $blockPrompt}
	<div class="backdrop" role="presentation" onclick={() => finish(null)}></div>
	<div
		class="br-modal"
		bind:this={modalEl}
		role="dialog"
		aria-modal="true"
		aria-label="Why is it blocked?"
		tabindex="-1"
		onkeydown={onKey}
	>
		<div class="br-head">
			<span class="br-title">Why is {$blockPrompt.label} blocked?</span>
		</div>
		<div class="br-body">
			<textarea
				bind:this={inputEl}
				bind:value={reason}
				oninput={() => (error = '')}
				class="textarea"
				class:bad={!!error}
				rows="3"
				maxlength={REASON_MAX}
				placeholder="What is it waiting on?"
				aria-label="Reason"
				aria-invalid={!!error}
				aria-describedby={error ? 'br-error' : undefined}
			></textarea>
			{#if error}<div id="br-error" class="br-error" role="alert">{error}</div>{/if}
		</div>
		<div class="br-foot">
			<span class="br-hint">↵ block · Esc cancel</span>
			<span class="spacer"></span>
			<button class="btn ghost" onclick={() => finish(null)}>Cancel</button>
			<button class="btn primary" onclick={submit}>Block</button>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: oklch(0.22 0.01 70 / 0.42);
		z-index: 80;
	}
	.br-modal {
		position: fixed;
		left: 50%;
		top: 18vh;
		transform: translateX(-50%);
		width: min(480px, 92vw);
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		z-index: 81;
		display: flex;
		flex-direction: column;
	}
	.br-head {
		padding: 15px 18px 8px;
	}
	.br-title {
		font-weight: 600;
		font-size: var(--t-base);
	}
	.br-body {
		padding: 4px 18px 6px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.textarea.bad {
		border-color: var(--danger);
	}
	.br-error {
		font-size: var(--t-sm);
		color: var(--danger);
	}
	.br-foot {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 13px 18px 15px;
		border-top: 1px solid var(--line);
		margin-top: 6px;
	}
	.br-hint {
		font-size: var(--t-xs);
		color: var(--ink-3);
	}
	.spacer {
		flex: 1;
	}
</style>
