<script>
	import { workspaces } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';
	import { copyToClipboard } from '../clipboard.js';

	let tokens = $state([]);
	let loading = $state(true);
	let name = $state('');
	let wsPin = $state('');
	let creating = $state(false);
	let secret = $state('');
	let confirmId = $state('');

	async function load() {
		loading = true;
		try {
			tokens = (await api.tokens()) || [];
		} catch (e) {
			showToast('Could not load tokens: ' + (e?.message || e), 'error');
			tokens = [];
		} finally {
			loading = false;
		}
	}
	load();

	async function createToken() {
		creating = true;
		try {
			const res = await api.createToken(name.trim() || 'token', wsPin);
			// Shown exactly once — kept only in this page's own state, so it
			// disappears the moment the user navigates away.
			secret = res.secret;
			name = '';
			wsPin = '';
			await load();
		} catch (e) {
			showToast('Could not create token: ' + (e?.message || e), 'error');
		} finally {
			creating = false;
		}
	}

	async function copySecret() {
		if (await copyToClipboard(secret)) showToast('Copied');
		else showToast('Could not copy — select and copy manually.', 'error');
	}

	// Code review: revoke had no confirmation and no try/catch. Two clicks
	// (arm, then confirm), and feedback either way.
	async function delToken(id) {
		if (confirmId !== id) {
			confirmId = id;
			return;
		}
		confirmId = '';
		try {
			await api.deleteToken(id);
			await load();
			showToast('Token revoked');
		} catch (e) {
			showToast('Could not revoke token: ' + (e?.message || e), 'error');
		}
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">API tokens</h1>
		<p class="settings-lede">
			Scoped to your account. Pin a token to one workspace to keep an agent inside one repo's
			tracker.
		</p>
	</div>

	<section class="settings-panel">
		<div class="settings-panel-h">Create a token</div>
		<div class="settings-inline">
			<input class="input" style="width:200px" placeholder="token name (e.g. claude)" bind:value={name} />
			<select class="select" style="width:170px" bind:value={wsPin}>
				<option value="">All workspaces</option>
				{#each $workspaces as w (w.id)}
					<option value={w.slug}>{w.name}</option>
				{/each}
			</select>
			<button class="btn primary" onclick={createToken} disabled={creating}>
				{creating ? 'Creating…' : 'Create'}
			</button>
		</div>
	</section>

	{#if secret}
		<div class="settings-secret">
			<b>Copy now — you won't see this again</b>
			<div class="code settings-mono">{secret}</div>
			<div class="settings-inline">
				<button class="btn settings-btn-sm" onclick={copySecret}>Copy</button>
				<button class="btn ghost settings-btn-sm" onclick={() => (secret = '')}>Dismiss</button>
			</div>
		</div>
	{/if}

	<section class="settings-panel">
		<div class="settings-panel-h">Tokens<span class="n">{tokens.length}</span></div>
		{#if loading}
			<p class="settings-hint">Loading…</p>
		{:else}
			{#each tokens as t (t.id)}
				<div class="settings-row">
					<div>
						<div>{t.name}</div>
						<div class="settings-hint settings-mono">
							{t.workspaceName || 'all workspaces'} · created {new Date(t.createdAt).toLocaleDateString()}
							{#if t.lastUsedAt}
								· last used {new Date(t.lastUsedAt).toLocaleDateString()}
							{:else}
								· never used
							{/if}
						</div>
					</div>
					{#if confirmId === t.id}
						<span class="settings-confirm">
							Revoke this token?
							<button class="btn danger settings-btn-sm" onclick={() => delToken(t.id)}>Confirm</button>
							<button class="btn ghost settings-btn-sm" onclick={() => (confirmId = '')}>Cancel</button>
						</span>
					{:else}
						<button class="btn ghost settings-btn-sm" onclick={() => delToken(t.id)}>Revoke</button>
					{/if}
				</div>
			{:else}
				<p class="settings-hint">No tokens yet.</p>
			{/each}
		{/if}
	</section>
</div>
