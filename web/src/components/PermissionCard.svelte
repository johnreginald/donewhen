<script>
	// After a work run that was refused commands: offer to allow them for this
	// agent and run the ticket again, in one click. Nothing is asked mid-run —
	// the refusal already happened, and this turns it into a rule.
	import { api } from '$lib/api.js';
	import { agents } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { ShieldAlert, Play } from '@lucide/svelte';

	let { run, issue, ondone } = $props();

	const agent = $derived($agents.find((a) => a.id === run.agentId));

	// Only shell commands can be allowed by a rule. A refused edit is a write
	// outside the worktree, and that stays refused.
	const shell = $derived((run.deniedTools || []).filter((d) => d.startsWith('Bash ')).map((d) => d.slice(5).trim()));
	const other = $derived((run.deniedTools || []).filter((d) => !d.startsWith('Bash ')));

	// Suggest a rule wide enough for the command's variants: its first two
	// words ("cargo test", "docker compose"), or the one word it has.
	function suggest(cmd) {
		const first = cmd.split(/&&|\|\||;|\|/)[0].trim();
		const words = first.split(/\s+/).filter(Boolean);
		return words.length > 1 ? `Bash(${words.slice(0, 2).join(' ')} *)` : `Bash(${words[0] || first}*)`;
	}

	let rules = $state([]);
	$effect(() => {
		rules = [...new Set(shell.map(suggest))].map((r) => ({ rule: r, on: true }));
	});
	let busy = $state(false);

	const chosen = $derived(rules.filter((r) => r.on && r.rule.trim()).map((r) => r.rule.trim()));

	async function allowAndRun() {
		if (!agent || !chosen.length) return;
		busy = true;
		try {
			const allowed = [...new Set([...(agent.allowedTools || []), ...chosen])];
			const updated = await api.updateAgent(agent.id, { allowedTools: allowed });
			agents.update((list) => list.map((a) => (a.id === updated.id ? updated : a)));
			await api.post(`/issues/${issue.key}/run`, {});
			showToast(`Allowed for ${agent.name} · running again`);
			ondone?.();
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			busy = false;
		}
	}
</script>

{#if agent && agent.harness === 'claude' && (shell.length || other.length)}
	<div class="pc">
		<div class="ph">
			<ShieldAlert size={15} strokeWidth={2} />
			<span>{agent.name} was refused {shell.length + other.length === 1 ? 'a command' : 'some commands'}</span>
		</div>
		{#if rules.length}
			<p class="hint">Allow these for {agent.name} from now on, and run the ticket again:</p>
			<ul class="rules">
				{#each rules as r, i (i)}
					<li>
						<input type="checkbox" bind:checked={r.on} aria-label="Allow" />
						<input class="mono" bind:value={r.rule} />
					</li>
				{/each}
			</ul>
			<details class="raw">
				<summary>What it tried</summary>
				<ul>{#each shell as c}<li class="mono">{c}</li>{/each}</ul>
			</details>
		{/if}
		{#if other.length}
			<p class="hint">Refused and not something a rule can allow — writes outside the ticket's worktree:</p>
			<ul class="raw-list">{#each other as d}<li class="mono">{d}</li>{/each}</ul>
		{/if}
		{#if rules.length}
			<div class="pf">
				<span class="faint">Rules can be changed on the agent's Harness page.</span>
				<button class="btn primary sm" onclick={allowAndRun} disabled={busy || !chosen.length}>
					<Play size={13} strokeWidth={2.4} />{busy ? 'Allowing…' : 'Allow and run again'}
				</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.pc {
		background: var(--bg-elev);
		border: 1px solid color-mix(in srgb, #f59e0b 40%, var(--border));
		border-radius: 12px;
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		font-size: 13px;
	}
	.ph {
		display: flex;
		align-items: center;
		gap: 8px;
		color: var(--text);
	}
	.ph :global(svg) {
		color: #f59e0b;
	}
	.hint {
		margin: 0;
		color: var(--text-dim);
		font-size: 12.5px;
	}
	.rules {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.rules li {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.rules input.mono {
		flex: 1;
		min-width: 0;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 5px 8px;
		font-family: var(--mono);
		font-size: 12px;
		color: var(--text);
		outline: none;
	}
	.rules input.mono:focus {
		border-color: var(--border-strong);
	}
	.raw summary {
		cursor: pointer;
		color: var(--text-faint);
		font-size: 12px;
	}
	.raw ul,
	.raw-list {
		margin: 4px 0 0;
		padding-left: 16px;
		color: var(--text-dim);
		font-size: 12px;
	}
	.mono {
		font-family: var(--mono);
		word-break: break-all;
	}
	.pf {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		font-size: 12px;
	}
</style>
