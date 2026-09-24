<script>
	// The front door is the dashboard, as in Paperclip.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	onMount(() => goto('/dashboard', { replaceState: true }));
</script>
