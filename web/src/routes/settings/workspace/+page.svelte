<script>
	import { activeWorkspace, loadWorkspaces, switchWorkspace } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';

	const canAdmin = $derived(['owner', 'admin'].includes($activeWorkspace?.role));

	let renameName = $state('');
	let renamePrefix = $state('');
	let renameAI = $state('');
	let wsErr = $state('');
	let wsSaving = $state(false);
	let aiSaving = $state(false);

	// Keep the rename/AI-name fields in step with whichever workspace is active
	// (switching workspace, or the initial load, both land here).
	let syncedFor = $state(null);
	$effect(() => {
		const w = $activeWorkspace;
		if (!w || syncedFor === w.id) return;
		syncedFor = w.id;
		renameName = w.name;
		renamePrefix = w.keyPrefix;
		renameAI = w.aiName || 'Clanker';
	});

	let newName = $state('');
	let newPrefix = $state('');
	let createErr = $state('');
	let creating = $state(false);
	let justCreated = $state(null); // offer to switch to it

	async function saveWorkspace() {
		wsErr = '';
		wsSaving = true;
		try {
			await api.updateWorkspace($activeWorkspace.id, {
				name: renameName,
				keyPrefix: renamePrefix.toUpperCase()
			});
			await loadWorkspaces();
			showToast('Workspace saved');
		} catch (e) {
			wsErr = e.message || 'Could not save the workspace.';
		} finally {
			wsSaving = false;
		}
	}

	async function saveAIName() {
		aiSaving = true;
		try {
			await api.updateWorkspace($activeWorkspace.id, { aiName: renameAI });
			await loadWorkspaces();
			showToast(`The AI is now called ${renameAI} in ${$activeWorkspace?.name}`);
		} catch (e) {
			showToast(e.message || 'Could not save the AI name.', 'error');
		} finally {
			aiSaving = false;
		}
	}

	async function createWorkspace() {
		createErr = '';
		if (!newName.trim() || !newPrefix.trim()) {
			createErr = 'Name and key prefix are required.';
			return;
		}
		creating = true;
		try {
			const w = await api.createWorkspace({ name: newName.trim(), keyPrefix: newPrefix.toUpperCase() });
			newName = '';
			newPrefix = '';
			await loadWorkspaces();
			justCreated = w;
			showToast(`Created ${w.name} — new issues will be ${w.keyPrefix}-1, ${w.keyPrefix}-2, …`);
		} catch (e) {
			createErr = e.message || 'Could not create the workspace.';
		} finally {
			creating = false;
		}
	}

	// Code review: creating a workspace never switched to it. Offer the switch
	// right where it was just created, instead of leaving the new workspace
	// invisible until the user finds it in the sidebar.
	async function switchToNew() {
		if (!justCreated) return;
		await switchWorkspace(justCreated.slug);
		justCreated = null;
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">Workspace</h1>
		<p class="settings-lede">{$activeWorkspace?.name} · you are <b>{$activeWorkspace?.role}</b>.</p>
	</div>

	{#if canAdmin}
		<section class="settings-panel">
			<div class="settings-panel-h">General</div>
			<div class="settings-field">
				<label for="ws-name">Name</label>
				<input id="ws-name" class="input settings-input-wide" bind:value={renameName} />
			</div>
			<div class="settings-field">
				<label for="ws-slug">Slug</label>
				<input
					id="ws-slug"
					class="input settings-input-wide settings-mono"
					value={$activeWorkspace?.slug || ''}
					disabled
				/>
				<span class="settings-hint">Used in the workspace URL — set once.</span>
			</div>
			<div class="settings-field">
				<label for="ws-prefix">Key prefix</label>
				<input id="ws-prefix" class="input settings-input-short" bind:value={renamePrefix} maxlength="6" />
			</div>
			{#if wsErr}<span class="settings-err">{wsErr}</span>{/if}
			<div class="settings-note">
				Changing the prefix only affects <b>new</b> issues — existing keys stay the same.
			</div>
			<div class="settings-inline">
				<button class="btn primary" onclick={saveWorkspace} disabled={wsSaving}>
					{wsSaving ? 'Saving…' : 'Save'}
				</button>
			</div>
		</section>

		<section class="settings-panel">
			<div class="settings-panel-h">AI name</div>
			<div class="settings-field">
				<label for="ws-ai">What the AI is called here</label>
				<input id="ws-ai" class="input settings-input-wide" maxlength="24" bind:value={renameAI} />
				<span class="settings-hint">
					1–24 characters. Shown in activity, comments, inbox and push notifications.
				</span>
			</div>
			<div class="settings-inline">
				<button class="btn primary" onclick={saveAIName} disabled={aiSaving || !renameAI.trim()}>
					{aiSaving ? 'Saving…' : 'Save'}
				</button>
			</div>
		</section>
	{:else}
		<section class="settings-panel">
			<div class="settings-panel-h">General</div>
			<div class="settings-row"><span>Name</span><span>{$activeWorkspace?.name}</span></div>
			<div class="settings-row"><span>Slug</span><span class="settings-mono">{$activeWorkspace?.slug}</span></div>
			<div class="settings-row">
				<span>Key prefix</span><span class="settings-mono">{$activeWorkspace?.keyPrefix}</span>
			</div>
			<span class="settings-hint">Only an owner or admin can change these.</span>
		</section>
	{/if}

	<section class="settings-panel">
		<div class="settings-panel-h">Create a workspace</div>
		<div class="settings-field">
			<label for="new-ws-name">Name</label>
			<input id="new-ws-name" class="input settings-input-wide" placeholder="e.g. Client Work" bind:value={newName} />
		</div>
		<div class="settings-field">
			<label for="new-ws-prefix">Key prefix</label>
			<input
				id="new-ws-prefix"
				class="input settings-input-short"
				class:errb={!!createErr}
				placeholder="PREFIX"
				bind:value={newPrefix}
				maxlength="6"
			/>
			{#if createErr}<span class="settings-err">{createErr}</span>{/if}
		</div>
		<div class="settings-inline">
			<button class="btn primary" onclick={createWorkspace} disabled={creating}>
				{creating ? 'Creating…' : 'Create'}
			</button>
		</div>
		{#if justCreated}
			<div class="settings-note">
				<span>Created {justCreated.name}.</span>
				<button class="btn settings-btn-sm" onclick={switchToNew}>Switch to it</button>
			</div>
		{/if}
	</section>
</div>
