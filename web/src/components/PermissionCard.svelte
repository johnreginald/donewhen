<script>
	// After a work run that was refused commands: offer to allow them for this
	// agent and run the ticket again, in one click. Nothing is asked mid-run —
	// the refusal already happened, and this turns it into a rule.
	import { onMount } from 'svelte';
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

	// Rules already in force: built-in defaults (as the host reports them),
	// the workspace's, and the agent's own. A suggestion one of them already
	// covers is left out — the refusal came from another part of the chain.
	let defaults = $state([]);
	let shared = $state([]);
	onMount(async () => {
		const [hosts, ws] = await Promise.all([api.hosts().catch(() => []), api.get('/allowed-tools').catch(() => null)]);
		for (const h of hosts || []) for (const hs of h.harnesses || []) if (hs.harness === 'claude') defaults = hs.alwaysAllowed || defaults;
		shared = ws?.allowedTools || [];
	});
	const inForce = $derived([...defaults, ...shared, ...(agent?.allowedTools || [])]);
	const covered = (cmd) =>
		inForce.some((r) => {
			const m = /^Bash\((.*)\)$/.exec(r.trim());
			if (!m) return false;
			const pat = m[1].replace(/\s*\*$/, '').trim();
			return pat === '' ? false : cmd === pat || cmd.startsWith(pat);
		});

	// Shell syntax — if/for/while and their parts — is not a command a rule
	// can allow; Claude refuses a chain that uses it.
	const SYNTAX = /^(if|then|else|elif|fi|for|do|done|while|until|case|esac|\[|\[\[|!|\{|\})$/;

	// One rule per part of a chain that is not already allowed, wide enough
	// for the command's variants: its first two words when the second is a
	// subcommand ("docker compose"), else the first. Never a path.
	function suggestions(cmd) {
		const out = [];
		for (let part of cmd.split(/&&|\|\||;|\||\n/)) {
			part = part.trim().replace(/^\(+|\)+$/g, '').replace(/\s*[0-9]?>\S*.*$/, '').trim();
			const words = part.split(/\s+/).filter(Boolean);
			if (!words.length || SYNTAX.test(words[0]) || words[0].includes('=') || covered(part)) continue;
			const second = /^[a-z][a-z-]*$/.test(words[1] || '') ? words[1] : '';
			out.push(second ? `Bash(${words[0]} ${second} *)` : `Bash(${words[0]} *)`);
		}
		return out;
	}
	const usesSyntax = $derived(shell.some((c) => /(^|[;&|]\s*)(if|for|while)\s/.test(c)));

	let rules = $state([]);
	$effect(() => {
		rules = [...new Set(shell.flatMap(suggestions))].map((r) => ({ rule: r, on: true }));
	});
	// A run that passed anyway needs no second go.
	const passed = $derived(run.verdict === 'passed');
	let busy = $state(false);
	// Who the rules are for: this agent, or every agent in the workspace.
	let scope = $state('agent');

	const chosen = $derived(rules.filter((r) => r.on && r.rule.trim()).map((r) => r.rule.trim()));

	async function allowAndRun() {
		if (!agent || !chosen.length) return;
		busy = true;
		try {
			if (scope === 'all') {
				const cur = (await api.get('/allowed-tools'))?.allowedTools || [];
				await api.put('/allowed-tools', { allowedTools: [...new Set([...cur, ...chosen])] });
			} else {
				const allowed = [...new Set([...(agent.allowedTools || []), ...chosen])];
				const updated = await api.updateAgent(agent.id, { allowedTools: allowed });
				agents.update((list) => list.map((a) => (a.id === updated.id ? updated : a)));
			}
			if (!passed) await api.post(`/issues/${issue.key}/run`, {});
			showToast(`Allowed for ${scope === 'all' ? 'every agent' : agent.name}${passed ? '' : ' · running again'}`);
			ondone?.();
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			busy = false;
		}
	}
</script>

<!-- Nothing left to decide on a run that passed: no card. -->
{#if agent && agent.harness === 'claude' && (rules.length || other.length || (shell.length && !passed))}
	<div class="pc">
		<div class="ph">
			<ShieldAlert size={15} strokeWidth={2} />
			<span>{agent.name} was refused {shell.length + other.length === 1 ? 'a command' : 'some commands'}</span>
		</div>
		{#if rules.length}
			<div class="scope" role="radiogroup" aria-label="Allow for">
				<span class="sl">Allow from now on for</span>
				<button role="radio" aria-checked={scope === 'agent'} class:on={scope === 'agent'} onclick={() => (scope = 'agent')}>{agent.name}</button>
				<button role="radio" aria-checked={scope === 'all'} class:on={scope === 'all'} onclick={() => (scope = 'all')}>Every agent</button>
			</div>
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
		{#if !rules.length && shell.length}
			<p class="hint">
				Every command in it is allowed now{usesSyntax ? ', but it was written as a shell script (if / for), which a rule cannot allow — the agent can run the same commands one at a time' : ''}.
			</p>
			<details class="raw">
				<summary>What it tried</summary>
				<ul>{#each shell as c}<li class="mono">{c}</li>{/each}</ul>
			</details>
		{/if}
		{#if usesSyntax && rules.length}
			<p class="hint">Part of it was a shell script (if / for), which no rule can allow.</p>
		{/if}
		{#if other.length}
			<p class="hint">Refused and not something a rule can allow — writes outside the ticket's worktree:</p>
			<ul class="raw-list">{#each other as d}<li class="mono">{d}</li>{/each}</ul>
		{/if}
		{#if rules.length}
			<div class="pf">
				<span class="faint">{scope === 'all' ? 'Shared rules are on each agent’s Harness page.' : 'Rules can be changed on the agent’s Harness page.'}</span>
				<button class="btn primary sm" onclick={allowAndRun} disabled={busy || !chosen.length}>
					<Play size={13} strokeWidth={2.4} />{busy ? 'Allowing…' : passed ? 'Allow' : 'Allow and run again'}
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
	.scope {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		font-size: 12.5px;
	}
	.sl {
		color: var(--text-dim);
	}
	.scope button {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 2px 10px;
		font-size: 12px;
		color: var(--text-dim);
	}
	.scope button.on {
		border-color: var(--accent2);
		color: var(--text);
		background: color-mix(in srgb, var(--accent2) 14%, var(--bg));
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
