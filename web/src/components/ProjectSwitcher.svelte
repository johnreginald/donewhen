<script>
	import { initiatives, activeInitiative, activeProject, loadIssues } from '$lib/store.js';
	import { ChevronDown, Check, Layers, Box } from '@lucide/svelte';

	let open = $state(false);
	const cur = $derived($initiatives.find((i) => i.id === $activeInitiative));

	function pick(id) {
		activeInitiative.set(id);
		activeProject.set('');
		loadIssues();
		open = false;
	}
</script>

<button class="psw" onclick={() => (open = true)} aria-label="Switch project">
	<Layers size={15} strokeWidth={2} />
	<span class="psw-name">{cur ? cur.name : 'All Issues'}</span>
	<ChevronDown size={13} strokeWidth={2.4} />
</button>

{#if open}
	<div class="sheet-bd" role="presentation" onclick={() => (open = false)}></div>
	<div class="sheet" role="dialog" aria-label="Switch project">
		<div class="grip"></div>
		<div class="sheet-h">Project</div>
		<button class="item" class:on={!$activeInitiative} onclick={() => pick('')}>
			<Layers size={17} strokeWidth={2} /><span class="nm">All Issues</span>
			{#if !$activeInitiative}<Check size={16} strokeWidth={2.5} />{/if}
		</button>
		{#each $initiatives as i (i.id)}
			<button class="item" class:on={$activeInitiative === i.id} onclick={() => pick(i.id)}>
				<Box size={17} strokeWidth={2} /><span class="nm">{i.name}</span>
				{#if $activeInitiative === i.id}<Check size={16} strokeWidth={2.5} />{/if}
			</button>
		{/each}
	</div>
{/if}

<style>
	.psw {
		display: none;
		align-items: center;
		gap: 6px;
		background: none;
		border: none;
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		padding: 4px 4px;
		max-width: 46vw;
	}
	.psw-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.psw :global(svg:first-child) {
		color: var(--text-dim);
		flex: none;
	}
	.psw :global(svg:last-child) {
		color: var(--text-faint);
		flex: none;
	}

	.sheet-bd {
		position: fixed;
		inset: 0;
		z-index: 60;
		background: rgba(0, 0, 0, 0.45);
	}
	.sheet {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 61;
		background: var(--bg-elev);
		border-top: 1px solid var(--border-strong);
		border-radius: 16px 16px 0 0;
		padding: 8px 10px calc(14px + env(safe-area-inset-bottom, 0px));
		box-shadow: 0 -8px 30px rgba(0, 0, 0, 0.35);
		animation: slideup 0.18s ease;
	}
	@keyframes slideup {
		from {
			transform: translateY(100%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.sheet {
			animation: none;
		}
	}
	.grip {
		width: 36px;
		height: 4px;
		border-radius: 2px;
		background: var(--border-strong);
		margin: 4px auto 8px;
	}
	.sheet-h {
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-faint);
		padding: 4px 12px 8px;
	}
	.item {
		display: flex;
		align-items: center;
		gap: 12px;
		width: 100%;
		background: none;
		border: none;
		color: var(--text-dim);
		text-align: left;
		padding: 13px 12px;
		border-radius: 10px;
		font-size: 15.5px;
	}
	.item .nm {
		flex: 1;
	}
	.item:active {
		background: var(--bg-hover);
	}
	.item.on {
		color: var(--text);
	}
	.item.on :global(svg) {
		color: var(--accent);
	}

	@media (max-width: 720px) {
		.psw {
			display: inline-flex;
		}
	}
</style>
