<script>
	// The controls above every issue view — search, scope, labels and
	// the Board / List / By epic / Links switch — laid out like Paperclip's Tasks bar.
	import { page } from '$app/stores';
	import { issueQuery } from '$lib/store.js';
	import ProjectSwitcher from './ProjectSwitcher.svelte';
	import LabelFilter from './LabelFilter.svelte';
	import { Search, List, Layers, Columns3, GitFork, ListChecks } from '@lucide/svelte';

	// Board · List · By epic · Tasks · Links (PP-209). The state-grouped List
	// view is PP-213's job; until it exists, "List" points at the same route as
	// "By epic" — the only "list" URL there is today — per the PP-209 spec.
	const views = [
		{ href: '/board', label: 'Board', icon: Columns3 },
		{ href: '/list', label: 'List', icon: List },
		{ href: '/list', label: 'By epic', icon: Layers },
		{ href: '/tasks', label: 'Tasks', icon: ListChecks },
		{ href: '/links', label: 'Links', icon: GitFork }
	];
</script>

<div class="bar">
	<label class="search">
		<Search size={14} strokeWidth={2} />
		<input bind:value={$issueQuery} placeholder="Search tasks…" />
	</label>
	<ProjectSwitcher />
	<LabelFilter />
	<div class="spacer"></div>
	<div class="vtoggle" role="tablist">
		{#each views as v (v.label)}
			{@const Icon = v.icon}
			<a href={v.href} class="vt" class:on={$page.url.pathname === v.href} aria-label={v.label}>
				<Icon size={14} strokeWidth={2} /><span class="vt-txt">{v.label}</span>
			</a>
		{/each}
	</div>
</div>

<style>
	.bar {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 20px;
		flex-wrap: wrap;
		flex-shrink: 0;
	}
	.search {
		display: flex;
		align-items: center;
		gap: 7px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 7px;
		padding: 5px 10px;
		color: var(--ink-3);
		width: min(260px, 100%);
	}
	.search:focus-within {
		border-color: var(--line-strong);
	}
	.search input {
		background: none;
		border: none;
		outline: none;
		font-size: 13px;
		width: 100%;
	}
	.search input::placeholder {
		color: var(--ink-3);
	}
	.spacer {
		flex: 1;
	}
	.vtoggle {
		display: flex;
		gap: 2px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 2px;
	}
	.vt {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 12.5px;
		color: var(--ink-2);
	}
	.vt:hover {
		color: var(--ink);
	}
	.vt.on {
		background: var(--hover);
		color: var(--ink);
	}
	@media (max-width: 720px) {
		.bar {
			padding: 8px 14px;
		}
		.vt-txt {
			display: none;
		}
		.search {
			flex: 1;
		}
	}
</style>
