<script>
	// The inbox's own toast — ink background, paper text, bottom-left, with an
	// optional Undo action. Separate from lib/ui.js's shared toast (surface bg,
	// bottom-center, no action slot) because Approve needs an Undo button.
	// Self-contained: the parent just sets/clears `toast`.
	let { toast = null } = $props();
</script>

{#if toast}
	<div class="ib-toast" class:error={toast.kind === 'error'} role="status">
		{#if toast.mono}<span class="mono">{toast.mono}</span>{/if}
		<span class="msg">{toast.message}</span>
		{#if toast.onUndo}
			<button class="undo" onclick={toast.onUndo}>Undo</button>
		{/if}
	</div>
{/if}

<style>
	.ib-toast {
		position: fixed;
		left: 20px;
		bottom: 20px;
		z-index: 85;
		background: var(--ink);
		color: var(--paper);
		border-radius: var(--r);
		padding: 10px 14px;
		display: inline-flex;
		align-items: center;
		gap: 10px;
		font-size: var(--t-sm);
		box-shadow: var(--shadow-2);
		max-width: calc(100vw - 40px);
	}
	.ib-toast.error {
		background: var(--danger);
		color: var(--danger-soft);
	}
	.mono {
		font-family: var(--mono);
		opacity: 0.72;
	}
	.msg {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.undo {
		color: inherit;
		text-decoration: underline;
		font-weight: 600;
		background: none;
		border: none;
		padding: 0;
		font-size: inherit;
		cursor: pointer;
		font-family: var(--font);
		flex: none;
	}
	@media (max-width: 720px) {
		.ib-toast {
			left: 12px;
			right: 12px;
			bottom: calc(66px + env(safe-area-inset-bottom, 0px));
			max-width: none;
		}
	}
</style>
