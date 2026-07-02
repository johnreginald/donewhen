<script>
	// Deep-link target (e.g. from a push notification): open the issue drawer
	// over the board.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { openIssue } from '$lib/ui.js';

	onMount(() => {
		const key = $page.params.key;
		goto('/', { replaceState: true }).then(() => {
			if (key) openIssue(key);
		});
	});
</script>

<div class="loading">Opening {$page.params.key}…</div>

<style>
	.loading {
		display: grid;
		place-items: center;
		height: 100%;
		color: var(--text-dim);
	}
</style>
