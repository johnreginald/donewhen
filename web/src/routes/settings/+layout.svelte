<script>
	// The settings shell: a left sub-nav local to /settings/*, plus the shared
	// PageHeader. The sub-nav lives here, not in the global Sidebar (PP-209 owns
	// that) — see PP-215.
	import PageHeader from '$components/PageHeader.svelte';
	import { page } from '$app/stores';

	let { children } = $props();

	const NAV = [
		{ seg: 'account', label: 'Account' },
		{ seg: 'workspace', label: 'Workspace' },
		{ seg: 'members', label: 'Members' },
		{ seg: 'tokens', label: 'API tokens' },
		{ seg: 'labels', label: 'Labels' },
		{ seg: 'notifications', label: 'Notifications' },
		{ seg: 'mcp', label: 'MCP connection help' }
	];

	const current = $derived($page.url.pathname.split('/')[2] || '');
	const isIndex = $derived(!current);
	const currentLabel = $derived(NAV.find((n) => n.seg === current)?.label || '');
	const crumbs = $derived(
		isIndex
			? [{ label: 'Settings' }]
			: [{ label: 'Settings', href: '/settings' }, { label: currentLabel }]
	);
</script>

<div class="pg">
	<PageHeader {crumbs} />
	<div class="pg-body">
		<div class="settings-shell">
			<nav class="settings-nav" aria-label="Settings sections">
				{#each NAV as n (n.seg)}
					<a href="/settings/{n.seg}" class="settings-navitem" class:on={current === n.seg}>{n.label}</a>
				{/each}
			</nav>
			<div class="settings-main">
				{@render children?.()}
			</div>
		</div>
	</div>
</div>
