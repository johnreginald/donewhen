<script>
	// /settings on its own has nothing to show: desktop always has a section
	// selected in the left nav, so it goes straight to Account. Mobile has no
	// room for that nav, so this becomes the section list instead (the design's
	// "Settings — nav" screen).
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	const NAV = [
		{ seg: 'account', label: 'Account' },
		{ seg: 'workspace', label: 'Workspace' },
		{ seg: 'members', label: 'Members' },
		{ seg: 'tokens', label: 'API tokens' },
		{ seg: 'labels', label: 'Labels' },
		{ seg: 'notifications', label: 'Notifications' },
		{ seg: 'mcp', label: 'MCP connection help' }
	];

	onMount(() => {
		if (window.matchMedia('(min-width: 721px)').matches) {
			goto('/settings/account', { replaceState: true });
		}
	});
</script>

<nav class="settings-nav-mobile" aria-label="Settings sections">
	{#each NAV as n (n.seg)}
		<a href="/settings/{n.seg}" class="settings-navitem">{n.label}<span class="chev">›</span></a>
	{/each}
</nav>
