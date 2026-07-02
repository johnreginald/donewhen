<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { appConfig } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { pushSupported, enablePush, disablePush, currentSubscription } from '$lib/push.js';

	let tokens = $state([]);
	let newTokenName = $state('');
	let createdSecret = $state('');
	let pushOn = $state(false);
	let pushBusy = $state(false);

	onMount(async () => {
		tokens = (await api.tokens()) || [];
		pushOn = !!(await currentSubscription());
	});

	async function createToken() {
		try {
			const res = await api.createToken(newTokenName || 'token');
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
					showToast('Push enabled');
				} else showToast('Push: ' + r.reason, 'error');
			}
		} finally {
			pushBusy = false;
		}
	}

	const mcpUrl = $derived(($appConfig.baseUrl || '') + '/mcp');
</script>

<div class="settings">
	<h1>Settings</h1>

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
			<button class="btn primary" onclick={createToken}>Create</button>
		</div>
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
					<span class="faint">{new Date(t.createdAt).toLocaleDateString()}</span>
					<button class="btn ghost sm" onclick={() => delToken(t.id)}>revoke</button>
				</li>
			{:else}
				<li class="faint">No tokens yet.</li>
			{/each}
		</ul>
	</section>
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
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 16px 18px;
		margin-bottom: 16px;
	}
	h2 {
		font-size: 14px;
		margin: 0 0 10px;
	}
	.row {
		display: flex;
		gap: 8px;
	}
	.code {
		background: var(--bg);
		border: 1px solid var(--border);
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
		background: var(--bg);
		border: 1px solid var(--accent-dim);
		border-radius: var(--radius);
		padding: 12px;
	}
	.toggle {
		display: flex;
		gap: 10px;
		align-items: center;
		font-size: 13.5px;
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
		border-bottom: 1px solid var(--border);
	}
	.tokens li span:first-child {
		flex: 1;
	}
	.btn.sm {
		padding: 3px 8px;
		font-size: 12px;
	}
</style>
