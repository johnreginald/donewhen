<script>
	// The bar at the top of every page, as Paperclip draws it: a small
	// uppercase breadcrumb on the left, the page's own controls on the right.
	import { navOpen } from '$lib/ui.js';
	import { Menu, ChevronRight } from '@lucide/svelte';

	// crumbs: [{ label, href? }] — the last one is where you are.
	let { crumbs = [], children } = $props();
</script>

<header class="ph">
	<button class="menu" onclick={() => navOpen.set(true)} aria-label="Open navigation">
		<Menu size={18} strokeWidth={2} />
	</button>
	<nav class="crumbs" aria-label="Breadcrumb">
		{#each crumbs as c, i}
			{#if i > 0}<ChevronRight size={13} strokeWidth={2} class="sep" />{/if}
			{#if c.href && i < crumbs.length - 1}
				<a href={c.href} class="crumb link">{c.label}</a>
			{:else}
				<span class="crumb" class:here={i === crumbs.length - 1} class:upper={c.upper !== false}>{c.label}</span>
			{/if}
		{/each}
	</nav>
	<div class="right">
		{@render children?.()}
	</div>
</header>

<style>
	.ph {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 48px;
		padding: 8px 20px;
		padding-top: calc(8px + env(safe-area-inset-top, 0px));
		border-bottom: 1px solid var(--border);
		background: var(--bg);
		flex-shrink: 0;
	}
	.menu {
		display: none;
		background: none;
		border: none;
		color: var(--text-dim);
		padding: 4px;
		margin-left: -6px;
		border-radius: 6px;
	}
	.crumbs {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
		color: var(--text-faint);
	}
	.crumbs :global(.sep) {
		flex-shrink: 0;
	}
	.crumb {
		font-size: 12px;
		letter-spacing: 0.06em;
		color: var(--text-dim);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.crumb.upper {
		text-transform: uppercase;
		font-weight: 500;
	}
	.crumb:not(.upper) {
		font-size: 13.5px;
		letter-spacing: 0;
	}
	.crumb.here {
		color: var(--text);
	}
	.crumb.link {
		text-transform: uppercase;
		font-weight: 500;
	}
	.crumb.link:hover {
		color: var(--text);
	}
	.right {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: 8px;
	}
	@media (max-width: 720px) {
		.menu {
			display: inline-flex;
		}
		.ph {
			padding-left: 14px;
			padding-right: 14px;
		}
	}
</style>
