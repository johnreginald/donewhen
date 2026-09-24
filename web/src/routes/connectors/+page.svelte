<script>
	// Connectors: which harnesses can run, on which machine, on which plan.
	// Paperclip keeps a subscription login server-side; here the runner host
	// proves what it is signed into and this page shows that proof.
	import { onMount } from 'svelte';
	import { api } from '$lib/api.js';
	import { onLive, showToast } from '$lib/ui.js';
	import { rel } from '$lib/format.js';
	import { HARNESSES, hostOnline } from '$lib/harness.js';
	import PageHeader from '$components/PageHeader.svelte';
	import { Check, X, Copy, Server } from '@lucide/svelte';

	let hosts = $state([]);
	let loaded = $state(false);
	async function load() {
		hosts = (await api.hosts().catch(() => [])) || [];
		loaded = true;
	}
	onMount(load);
	onMount(() => onLive((ev) => ev.type === 'host.updated' && load()));
	// Re-render "last seen" and online-ness without a reload.
	let tick = $state(0);
	onMount(() => {
		const t = setInterval(() => tick++, 15000);
		return () => clearInterval(t);
	});

	const statusOn = (h, id) => (h.harnesses || []).find((x) => x.harness === id);
	function copy(text) {
		navigator.clipboard?.writeText(text).then(() => showToast('Copied'));
	}
</script>

<div class="pg">
	<PageHeader crumbs={[{ label: 'Connectors' }]} />
	<div class="pg-body">
		<div class="wrap">
			<section>
				<h2>AI connections</h2>
				<p class="lead">
					Claude Code and Codex run on your subscription logins, never an API key. OpenCode Go is the one
					key-based connection. Each is checked on the machine that runs it.
				</p>
				<div class="conns">
					{#each HARNESSES as hz (hz.id)}
						{@const rows = hosts.map((h) => ({ h, st: statusOn(h, hz.id) })).filter((x) => x.st)}
						{@const anyReady = rows.some((x) => x.st.ready && hostOnline(x.h))}
						<article class="conn" class:ready={anyReady}>
							<header>
								<div>
									<div class="cn">{hz.name}</div>
									<div class="cp">{hz.plan}</div>
								</div>
								<span class="badge" class:ok={anyReady}>{anyReady ? 'Connected' : 'Not connected'}</span>
							</header>
							{#each rows as { h, st } (h.id)}
								{@const online = (tick, hostOnline(h))}
								<div class="hrow">
									<span class="hs" class:ok={st.ready && online}>
										{#if st.ready && online}<Check size={13} strokeWidth={2.4} />{:else}<X size={13} strokeWidth={2.4} />{/if}
									</span>
									<span class="hn">{h.name}</span>
									<span class="hd">
										{#if !online}offline · last seen {rel(h.lastSeenAt)}
										{:else if st.ready}{st.auth}{st.detail ? ' · ' + st.detail : ''}
										{:else}{st.detail || 'not ready'}{/if}
									</span>
								</div>
								{#if online && !st.ready && st.fix}
									<div class="fix">
										<span>Run on {h.name}:</span><code>{st.fix}</code>
										<button class="icon" onclick={() => copy(st.fix)} aria-label="Copy"><Copy size={13} /></button>
									</div>
								{/if}
							{:else}
								<div class="none">
									No machine has reported this yet. Sign in on your Mac with <code>{hz.connect}</code>, then start the runner host.
								</div>
							{/each}
						</article>
					{/each}
				</div>
			</section>

			<section>
				<h2>Runner hosts</h2>
				<p class="lead">
					The machines agents run on — where your repos and CLI logins are. A host reports in every 30 seconds
					and takes the work you queue from here.
				</p>
				{#if loaded && !hosts.length}
					<div class="setup">
						<Server size={18} strokeWidth={1.8} />
						<div>
							<div>No host has reported in. On your Mac:</div>
							<pre>RAENIL_URL={location.origin} RAENIL_TOKEN=&lt;token from Settings&gt; orchestrator host</pre>
						</div>
					</div>
				{/if}
				{#each hosts as h (h.id)}
					{@const online = (tick, hostOnline(h))}
					<div class="host">
						<span class="dot" class:ok={online}></span>
						<span class="hn">{h.name}</span>
						<span class="hd">{online ? 'online' : 'offline'} · last seen {rel(h.lastSeenAt)}</span>
						<span class="hv">
							{(h.harnesses || []).filter((x) => x.ready).map((x) => HARNESSES.find((z) => z.id === x.harness)?.name).join(', ') || 'nothing ready'}
						</span>
					</div>
				{/each}
			</section>
		</div>
	</div>
</div>

<style>
	.wrap {
		padding: 20px clamp(16px, 3vw, 28px) 40px;
		max-width: 960px;
		display: flex;
		flex-direction: column;
		gap: 28px;
	}
	h2 {
		margin: 0 0 4px;
		font-size: 15px;
		font-weight: 600;
	}
	.lead {
		margin: 0 0 12px;
		font-size: 13px;
		color: var(--text-faint);
		max-width: 680px;
	}
	.conns {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.conn {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.conn header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.cn {
		font-weight: 500;
	}
	.cp {
		font-size: 12px;
		color: var(--text-faint);
	}
	.badge {
		font-size: 11.5px;
		padding: 2px 9px;
		border-radius: 999px;
		border: 1px solid color-mix(in srgb, #d03b3b 40%, transparent);
		color: #fca5a5;
	}
	.badge.ok {
		border-color: color-mix(in srgb, #0ca30c 40%, transparent);
		color: #86efac;
	}
	.hrow,
	.host {
		display: flex;
		align-items: center;
		gap: 9px;
		font-size: 13px;
		min-width: 0;
	}
	.hs {
		display: inline-flex;
		color: #fca5a5;
	}
	.hs.ok {
		color: #86efac;
	}
	.hn {
		font-weight: 500;
	}
	.hd {
		color: var(--text-faint);
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.hv {
		margin-left: auto;
		color: var(--text-dim);
		font-size: 12px;
	}
	.fix {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--text-faint);
		padding-left: 22px;
	}
	code,
	pre {
		font-family: var(--mono);
		font-size: 11.5px;
		background: var(--bg-elev2);
		border: 1px solid var(--border);
		border-radius: 4px;
		padding: 1px 5px;
		color: var(--text);
	}
	pre {
		padding: 8px 10px;
		margin: 6px 0 0;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.icon {
		background: none;
		border: none;
		color: var(--text-faint);
		display: inline-flex;
		padding: 3px;
		border-radius: 5px;
	}
	.icon:hover {
		color: var(--text);
		background: var(--bg-hover);
	}
	.none {
		font-size: 12.5px;
		color: var(--text-faint);
	}
	.setup {
		display: flex;
		gap: 12px;
		align-items: flex-start;
		font-size: 13px;
		color: var(--text-dim);
		border: 1px dashed var(--border-strong);
		border-radius: var(--radius-lg);
		padding: 14px 16px;
	}
	.host {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 9px;
		padding: 10px 14px;
		margin-bottom: 8px;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--text-faint);
		flex-shrink: 0;
	}
	.dot.ok {
		background: #0ca30c;
	}
</style>
