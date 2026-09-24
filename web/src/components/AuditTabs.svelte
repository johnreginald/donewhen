<script>
	// Paperclip's Audit: what happened, what ran, what it cost, and the caps.
	import { page } from '$app/stores';
	const TABS = [
		{ href: '/log', label: 'Activity' },
		{ href: '/audit/runs', label: 'Runs' },
		{ href: '/audit/costs', label: 'Costs' },
		{ href: '/audit/budgets', label: 'Budgets' }
	];
</script>

<nav class="tabs" aria-label="Audit">
	{#each TABS as t (t.href)}
		<a href={t.href} class="tab" class:on={$page.url.pathname === t.href}>{t.label}</a>
	{/each}
</nav>

<style>
	.tabs {
		display: flex;
		gap: 4px;
		padding: 10px 20px 0;
		border-bottom: 1px solid var(--border);
	}
	.tab {
		font-size: 13px;
		color: var(--text-dim);
		padding: 6px 10px 8px;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
	}
	.tab:hover {
		color: var(--text);
	}
	.tab.on {
		color: var(--text);
		border-bottom-color: var(--accent);
	}
</style>
