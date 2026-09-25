<script>
	// Choose an agent's model from what the host reported its harness can run
	// — Claude's aliases, the Codex login's models, the OpenCode providers that
	// have keys — grouped by provider, with a custom value for anything else.
	let { value = $bindable(''), models = [], harness = '' } = $props();

	const CUSTOM = '__custom__';
	// Claude and Codex fall back to their own configured model; OpenCode has no
	// such default, so it must name one.
	const needsModel = $derived(harness === 'opencode');
	const listed = $derived(models.filter((m) => m !== 'default'));
	const groups = $derived.by(() => {
		const out = new Map();
		for (const m of listed) {
			const [prov, rest] = m.includes('/') ? [m.slice(0, m.indexOf('/')), m] : ['', m];
			if (!out.has(prov)) out.set(prov, []);
			out.get(prov).push(rest);
		}
		return [...out.entries()];
	});
	let custom = $state(false);
	$effect(() => {
		// A saved value the host does not list is shown as custom, not lost.
		if (value && value !== 'default' && !listed.includes(value)) custom = true;
	});
	const selected = $derived(custom ? CUSTOM : value || (needsModel ? '' : 'default'));

	function pick(e) {
		const v = e.currentTarget.value;
		if (v === CUSTOM) {
			custom = true;
			return;
		}
		custom = false;
		value = v === 'default' ? '' : v;
	}
</script>

<div class="mp">
	<select value={selected} onchange={pick}>
		{#if needsModel}
			<option value="" disabled>Choose a model…</option>
		{:else}
			<option value="default">Default (its own configured model)</option>
		{/if}
		{#each groups as [prov, ms] (prov)}
			{#if prov}
				<optgroup label={prov}>
					{#each ms as m (m)}<option value={m}>{m.slice(prov.length + 1)}</option>{/each}
				</optgroup>
			{:else}
				{#each ms as m (m)}<option value={m}>{m}</option>{/each}
			{/if}
		{/each}
		<option value={CUSTOM}>Custom…</option>
	</select>
	{#if custom}
		<input
			class="mono"
			bind:value
			placeholder={harness === 'opencode' ? 'provider/model, e.g. opencode-go/glm-5.3-flash' : 'model name'}
		/>
	{/if}
	{#if !listed.length}
		<span class="hint">No list yet — start the runner host so it can report what this harness offers.</span>
	{/if}
</div>

<style>
	.mp {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	select,
	input {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 7px 9px;
		font-size: 13px;
		color: var(--text);
		outline: none;
		font-family: inherit;
	}
	select:focus,
	input:focus {
		border-color: var(--border-strong);
	}
	.mono {
		font-family: var(--mono);
		font-size: 12px;
	}
	.hint {
		font-size: 11.5px;
		color: var(--text-faint);
	}
</style>
