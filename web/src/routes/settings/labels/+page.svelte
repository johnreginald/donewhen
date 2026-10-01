<script>
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';

	// Matches the exclusive-group taxonomy in AGENTS.md: these sort first, in
	// this order; any other group the workspace has follows, alphabetically.
	const CANON_ORDER = ['repo', 'platform', 'type', 'domain', 'triage'];
	const SWATCHES = ['#38BDF8', '#A78BFA', '#34D399', '#FBBF24', '#F87171', '#FB923C', '#8A8F9C', '#6B7280'];

	let groups = $state([]);
	let labels = $state([]);
	let loading = $state(true);

	let newGroup = $state('');
	let newName = $state('');
	let newColor = $state(SWATCHES[0]);
	let adding = $state(false);
	let addErr = $state('');

	async function load() {
		loading = true;
		try {
			const [g, l] = await Promise.all([api.labelGroups(), api.labels()]);
			groups = g || [];
			labels = l || [];
		} catch (e) {
			showToast('Could not load labels: ' + (e?.message || e), 'error');
		} finally {
			loading = false;
		}
	}
	load();

	const groupRows = $derived.by(() => {
		const byId = new Map(groups.map((g) => [g.id, g]));
		const byName = new Map();
		for (const l of labels) {
			const g = l.groupId ? byId.get(l.groupId) : null;
			const gname = g?.name || 'ungrouped';
			if (!byName.has(gname)) byName.set(gname, { name: gname, exclusive: g?.exclusive ?? false, items: [] });
			byName.get(gname).items.push(l);
		}
		// Include a known group even with zero labels yet, so "+ Add label" is
		// reachable for it.
		for (const g of groups) if (!byName.has(g.name)) byName.set(g.name, { name: g.name, exclusive: g.exclusive, items: [] });
		const rows = [...byName.values()];
		rows.sort((a, b) => {
			const ai = CANON_ORDER.indexOf(a.name);
			const bi = CANON_ORDER.indexOf(b.name);
			if (ai !== -1 || bi !== -1) return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi);
			return a.name.localeCompare(b.name);
		});
		return rows;
	});

	const groupNames = $derived([...new Set([...CANON_ORDER, ...groups.map((g) => g.name)])]);

	function startAdd(groupName) {
		newGroup = groupName;
		document.getElementById('label-name-input')?.focus();
	}

	async function addLabel() {
		addErr = '';
		if (!newName.trim()) {
			addErr = 'Label name is required.';
			return;
		}
		adding = true;
		try {
			await api.createLabel({ name: newName.trim(), color: newColor, group: newGroup || undefined });
			newName = '';
			await load();
			showToast('Label added');
		} catch (e) {
			addErr = e.message || 'Could not add label.';
		} finally {
			adding = false;
		}
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">Labels</h1>
		<p class="settings-lede">Exclusive groups — one label per group, per issue.</p>
	</div>

	{#if loading}
		<p class="settings-hint">Loading…</p>
	{:else}
		{#each groupRows as row (row.name)}
			<section class="settings-panel">
				<div class="settings-panel-h">
					{row.name}{#if row.exclusive}<span class="n">exclusive</span>{/if}
				</div>
				<div class="settings-inline">
					{#each row.items as l (l.id)}
						<span class="pill"><span class="dot" style="background:{l.color}"></span>{l.name}</span>
					{:else}
						<span class="settings-hint">No labels yet.</span>
					{/each}
					<button class="btn ghost settings-btn-sm" onclick={() => startAdd(row.name)}>+ Add label</button>
				</div>
			</section>
		{/each}
	{/if}

	<section class="settings-panel">
		<div class="settings-panel-h">Add a label</div>
		<div class="settings-inline">
			<select class="select settings-input-short" bind:value={newGroup}>
				<option value="">(no group)</option>
				{#each groupNames as g (g)}
					<option value={g}>{g}</option>
				{/each}
			</select>
			<input id="label-name-input" class="input" style="width:180px" placeholder="label name" bind:value={newName} />
			<div class="settings-swatches">
				{#each SWATCHES as hex (hex)}
					<button
						type="button"
						class="settings-swatch"
						class:on={newColor === hex}
						style="background:{hex}"
						aria-label={`Use ${hex}`}
						onclick={() => (newColor = hex)}
					></button>
				{/each}
			</div>
			<button class="btn primary" onclick={addLabel} disabled={adding}>{adding ? 'Adding…' : 'Add'}</button>
		</div>
		{#if addErr}<span class="settings-err">{addErr}</span>{/if}
	</section>
</div>
