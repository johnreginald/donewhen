<script>
	// The workspace list shown by both the sidebar switcher and the header's
	// workspace crumb — one place for "which workspaces, which is active, how
	// many need attention in each". The parent owns the popover positioning.
	import { Check, Settings } from '@lucide/svelte';
	import { workspaces, activeWorkspace, workspaceCounts, switchWorkspace, loadWorkspaceCounts } from '$lib/store.js';
	import { onMount } from 'svelte';

	let { onpick = () => {}, showManage = true } = $props();

	onMount(loadWorkspaceCounts);

	async function pick(slug) {
		if (slug !== $activeWorkspace?.slug) await switchWorkspace(slug);
		onpick();
	}
</script>

<div class="wsmenu" role="menu">
	{#each $workspaces as w, i (w.id)}
		{@const on = w.id === $activeWorkspace?.id}
		{@const count = $workspaceCounts[w.id] || 0}
		<button class="wsitem" class:on role="menuitemradio" aria-checked={on} onclick={() => pick(w.slug)}>
			<span class="k">{w.keyPrefix}</span>
			<span class="nm">{w.name}</span>
			{#if count > 0}<span class="wsbadge">{count}</span>{/if}
			{#if i < 9}<span class="wskbd">⌘{i + 1}</span>{/if}
			{#if on}<Check size={14} strokeWidth={2.4} class="ck" />{/if}
		</button>
	{/each}
	{#if showManage}
		<a href="/settings" class="wsitem manage" role="menuitem" onclick={onpick}>
			<Settings size={13} strokeWidth={2} />Manage workspaces
		</a>
	{/if}
</div>

<style>
	.wsmenu {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 230px;
	}
	.wsitem {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 7px 8px;
		border-radius: var(--r-sm);
		font-size: 13px;
		color: var(--ink-2);
		background: none;
		border: none;
		text-align: left;
		text-decoration: none;
		width: 100%;
		box-sizing: border-box;
		font-family: inherit;
	}
	.wsitem:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.wsitem.on {
		color: var(--ink);
	}
	.k {
		font-family: var(--mono);
		font-size: 10px;
		color: var(--ink-3);
		background: var(--sunken);
		border-radius: 4px;
		padding: 2px 5px;
		min-width: 30px;
		text-align: center;
		flex: none;
	}
	.nm {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.wsbadge {
		font: 600 10px var(--font);
		background: var(--accent);
		color: var(--accent-ink);
		border-radius: 999px;
		padding: 0 5px;
		line-height: 15px;
		flex: none;
	}
	.wskbd {
		font-family: var(--mono);
		font-size: 9.5px;
		color: var(--ink-3);
		border: 1px solid var(--line-strong);
		border-bottom-width: 2px;
		border-radius: 4px;
		padding: 0 4px;
		flex: none;
	}
	.wsitem :global(.ck) {
		color: var(--accent);
		flex: none;
	}
	.wsitem.manage {
		border-top: 1px solid var(--line);
		margin-top: 3px;
		padding-top: 8px;
		color: var(--ink-3);
		font-size: 12px;
	}
</style>
