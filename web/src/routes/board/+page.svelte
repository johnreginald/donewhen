<script>
	import Board from '$components/Board.svelte';
	import PageHeader from '$components/PageHeader.svelte';
	import IssuesToolbar from '$components/IssuesToolbar.svelte';
</script>

<div class="page">
	<PageHeader crumbs={[{ label: 'Tasks', href: '/tasks' }, { label: 'Board' }]} />
	<IssuesToolbar />
	<div class="fill"><Board /></div>
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
