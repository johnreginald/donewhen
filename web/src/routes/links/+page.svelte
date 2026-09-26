<script>
	import DependencyGraph from '$components/DependencyGraph.svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import IssuesToolbar from '$components/IssuesToolbar.svelte';
</script>

<div class="page">
	<PageHeader crumbs={[{ label: 'Tasks', href: '/board' }, { label: 'Links' }]} />
	<IssuesToolbar />
	<div class="fill"><DependencyGraph /></div>
</div>

<style>
	.page {
		height: 100%;
		display: flex;
		flex-direction: column;
	}
	.fill {
		flex: 1;
		min-height: 0;
	}
</style>
