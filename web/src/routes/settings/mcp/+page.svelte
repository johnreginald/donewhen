<script>
	import { appConfig } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { copyToClipboard } from '../clipboard.js';

	const mcpUrl = $derived(($appConfig.baseUrl || '') + '/mcp');
	const cmd = $derived(
		`claude mcp add --transport http donewhen \\\n  ${mcpUrl} \\\n  --header "Authorization: Bearer <token>"`
	);

	async function copy(text) {
		if (await copyToClipboard(text)) showToast('Copied');
		else showToast('Could not copy — select and copy manually.', 'error');
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">MCP connection help</h1>
		<p class="settings-lede">Point Claude Code, or any MCP client, at this workspace's tracker.</p>
	</div>

	<section class="settings-panel">
		<div class="settings-panel-h">Endpoint</div>
		<div class="settings-cmdwrap">
			<div class="settings-cmdblock">{mcpUrl}</div>
			<button class="btn settings-btn-sm" onclick={() => copy(mcpUrl)}>Copy</button>
		</div>
		<span class="settings-hint">Transport: Streamable HTTP. Authenticate with an API token as a Bearer token.</span>
	</section>

	<section class="settings-panel">
		<div class="settings-panel-h">Register with Claude Code</div>
		<div class="settings-cmdwrap">
			<div class="settings-cmdblock">{cmd}</div>
			<button class="btn settings-btn-sm" onclick={() => copy(cmd)}>Copy</button>
		</div>
		<span class="settings-hint">
			Create a token on the <a href="/settings/tokens">API tokens</a> page — the secret is shown once.
		</span>
	</section>

	<section class="settings-panel">
		<div class="settings-panel-h">Tools</div>
		<p class="settings-hint" style="font-size:var(--t-sm);color:var(--ink-2)">
			Mirrors Linear verbs — save_issue, list_issues, save_project, set_criteria, link_commit…
		</p>
	</section>
</div>
