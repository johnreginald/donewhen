// What each harness is called, and what connects it — the words the Agents
// and Connectors pages share.

export const HARNESSES = [
	{
		id: 'claude',
		name: 'Claude Code',
		plan: 'Claude subscription',
		connect: 'orchestrator connect claude',
		models: ['default', 'sonnet', 'opus', 'haiku']
	},
	{ id: 'codex', name: 'Codex', plan: 'ChatGPT', connect: 'orchestrator connect codex', models: ['default'] },
	{
		id: 'opencode',
		name: 'OpenCode',
		plan: 'Keys kept in OpenCode',
		connect: 'opencode auth login, then opencode serve (OPENCODE_URL for the host)',
		models: []
	}
];

export const harnessName = (id) => HARNESSES.find((h) => h.id === id)?.name || id;

// A host that has not reported in for this long is shown as offline: it
// heartbeats every 30 seconds.
export const HOST_STALE_MS = 90_000;
export const hostOnline = (h) => Date.now() - new Date(h.lastSeenAt).getTime() < HOST_STALE_MS;

// harnessOn finds a harness's status across hosts, preferring an online host
// where it is ready.
export function harnessOn(hosts, id) {
	let best = null;
	for (const h of hosts) {
		const st = (h.harnesses || []).find((x) => x.harness === id);
		if (!st) continue;
		const cand = { host: h, status: st, online: hostOnline(h) };
		const score = (c) => (c.online ? 2 : 0) + (c.status.ready ? 1 : 0);
		if (!best || score(cand) > score(best)) best = cand;
	}
	return best;
}
