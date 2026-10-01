<script>
	// "?" — the keyboard reference. Static content; nothing to load.
	import { shortcutHelp } from '$lib/ui.js';
	import { X } from '@lucide/svelte';

	let closeEl = $state(null);
	let lastFocused = null;

	$effect(() => {
		if ($shortcutHelp) {
			lastFocused = document.activeElement;
			queueMicrotask(() => closeEl && closeEl.focus());
		} else if (lastFocused) {
			lastFocused.focus?.();
			lastFocused = null;
		}
	});

	function close() {
		shortcutHelp.set(false);
	}

	function onKey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			close();
		} else if (e.key === 'Tab') {
			// The close button is the dialog's only focusable element.
			e.preventDefault();
			closeEl?.focus();
		}
	}
</script>

{#if $shortcutHelp}
	<div class="backdrop" role="presentation" onclick={close}></div>
	<div class="sk-modal" role="dialog" aria-modal="true" aria-label="Keyboard shortcuts" tabindex="-1" onkeydown={onKey}>
		<div class="sk-head">
			<span class="sk-title">Keyboard shortcuts</span>
			<button bind:this={closeEl} class="qc-close" onclick={close} aria-label="Close">
				<X size={15} strokeWidth={2} />
			</button>
		</div>
		<div class="sk-groups">
			<div>
				<div class="sk-gtitle">Global</div>
				<div class="sk-row"><kbd class="kbd">⌘K</kbd><span class="sk-desc">Search &amp; command palette</span></div>
				<div class="sk-row"><kbd class="kbd">Ctrl</kbd><kbd class="kbd">1–9</kbd><span class="sk-desc">Switch workspace</span></div>
				<div class="sk-row"><kbd class="kbd">C</kbd><span class="sk-desc">New issue (quick capture)</span></div>
				<div class="sk-row"><kbd class="kbd">?</kbd><span class="sk-desc">This help</span></div>
			</div>
			<div>
				<div class="sk-gtitle">In a list</div>
				<div class="sk-row"><kbd class="kbd">↑</kbd><kbd class="kbd">↓</kbd><span class="sk-desc">Move selection</span></div>
				<div class="sk-row"><kbd class="kbd">↵</kbd><span class="sk-desc">Open selected</span></div>
				<div class="sk-row"><kbd class="kbd">Esc</kbd><span class="sk-desc">Close</span></div>
			</div>
			<div>
				<div class="sk-gtitle">In a form</div>
				<div class="sk-row"><kbd class="kbd">⌘↵</kbd><span class="sk-desc">Save</span></div>
				<div class="sk-row"><kbd class="kbd">Esc</kbd><span class="sk-desc">Cancel</span></div>
			</div>
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
	.sk-modal {
		position: fixed;
		left: 50%;
		top: 10vh;
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
	.sk-head {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 15px 18px 11px;
	}
	.sk-title {
		font-weight: 600;
		font-size: 14px;
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
	.sk-groups {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 22px;
		padding: 6px 22px 20px;
	}
	.sk-gtitle {
		font-family: var(--mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ink-3);
		margin-bottom: 10px;
	}
	.sk-row {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 6px 0;
		border-bottom: 1px dashed var(--line);
	}
	.sk-row:last-child {
		border-bottom: none;
	}
	.kbd {
		font-family: var(--mono);
		font-size: 11px;
		padding: 2px 6px;
		border: 1px solid var(--line-strong);
		border-bottom-width: 2px;
		border-radius: 5px;
		color: var(--ink-2);
		background: var(--surface);
	}
	.sk-desc {
		font-size: 12.5px;
		color: var(--ink-2);
		margin-left: 4px;
	}
	@media (max-width: 600px) {
		.sk-groups {
			grid-template-columns: 1fr;
			gap: 14px;
		}
	}
</style>
