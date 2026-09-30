<script>
	// The front door is the board.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	onMount(() => goto('/board', { replaceState: true }));
</script>
