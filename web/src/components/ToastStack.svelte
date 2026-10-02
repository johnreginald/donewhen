<script>
	// Bottom-left toast stack (PP-209 restyle): ink background, paper text,
	// stacked bottom-up, each with its own 4s timer. Error toasts can carry an
	// action link (e.g. "Retry", "Undo"). The one toast component: the inbox's
	// Approve toast uses it too, through showToast.
	import { toasts, dismissToast } from '$lib/ui.js';
</script>

{#if $toasts.length}
	<div class="toaststack" role="status" aria-live="polite">
		{#each $toasts as t (t.id)}
			<div class="toast" class:err={t.kind === 'error'}>
				{#if t.kind === 'error'}
					<span class="lead">error</span>
				{:else if t.mono}
					<span class="lead">{t.mono}</span>
				{/if}
				<span class="msg">{t.message}</span>
				{#if t.actionLabel}
					<button
						class="retry"
						onclick={() => {
							t.onAction?.();
							dismissToast(t.id);
						}}
					>
						{t.actionLabel}
					</button>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<style>
	.toaststack {
		position: fixed;
		left: 24px;
		bottom: 24px;
		z-index: 90;
		display: flex;
		flex-direction: column-reverse;
		gap: 8px;
		align-items: flex-start;
		max-width: calc(100vw - 48px);
	}
	.toast {
		background: var(--ink);
		color: var(--paper);
		border-radius: var(--r);
		padding: 10px 14px;
		display: inline-flex;
		align-items: center;
		gap: 10px;
		font-size: var(--t-sm);
		box-shadow: var(--shadow-2);
	}
	.msg {
		min-width: 0;
	}
	.lead {
		font-family: var(--mono);
		font-size: var(--t-xs);
		opacity: 0.72;
		flex: none;
	}
	.toast.err .lead {
		color: var(--danger);
		opacity: 1;
	}
	.retry {
		background: none;
		border: none;
		padding: 0;
		color: inherit;
		font: inherit;
		font-weight: 600;
		text-decoration: underline;
		cursor: pointer;
		flex: none;
	}
	@media (max-width: 720px) {
		.toaststack {
			left: 14px;
			right: 14px;
			bottom: calc(66px + env(safe-area-inset-bottom, 0px));
			max-width: none;
		}
		.toast {
			width: 100%;
			box-sizing: border-box;
		}
	}
</style>
