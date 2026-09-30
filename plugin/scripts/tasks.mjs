#!/usr/bin/env node
// /tasks — print a workspace's open Raenil issues grouped by epic.
// Zero dependencies: Node built-ins only. Reads the Raenil REST API with a
// bearer token (RAENIL_TOKEN) so the output is deterministic and costs no
// model tokens to fetch.

import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const DEFAULT_URL = 'https://tracker.example.com';
const TIMEOUT_MS = 5000;
const MIN_WIDTH = 100;

// ---- args ----

export function parseArgs(argv) {
	const out = { workspace: '', project: '', epic: '', state: '', all: false };
	for (let i = 0; i < argv.length; i++) {
		const a = argv[i];
		if (a === '--all') out.all = true;
		else if (a === '--project' || a === '--epic' || a === '--state') {
			const v = argv[++i];
			if (v === undefined || v.startsWith('--')) throw new UsageError(`${a} needs a value`);
			out[a.slice(2)] = v;
		} else if (a.startsWith('--')) throw new UsageError(`Unknown option ${a}`);
		else if (!out.workspace) out.workspace = a;
		else throw new UsageError(`Unexpected argument '${a}'`);
	}
	return out;
}

export class UsageError extends Error {}

// ---- config ----

// findConfig walks up from dir to the filesystem root looking for
// .claude/raenil.json — the repo's default workspace and project.
export function findConfig(dir) {
	let d = resolve(dir);
	for (;;) {
		const p = join(d, '.claude', 'raenil.json');
		if (existsSync(p)) {
			try {
				return JSON.parse(readFileSync(p, 'utf8'));
			} catch {
				return {};
			}
		}
		const up = dirname(d);
		if (up === d) return {};
		d = up;
	}
}

// ---- glyphs ----

export function glyph(state) {
	const n = state.name.toLowerCase();
	if (n === 'blocked') return '⊘';
	if (n === 'in review') return '◕';
	switch (state.category) {
		case 'triage':
			return '◌';
		case 'started':
			return '◐';
		case 'completed':
			return '●';
		case 'canceled':
			return '×';
		default:
			return '○';
	}
}

const isClosed = (state) => state && (state.category === 'completed' || state.category === 'canceled');
const contains = (hay, needle) => hay.toLowerCase().includes(needle.toLowerCase());

// ---- render ----

// render turns the API payloads into the grouped text. Pure: no I/O.
export function render(data, opts, width = MIN_WIDTH) {
	const { workspaceName, states, initiatives, projects, issues, blockers } = data;
	const stateById = new Map(states.map((s) => [s.id, s]));
	const iniById = new Map(initiatives.map((i) => [i.id, i]));
	const issueById = new Map(issues.map((i) => [i.id, i]));

	// Open blockers per issue, as keys.
	const waiting = new Map();
	for (const l of blockers) {
		if (l.done) continue;
		const key = issueById.get(l.blockerId)?.key;
		if (!key) continue;
		if (!waiting.has(l.issueId)) waiting.set(l.issueId, []);
		waiting.get(l.issueId).push(key);
	}

	let epics = projects.slice();
	if (opts.project) {
		epics = epics.filter((p) => p.initiativeId && contains(iniById.get(p.initiativeId)?.name || '', opts.project));
	}
	if (opts.epic) epics = epics.filter((p) => contains(p.name, opts.epic));

	// Epics ordered by initiative (position, name), then epic name; epics
	// with no initiative after those that have one.
	const iniRank = (p) => {
		const ini = p.initiativeId && iniById.get(p.initiativeId);
		return ini ? [0, ini.position, ini.name] : [1, 0, ''];
	};
	epics.sort((a, b) => {
		const [ga, pa, na] = iniRank(a);
		const [gb, pb, nb] = iniRank(b);
		return ga - gb || pa - pb || na.localeCompare(nb) || a.name.localeCompare(b.name);
	});

	const groups = epics.map((p) => ({ name: p.name, all: issues.filter((i) => i.projectId === p.id) }));
	// "No epic" only when no epic/project filter narrows the view.
	if (!opts.epic && !opts.project) {
		groups.push({ name: 'No epic', all: issues.filter((i) => !i.projectId) });
	}

	const visible = (i) => {
		const st = stateById.get(i.stateId);
		if (!opts.all && isClosed(st)) return false;
		if (opts.state && (!st || st.name.toLowerCase() !== opts.state.toLowerCase())) return false;
		return true;
	};
	const order = (a, b) => (stateById.get(a.stateId)?.position ?? 99) - (stateById.get(b.stateId)?.position ?? 99) || a.number - b.number;

	const shown = groups
		.map((g) => ({ ...g, rows: g.all.filter(visible).sort(order) }))
		.filter((g) => g.rows.length > 0 || (opts.all && g.all.length > 0));

	const rows = shown.flatMap((g) => g.rows);
	const keyW = Math.max(0, ...rows.map((i) => i.key.length));
	const stW = Math.max(0, ...rows.map((i) => stateById.get(i.stateId)?.name.length || 0));
	const headW = Math.max(40, ...shown.map((g) => g.name.length + 2));

	const noun = opts.all ? 'issues' : 'open';
	const lines = [`${workspaceName} · ${rows.length} ${noun}`];
	if (shown.length === 0) {
		lines.push('', opts.all ? 'No issues match.' : 'No open issues match.');
		return lines.join('\n');
	}
	for (const g of shown) {
		const done = g.all.filter((i) => stateById.get(i.stateId)?.category === 'completed').length;
		lines.push('', `${g.name.padEnd(headW)}${done}/${g.all.length} done`);
		for (const i of g.rows) {
			const st = stateById.get(i.stateId) || { name: '?', category: '' };
			const prefix = `  ${glyph(st)} ${i.key.padEnd(keyW)}  ${st.name.padEnd(stW)}  `;
			const blk = waiting.get(i.id);
			const suffix = blk ? `  ⊘ ${blk.join(' ')}` : '';
			lines.push(prefix + truncate(i.title, width - [...prefix].length - [...suffix].length) + suffix);
		}
	}
	return lines.join('\n');
}

export function truncate(s, max) {
	const chars = [...s];
	if (max < 2) return '…';
	return chars.length <= max ? s : chars.slice(0, max - 1).join('') + '…';
}

export function renderWorkspaces(list) {
	if (!list.length) return 'This token reaches no workspaces.';
	const w = Math.max(...list.map((x) => x.slug.length));
	return list.map((x) => `  ${x.slug.padEnd(w)}  ${x.keyPrefix.padEnd(5)}  ${x.name}`).join('\n');
}

// ---- api ----

export class ApiError extends Error {
	constructor(status, message) {
		super(message);
		this.status = status;
	}
}
export class Unreachable extends Error {}

export function client(baseURL, token, fetchImpl = fetch) {
	return async function get(path, workspace) {
		const headers = { Authorization: `Bearer ${token}`, Accept: 'application/json' };
		if (workspace) headers['X-Workspace'] = workspace;
		let res;
		try {
			res = await fetchImpl(baseURL.replace(/\/+$/, '') + path, { headers, signal: AbortSignal.timeout(TIMEOUT_MS) });
		} catch {
			throw new Unreachable();
		}
		if (!res.ok) {
			let msg = '';
			try {
				msg = (await res.json()).error || '';
			} catch {}
			throw new ApiError(res.status, msg);
		}
		return res.json();
	};
}

// run is the whole command; returns the text to print. Every failure becomes
// a one-line message — never a stack trace.
export async function run(argv, env, cwd, fetchImpl = fetch) {
	let opts;
	try {
		opts = parseArgs(argv);
	} catch (e) {
		if (e instanceof UsageError) return `${e.message}\nUsage: /tasks [workspace] [--project <text>] [--all] [--epic <text>] [--state <name>]`;
		throw e;
	}
	const token = env.RAENIL_TOKEN;
	if (!token) return 'Set RAENIL_TOKEN (mint one: raenil token <name> [workspace])';
	const url = env.RAENIL_URL || DEFAULT_URL;
	const cfg = findConfig(cwd);
	const workspace = opts.workspace || cfg.workspace || '';
	if (!opts.project && !opts.workspace && cfg.project) opts.project = cfg.project;

	const get = client(url, token, fetchImpl);
	const listWorkspaces = async () => {
		try {
			return renderWorkspaces(await get('/api/workspaces'));
		} catch {
			return '';
		}
	};

	try {
		const [states, initiatives, projects, issues, blockers, memberships] = await Promise.all([
			get('/api/states', workspace),
			get('/api/initiatives', workspace),
			get('/api/projects', workspace),
			get('/api/issues', workspace),
			get('/api/blockers', workspace),
			get('/api/workspaces')
		]);
		const wsId = issues[0]?.workspaceId;
		const w = memberships.find((m) => m.id === wsId) || findWorkspace(memberships, workspace) || (memberships.length === 1 ? memberships[0] : null);
		const workspaceName = w?.name || workspace || 'Workspace';
		return render({ workspaceName, states, initiatives, projects, issues, blockers }, opts, width(env));
	} catch (e) {
		if (e instanceof Unreachable) return `Raenil unreachable at ${url} — is Tailscale on?`;
		if (e instanceof ApiError) {
			if (e.status === 401) return 'Token rejected — mint a new one';
			if (e.status === 400 && /workspace/i.test(e.message)) {
				return `Several workspaces — pass one: /tasks <workspace>\n${await listWorkspaces()}`.trimEnd();
			}
			if (e.status === 403 || e.status === 404) {
				return `No access to workspace '${workspace}'\n${await listWorkspaces()}`.trimEnd();
			}
			return `Raenil error ${e.status}${e.message ? `: ${e.message}` : ''}`;
		}
		return `Unexpected error: ${e?.message || e}`;
	}
}

function findWorkspace(list, ref) {
	if (!ref) return null;
	const r = ref.toLowerCase();
	return list.find((m) => m.id === ref || m.slug.toLowerCase() === r || m.keyPrefix.toLowerCase() === r) || null;
}

function width(env) {
	const cols = Number(env.COLUMNS) || process.stdout.columns || 0;
	return Math.max(MIN_WIDTH, cols);
}

// ---- main ----

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	run(process.argv.slice(2), process.env, process.cwd()).then(
		(text) => process.stdout.write(text + '\n'),
		(e) => process.stdout.write(`Unexpected error: ${e?.message || e}\n`)
	);
}
