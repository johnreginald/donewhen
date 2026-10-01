<script>
	// Bottom-left toast stack (PP-209 restyle): ink background, paper text,
	// stacked bottom-up, each with its own 4s timer. Error toasts can carry an
	// action link (e.g. "Retry").
	import { toasts, dismissToast } from '$lib/ui.js';
</script>

{#if $toasts.length}
	<div class="toaststack" role="status" aria-live="polite">
		{#each $toasts as t (t.id)}
			<div class="toast" class:err={t.kind === 'error'}>
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
		z-index: 80;
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
		font-size: 13px;
		box-shadow: var(--shadow-2);
	}
	.msg {
		min-width: 0;
	}
	.retry {
		background: none;
		border: 1px solid oklch(1 0 0 / 0.3);
		color: var(--paper);
		border-radius: 5px;
		height: 22px;
		padding: 0 9px;
		font-size: 11.5px;
		flex: none;
	}
	.retry:hover {
		background: oklch(1 0 0 / 0.12);
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
