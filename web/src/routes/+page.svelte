<script>
	// The front door. Tasks until the dashboard exists.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	onMount(() => goto('/tasks', { replaceState: true }));
</script>
