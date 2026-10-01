#!/usr/bin/env node
// /tasks — print a workspace's open DoneWhen issues grouped by epic.
// Zero dependencies: Node built-ins only. Reads the DoneWhen REST API with a
// bearer token (DONEWHEN_TOKEN) so the output is deterministic and costs no
// model tokens to fetch.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const TIMEOUT_MS = 5000;
const MIN_WIDTH = 100;

// ---- args ----

// Subcommands: `workspaces` lists every reachable workspace; `use <ws>`
// switches this repo's default; `outline <ws>` lists a workspace's projects,
// epics and states for menus. Anything else is the grouped task view.
// `--json` makes `workspaces` and `outline` machine-readable.
export function parseArgs(argv) {
	const out = { cmd: 'tasks', workspace: '', project: '', epic: '', state: '', all: false, json: false };
	for (let i = 0; i < argv.length; i++) {
		const a = argv[i];
		if (i === 0 && (a === 'workspaces' || a === 'use' || a === 'outline')) out.cmd = a;
		else if (a === '--all') out.all = true;
		else if (a === '--json') out.json = true;
		else if (a === '--project' || a === '--epic' || a === '--state') {
			const v = argv[++i];
			if (v === undefined || v.startsWith('--')) throw new UsageError(`${a} needs a value`);
			out[a.slice(2)] = v;
		} else if (a.startsWith('--')) throw new UsageError(`Unknown option ${a}`);
		else if (!out.workspace) out.workspace = a;
		else throw new UsageError(`Unexpected argument '${a}'`);
	}
	if (out.cmd === 'use' && !out.workspace) throw new UsageError('use needs a workspace');
	if (out.cmd === 'workspaces' && out.workspace) throw new UsageError(`Unexpected argument '${out.workspace}'`);
	return out;
}

export class UsageError extends Error {}

// ---- config ----

// locateConfig walks up from dir looking for .claude/donewhen.json (the repo's
// default workspace and project), then falls back to the user-wide
// ~/.config/donewhen/default.json. Returns the file it used, or path null.
export function locateConfig(dir, home = homedir()) {
	const read = (p) => {
		try {
			return JSON.parse(readFileSync(p, 'utf8'));
		} catch {
			return {};
		}
	};
	let d = resolve(dir);
	for (;;) {
		// donewhen.json wins; raenil.json is the pre-rename name, read as a fallback.
		for (const name of ['donewhen.json', 'raenil.json']) {
			const p = join(d, '.claude', name);
			if (existsSync(p)) return { path: p, data: read(p) };
		}
		const up = dirname(d);
		if (up === d) break;
		d = up;
	}
	for (const p of [userConfigPath(home), legacyUserConfigPath(home)]) {
		if (existsSync(p)) return { path: p, data: read(p) };
	}
	return { path: null, data: {} };
}

export const findConfig = (dir, home) => locateConfig(dir, home).data;

const userConfigPath = (home) => join(home, '.config', 'donewhen', 'default.json');
// Pre-rename location, still read when the new one is absent.
const legacyUserConfigPath = (home) => join(home, '.config', 'raenil', 'default.json');

// configTarget is where `use` writes: the git root's .claude/donewhen.json, or
// the user-wide default outside a repo.
export function configTarget(cwd, home, gitRoot = findGitRoot) {
	const root = gitRoot(cwd);
	return root ? join(root, '.claude', 'donewhen.json') : userConfigPath(home);
}

function findGitRoot(cwd) {
	try {
		return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, stdio: ['ignore', 'pipe', 'ignore'] })
			.toString()
			.trim();
	} catch {
		return null;
	}
}

// writeConfig sets workspace and project in the file, keeping every other key.
// No project clears a stored one.
export function writeConfig(path, workspace, project) {
	let data = {};
	try {
		data = JSON.parse(readFileSync(path, 'utf8'));
	} catch {}
	data.workspace = workspace;
	if (project) data.project = project;
	else delete data.project;
	mkdirSync(dirname(path), { recursive: true });
	writeFileSync(path, JSON.stringify(data, null, 2) + '\n');
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

// renderWorkspaceTable is `/tasks workspaces`: one row per workspace with its
// open count and each project's open count; ▸ marks the current default.
// rows: [{ ws, open, projects: [{ name, open }] }]
export function renderWorkspaceTable(rows, currentId) {
	if (!rows.length) return 'This token reaches no workspaces.';
	const slugW = Math.max('Workspace'.length, ...rows.map((r) => r.ws.slug.length));
	const preW = Math.max('Prefix'.length, ...rows.map((r) => r.ws.keyPrefix.length));
	const openW = Math.max('Open'.length, ...rows.map((r) => String(r.open).length));
	const line = (mark, slug, pre, open, projects) =>
		`${mark} ${slug.padEnd(slugW)}  ${pre.padEnd(preW)}  ${open.padStart(openW)}  ${projects}`;
	return [
		line(' ', 'Workspace', 'Prefix', 'Open', 'Projects'),
		...rows.map((r) =>
			line(
				r.ws.id === currentId ? '▸' : ' ',
				r.ws.slug,
				r.ws.keyPrefix,
				String(r.open),
				r.projects.length ? r.projects.map((p) => `${p.name} (${p.open})`).join(', ') : '—'
			)
		)
	].join('\n');
}

// outline is what the interactive command builds its menus from: projects and
// epics with their open counts, open counts per state, and the tickets that
// need a person's attention (Blocked first, then In Review, then In Progress).
export function outline(states, initiatives, projects, issues, workspaceName = '') {
	const stateById = new Map(states.map((s) => [s.id, s]));
	const open = issues.filter((i) => !isClosed(stateById.get(i.stateId)));
	const iniName = new Map(initiatives.map((i) => [i.id, i.name]));
	const byIni = initiatives.slice().sort((a, b) => a.position - b.position || a.name.localeCompare(b.name));
	const rank = { blocked: 0, 'in review': 1, 'in progress': 2 };
	const hot = open
		.filter((i) => rank[stateById.get(i.stateId)?.name.toLowerCase()] !== undefined)
		.sort((a, b) => rank[stateById.get(a.stateId).name.toLowerCase()] - rank[stateById.get(b.stateId).name.toLowerCase()] || a.number - b.number)
		.map((i) => ({ key: i.key, title: i.title, state: stateById.get(i.stateId).name }));
	return {
		workspace: workspaceName,
		open: open.length,
		projects: byIni.map((i) => ({ name: i.name, open: open.filter((x) => projects.some((p) => p.id === x.projectId && p.initiativeId === i.id)).length })),
		epics: projects
			.map((p) => ({ name: p.name, project: iniName.get(p.initiativeId) || '', open: open.filter((i) => i.projectId === p.id).length }))
			.filter((e) => e.open > 0)
			.sort((a, b) => b.open - a.open || a.name.localeCompare(b.name)),
		states: states
			.slice()
			.sort((a, b) => a.position - b.position)
			.filter((s) => !isClosed(s))
			.map((s) => ({ name: s.name, open: open.filter((i) => i.stateId === s.id).length }))
			.filter((s) => s.open > 0),
		hot
	};
}

// openCounts tallies open issues per workspace and per project (initiative).
export function openCounts(states, initiatives, projects, issues) {
	const closed = new Set(states.filter(isClosed).map((s) => s.id));
	const iniOf = new Map(projects.map((p) => [p.id, p.initiativeId]));
	const perIni = new Map(initiatives.map((i) => [i.id, 0]));
	let open = 0;
	for (const i of issues) {
		if (closed.has(i.stateId)) continue;
		open++;
		const ini = i.projectId && iniOf.get(i.projectId);
		if (ini && perIni.has(ini)) perIni.set(ini, perIni.get(ini) + 1);
	}
	const byPos = initiatives.slice().sort((a, b) => a.position - b.position || a.name.localeCompare(b.name));
	return { open, projects: byPos.map((i) => ({ name: i.name, open: perIni.get(i.id) })) };
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

const USAGE = `Usage: /tasks [workspace] [--project <text>] [--all] [--epic <text>] [--state <name>]
       /tasks workspaces [--json]
       /tasks outline <workspace>      (JSON for menus)
       /tasks use <workspace> [--project <text>]`;

// run is the whole command; returns the text to print. Every failure becomes
// a one-line message — never a stack trace. ctx lets tests stub the home dir
// and git root.
export async function run(argv, env, cwd, fetchImpl = fetch, ctx = {}) {
	const home = ctx.home ?? homedir();
	let opts;
	try {
		opts = parseArgs(argv);
	} catch (e) {
		if (e instanceof UsageError) return `${e.message}\n${USAGE}`;
		throw e;
	}
	const token = env.DONEWHEN_TOKEN || env.RAENIL_TOKEN;
	if (!token) return 'Set DONEWHEN_TOKEN (mint one: donewhen token <name> [workspace])';
	const url = env.DONEWHEN_URL || env.RAENIL_URL;
	if (!url) return 'Set DONEWHEN_URL (the address of your DoneWhen server, e.g. https://tracker.example.com)';
	const cfg = findConfig(cwd, home);
	const get = client(url, token, fetchImpl);

	const wantJson = opts.json || opts.cmd === 'outline';
	const asJson = (out) => (wantJson && !out.trimStart().startsWith('{') ? JSON.stringify({ error: out }) : out);
	try {
		if (opts.cmd === 'workspaces') return opts.json ? JSON.stringify(await workspacesData(get, cfg.workspace)) : await workspacesView(get, cfg.workspace);
		if (opts.cmd === 'use') return await useWorkspace(get, opts, configTarget(cwd, home, ctx.gitRoot), env);
		const workspace = opts.workspace || cfg.workspace || '';
		if (opts.cmd === 'outline') return JSON.stringify(await outlineData(get, workspace));
		if (!opts.project && !opts.workspace && cfg.project) opts.project = cfg.project;
		return await tasksView(get, workspace, opts, env);
	} catch (e) {
		return asJson(await explain(e, url, opts.workspace || cfg.workspace || '', get));
	}
}

async function outlineData(get, workspace) {
	const [states, initiatives, projects, issues, memberships] = await Promise.all([
		get('/api/states', workspace),
		get('/api/initiatives', workspace),
		get('/api/projects', workspace),
		get('/api/issues', workspace),
		get('/api/workspaces')
	]);
	const wsId = issues[0]?.workspaceId;
	const w = memberships.find((m) => m.id === wsId) || findWorkspace(memberships, workspace) || (memberships.length === 1 ? memberships[0] : null);
	return outline(states, initiatives, projects, issues, w?.name || workspace);
}

async function tasksView(get, workspace, opts, env) {
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
}

async function workspacesData(get, currentRef) {
	const { rows, current } = await workspaceRows(get, currentRef);
	return {
		current: current?.slug || '',
		workspaces: rows
			.map((r) => ({ slug: r.ws.slug, name: r.ws.name, prefix: r.ws.keyPrefix, open: r.open, projects: r.projects }))
			.sort((a, b) => b.open - a.open || a.name.localeCompare(b.name))
	};
}

async function workspacesView(get, currentRef) {
	const { rows, current } = await workspaceRows(get, currentRef);
	return renderWorkspaceTable(rows, current?.id);
}

async function workspaceRows(get, currentRef) {
	const memberships = await get('/api/workspaces');
	const rows = await Promise.all(
		memberships.map(async (ws) => {
			const [states, initiatives, projects, issues] = await Promise.all([
				get('/api/states', ws.id),
				get('/api/initiatives', ws.id),
				get('/api/projects', ws.id),
				get('/api/issues', ws.id)
			]);
			return { ws, ...openCounts(states, initiatives, projects, issues) };
		})
	);
	const current = findWorkspace(memberships, currentRef) || (memberships.length === 1 ? memberships[0] : null);
	return { rows, current };
}

async function useWorkspace(get, opts, target, env) {
	const memberships = await get('/api/workspaces');
	const ws = findWorkspace(memberships, opts.workspace);
	if (!ws) return `No access to workspace '${opts.workspace}'\n${renderWorkspaces(memberships)}`.trimEnd();

	let project = '';
	if (opts.project) {
		const initiatives = await get('/api/initiatives', ws.id);
		const exact = initiatives.filter((i) => i.name.toLowerCase() === opts.project.toLowerCase());
		const hits = exact.length ? exact : initiatives.filter((i) => contains(i.name, opts.project));
		const names = (list) => list.map((i) => `  ${i.name}`).join('\n');
		if (hits.length === 0) {
			const all = initiatives.length ? names(initiatives) : '  (no projects)';
			return `No project matching '${opts.project}' in ${ws.slug}\n${all}`;
		}
		if (hits.length > 1) return `Several projects match '${opts.project}' in ${ws.slug} — be more specific:\n${names(hits)}`;
		project = hits[0].name;
	}

	writeConfig(target, ws.slug, project);
	const view = await tasksView(get, ws.id, { ...opts, cmd: 'tasks', workspace: ws.slug, project }, env);
	return `Default for ${target} → ${ws.name}${project ? ` › ${project}` : ''}\n\n${view}`;
}

async function explain(e, url, workspace, get) {
	const listWorkspaces = async () => {
		try {
			return renderWorkspaces(await get('/api/workspaces'));
		} catch {
			return '';
		}
	};
	if (e instanceof Unreachable) return `DoneWhen unreachable at ${url} — is the server running and reachable?`;
	if (e instanceof ApiError) {
		if (e.status === 401) return 'Token rejected — mint a new one';
		if (e.status === 400 && /workspace/i.test(e.message)) {
			return `Several workspaces — pass one: /tasks <workspace>\n${await listWorkspaces()}`.trimEnd();
		}
		if (e.status === 403 || e.status === 404) {
			return `No access to workspace '${workspace}'\n${await listWorkspaces()}`.trimEnd();
		}
		return `DoneWhen error ${e.status}${e.message ? `: ${e.message}` : ''}`;
	}
	return `Unexpected error: ${e?.message || e}`;
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
