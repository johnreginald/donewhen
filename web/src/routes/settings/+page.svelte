<script>
	import PageHeader from '$components/PageHeader.svelte';
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { appConfig, workspaces, activeWorkspace, loadWorkspaces, switchWorkspace } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { theme, setTheme } from '$lib/theme.js';
	import { pushSupported, enablePush, disablePush, currentSubscription } from '$lib/push.js';

	let tokens = $state([]);
	let members = $state([]);
	let newTokenName = $state('');
	let newTokenWs = $state('');
	// workspace admin
	let wsName = $state('');
	let wsPrefix = $state('');
	let renameName = $state('');
	let renamePrefix = $state('');
	let renameAI = $state('');
	let memberEmail = $state('');
	let memberRole = $state('member');
	const canAdmin = $derived(['owner', 'admin'].includes($activeWorkspace?.role));
	let createdSecret = $state('');
	let pushOn = $state(false);
	let pushBusy = $state(false);
	let pushPerm = $state('');
	let pushSupp = $state(true);
	let pushErr = $state('');

	function refreshPushState() {
		try {
			pushSupp = pushSupported();
			pushPerm = typeof Notification !== 'undefined' ? Notification.permission : 'n/a';
		} catch {
			/* ignore */
		}
	}

	onMount(async () => {
		tokens = (await api.tokens()) || [];
		await refreshMembers();
		renameName = $activeWorkspace?.name || '';
		renamePrefix = $activeWorkspace?.keyPrefix || '';
		renameAI = $activeWorkspace?.aiName || 'Clanker';
		pushOn = !!(await currentSubscription());
		refreshPushState();
	});

	async function refreshMembers() {
		if (!$activeWorkspace) return;
		try {
			members = (await api.members($activeWorkspace.id)) || [];
		} catch {
			members = [];
		}
	}

	async function createWorkspace() {
		try {
			const w = await api.createWorkspace({ name: wsName, keyPrefix: wsPrefix.toUpperCase() });
			wsName = '';
			wsPrefix = '';
			await loadWorkspaces();
			showToast(`Created ${w.name} — new issues will be ${w.keyPrefix}-1, ${w.keyPrefix}-2, …`);
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	async function saveWorkspace() {
		try {
			await api.updateWorkspace($activeWorkspace.id, {
				name: renameName,
				keyPrefix: renamePrefix.toUpperCase()
			});
			await loadWorkspaces();
			showToast('Workspace updated');
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	async function saveAIName() {
		try {
			await api.updateWorkspace($activeWorkspace.id, { aiName: renameAI });
			await loadWorkspaces();
			renameAI = $activeWorkspace?.aiName || renameAI;
			showToast(`The AI is now called ${renameAI} in ${$activeWorkspace?.name}`);
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	async function addMember() {
		try {
			await api.addMember($activeWorkspace.id, { email: memberEmail, role: memberRole });
			memberEmail = '';
			await refreshMembers();
			showToast('Member added');
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	async function removeMember(userId) {
		try {
			await api.removeMember($activeWorkspace.id, userId);
			await refreshMembers();
		} catch (e) {
			showToast(e.message, 'error');
		}
	}

	async function createToken() {
		try {
			const res = await api.createToken(newTokenName || 'token', newTokenWs);
			createdSecret = res.secret;
			newTokenName = '';
			tokens = (await api.tokens()) || [];
		} catch (e) {
			showToast(e.message, 'error');
		}
	}
	async function delToken(id) {
		await api.deleteToken(id);
		tokens = (await api.tokens()) || [];
	}
	async function togglePush() {
		pushBusy = true;
		try {
			if (pushOn) {
				await disablePush();
				pushOn = false;
				showToast('Push disabled');
			} else {
				const r = await enablePush($appConfig.vapidPublicKey);
				if (r.ok) {
					pushOn = true;
					pushErr = '';
					showToast('Push enabled');
				} else {
					pushErr = r.reason || 'unknown';
					if (r.reason === 'blocked') {
						showToast('Notifications are blocked for this site. Allow them in the browser menu (site permissions), then retry.', 'error');
					} else if (r.reason === 'unsupported') {
						showToast('This browser can’t do Web Push here.', 'error');
					} else {
						showToast('Push failed: ' + r.reason, 'error');
					}
				}
			}
		} catch (e) {
			pushErr = e?.message || String(e);
			showToast('Push failed: ' + pushErr, 'error');
		} finally {
			refreshPushState();
			pushBusy = false;
		}
	}

	const mcpUrl = $derived(($appConfig.baseUrl || '') + '/mcp');
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Settings' }]} />
	<div class="pg-body">
<div class="settings">
	<h1>Settings</h1>

	<section>
		<h2>Appearance</h2>
		<div class="row">
			<span>Theme</span>
			<div class="seg" role="radiogroup" aria-label="Theme">
				{#each [['system', 'System'], ['light', 'Light'], ['dark', 'Dark']] as [v, label] (v)}
					<button
						class="sg"
						class:on={$theme === v}
						role="radio"
						aria-checked={$theme === v}
						onclick={() => setTheme(v)}>{label}</button
					>
				{/each}
			</div>
		</div>
		<p class="faint hint">Saved on this device. System follows your OS light or dark setting.</p>
	</section>

	<section>
		<h2>Workspaces</h2>
		<p class="faint">
			A workspace is the boundary: issues, epics, labels, board columns, artifacts and the
			activity log all belong to exactly one, and only its members can see them.
		</p>
		<ul class="tokens">
			{#each $workspaces as w (w.id)}
				<li>
					<span>
						{w.name}
						{#if w.id === $activeWorkspace?.id}<em class="faint">— active</em>{/if}
					</span>
					<span class="mono faint">{w.keyPrefix}</span>
					<span class="faint">{w.role}</span>
					{#if w.id !== $activeWorkspace?.id}
						<button class="btn ghost sm" onclick={() => switchWorkspace(w.slug)}>switch</button>
					{/if}
				</li>
			{/each}
		</ul>

		{#if canAdmin}
			<h3>Rename the active workspace</h3>
			<div class="row">
				<input class="input" placeholder="name" bind:value={renameName} />
				<input class="input short" placeholder="PREFIX" bind:value={renamePrefix} />
				<button class="btn" onclick={saveWorkspace}>Save</button>
			</div>
			<p class="faint hint">
				The prefix applies to <b>new</b> issues only — existing keys never change.
			</p>

			<h3>AI name</h3>
			<div class="row">
				<input class="input" maxlength="24" placeholder="Clanker" bind:value={renameAI} />
				<button class="btn" onclick={saveAIName} disabled={!renameAI.trim()}>Save</button>
			</div>
			<p class="faint hint">
				What the AI is called in {$activeWorkspace?.name || 'this workspace'}: activity, comments,
				inbox and push notifications. 1–24 characters.
			</p>
		{/if}

		<h3>Create a workspace</h3>
		<div class="row">
			<input class="input" placeholder="name (e.g. Client Work)" bind:value={wsName} />
			<input class="input short" placeholder="PREFIX" bind:value={wsPrefix} />
			<button class="btn primary" onclick={createWorkspace}>Create</button>
		</div>
	</section>

	<section>
		<h2>Members of {$activeWorkspace?.name || 'this workspace'}</h2>
		<ul class="tokens">
			{#each members as m (m.userId)}
				<li>
					<span>{m.email}</span>
					<span class="faint">{m.role}</span>
					{#if canAdmin}
						<button class="btn ghost sm" onclick={() => removeMember(m.userId)}>remove</button>
					{/if}
				</li>
			{:else}
				<li class="faint">No members listed.</li>
			{/each}
		</ul>
		{#if canAdmin}
			<div class="row">
				<input class="input" placeholder="existing account email" bind:value={memberEmail} />
				<select class="input short" bind:value={memberRole}>
					<option value="member">member</option>
					<option value="admin">admin</option>
					<option value="owner">owner</option>
				</select>
				<button class="btn" onclick={addMember}>Add</button>
			</div>
			<p class="faint hint">
				The account must already exist — create it with <code>raenil user &lt;email&gt; &lt;pass&gt;</code>.
			</p>
		{/if}
	</section>

	<section>
		<h2>Notifications</h2>
		{#if !$appConfig.pushEnabled}
			<p class="faint">
				Web Push is not configured on the server (set VAPID keys). In-app SSE updates still work.
			</p>
		{:else if !pushSupported()}
			<p class="faint">This browser does not support Web Push.</p>
		{:else}
			<label class="toggle">
				<input type="checkbox" checked={pushOn} disabled={pushBusy} onchange={togglePush} />
				<span>Push notifications to this device (works when the app is closed)</span>
			</label>
			<div class="diag faint">
				<span>subscribed: <b>{pushOn ? 'yes' : 'no'}</b></span>
				<span>permission: <b>{pushPerm || '—'}</b></span>
				<span>supported: <b>{pushSupp ? 'yes' : 'no'}</b></span>
				{#if pushErr}<span class="err">last error: <b>{pushErr}</b></span>{/if}
			</div>
		{/if}
	</section>

	<section>
		<h2>MCP endpoint</h2>
		<p class="faint">Point Claude Code / an MCP client at:</p>
		<pre class="code">{mcpUrl}</pre>
		<p class="faint">
			Transport: Streamable HTTP. Authenticate with an API token below as a Bearer token. Tools
			mirror Linear verbs (save_issue, list_issues, save_project…).
		</p>
	</section>

	<section>
		<h2>API tokens</h2>
		<div class="row">
			<input class="input" placeholder="token name (e.g. claude)" bind:value={newTokenName} />
			<select class="input short" bind:value={newTokenWs}>
				<option value="">all my workspaces</option>
				{#each $workspaces as w (w.id)}
					<option value={w.slug}>{w.name}</option>
				{/each}
			</select>
			<button class="btn primary" onclick={createToken}>Create</button>
		</div>
		<p class="faint hint">
			Pinning a token to one workspace is how an agent working in a single repo only ever sees
			that repo's tracker.
		</p>
		{#if createdSecret}
			<div class="secret">
				<strong>Copy now — shown once:</strong>
				<pre class="code">{createdSecret}</pre>
				<button class="btn ghost sm" onclick={() => (createdSecret = '')}>dismiss</button>
			</div>
		{/if}
		<ul class="tokens">
			{#each tokens as t (t.id)}
				<li>
					<span>{t.name}</span>
					<span class="mono faint">{t.workspaceName || 'all workspaces'}</span>
					<span class="faint">{new Date(t.createdAt).toLocaleDateString()}</span>
					<button class="btn ghost sm" onclick={() => delToken(t.id)}>revoke</button>
				</li>
			{:else}
				<li class="faint">No tokens yet.</li>
			{/each}
		</ul>
	</section>
</div>
	</div>
</div>

<style>
	.settings {
		max-width: 640px;
		margin: 0 auto;
		padding: 24px 20px 60px;
		height: 100%;
		overflow-y: auto;
	}
	h1 {
		font-size: 22px;
		margin-bottom: 20px;
	}
	section {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 16px 18px;
		margin-bottom: 16px;
	}
	h2 {
		font-size: 14px;
		margin: 0 0 10px;
	}
	h3 {
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--ink-3);
		margin: 18px 0 8px;
		font-weight: 600;
	}
	.input.short {
		max-width: 150px;
		flex: none;
	}
	.hint {
		font-size: 12px;
		margin-top: 8px;
	}
	.mono {
		font-family: var(--mono);
		font-size: 11.5px;
	}
	.seg {
		display: flex;
		gap: 2px;
		background: var(--sunken);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 2px;
	}
	.sg {
		padding: 4px 12px;
		border-radius: var(--r-sm);
		font-size: 13px;
		color: var(--ink-2);
		background: none;
		border: none;
		cursor: pointer;
	}
	.sg:hover {
		color: var(--ink);
	}
	.sg.on {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow-1);
	}
	.row {
		display: flex;
		gap: 8px;
	}
	.code {
		background: var(--paper);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 10px;
		font-family: var(--mono);
		font-size: 12.5px;
		overflow-x: auto;
		white-space: pre-wrap;
		word-break: break-all;
	}
	.secret {
		margin-top: 12px;
		background: var(--paper);
		border: 1px solid var(--accent-soft);
		border-radius: var(--r);
		padding: 12px;
	}
	.toggle {
		display: flex;
		gap: 10px;
		align-items: center;
		font-size: 13.5px;
	}
	.diag {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 14px;
		margin-top: 10px;
		font-size: 12px;
		font-family: var(--mono);
	}
	.diag b {
		color: var(--ink);
		font-weight: 600;
	}
	.diag .err b {
		color: var(--danger);
		word-break: break-word;
	}
	.tokens {
		list-style: none;
		padding: 0;
		margin: 12px 0 0;
	}
	.tokens li {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 8px 0;
		border-bottom: 1px solid var(--line);
	}
	.tokens li span:first-child {
		flex: 1;
	}
	.btn.sm {
		padding: 3px 8px;
		font-size: 12px;
	}
</style>
