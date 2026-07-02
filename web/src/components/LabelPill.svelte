<script>
	let { label } = $props();
</script>

<span class="pill">
	<span class="dot" style:background={label.color}></span>
	{label.name}
</span>
