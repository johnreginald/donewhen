import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { existsSync, readFileSync } from 'node:fs';
import { parseArgs, render, run, truncate, findConfig, glyph, renderWorkspaceTable, openCounts } from './tasks.mjs';

const states = [
	{ id: 's-tri', name: 'Triage', category: 'triage', position: 0 },
	{ id: 's-bl', name: 'Backlog', category: 'backlog', position: 1 },
	{ id: 's-rdy', name: 'Ready', category: 'unstarted', position: 3 },
	{ id: 's-prog', name: 'In Progress', category: 'started', position: 4 },
	{ id: 's-blk', name: 'Blocked', category: 'started', position: 5 },
	{ id: 's-rev', name: 'In Review', category: 'started', position: 6 },
	{ id: 's-done', name: 'Done', category: 'completed', position: 7 },
	{ id: 's-can', name: 'Canceled', category: 'canceled', position: 8 }
];
const initiatives = [
	{ id: 'i-b', name: 'Zeta', position: 1 },
	{ id: 'i-a', name: 'Platform', position: 0 }
];
const projects = [
	{ id: 'p-ui', name: 'Core — UI refresh', initiativeId: 'i-a' },
	{ id: 'p-data', name: 'Core — Data integrity', initiativeId: 'i-a' },
	{ id: 'p-z', name: 'Other epic', initiativeId: 'i-b' },
	{ id: 'p-empty', name: 'Empty epic', initiativeId: 'i-a' }
];
const issue = (n, stateId, projectId, title = `Issue ${n}`) => ({
	id: `id-${n}`,
	key: `PLT-${n}`,
	number: n,
	title,
	stateId,
	projectId,
	workspaceId: 'ws-pp'
});
const issues = [
	issue(210, 's-bl', 'p-ui', 'Board screen redesign'),
	issue(208, 's-prog', 'p-ui', 'DoneWhen design system in Claude Design'),
	issue(181, 's-bl', 'p-data'),
	issue(185, 's-prog', 'p-data'),
	issue(187, 's-done', 'p-data'),
	issue(186, 's-can', 'p-data'),
	issue(9, 's-rdy', 'p-z'),
	issue(150, 's-tri', null, 'Loose idea')
];
const blockers = [
	{ issueId: 'id-210', blockerId: 'id-208', done: false },
	{ issueId: 'id-185', blockerId: 'id-187', done: true }
];
const data = { workspaceName: 'Platform', states, initiatives, projects, issues, blockers };
const opts = (o = {}) => ({ cmd: 'tasks', workspace: '', project: '', epic: '', state: '', all: false, json: false, ...o });

test('groups open issues by epic, initiative order then epic name, No epic last', () => {
	const out = render(data, opts());
	const heads = out.split('\n').filter((l) => l && !l.startsWith('  '));
	assert.deepEqual(
		heads.map((h) => h.split(/\s{2,}/)[0]),
		['Platform · 6 open', 'Core — Data integrity', 'Core — UI refresh', 'Other epic', 'No epic']
	);
});

test('epic count covers hidden issues; closed issues hidden by default', () => {
	const out = render(data, opts());
	assert.match(out, /Core — Data integrity\s+1\/4 done/);
	assert.doesNotMatch(out, /PLT-187/);
	assert.doesNotMatch(out, /PLT-186/);
	assert.doesNotMatch(out, /Empty epic/);
});

test('issues ordered by state position then number', () => {
	const out = render(data, opts());
	assert.ok(out.indexOf('PLT-181') < out.indexOf('PLT-185'));
	assert.ok(out.indexOf('PLT-210') < out.indexOf('PLT-208')); // Backlog before In Progress
});

test('open blocker marked, done blocker not', () => {
	const out = render(data, opts());
	assert.match(out, /PLT-210 .*Board screen redesign  ⊘ PLT-208$/m);
	assert.doesNotMatch(out.split('\n').find((l) => l.includes('PLT-185')), /⊘/);
});

test('--all shows closed issues and the header says issues', () => {
	const out = render(data, opts({ all: true }));
	assert.match(out, /^Platform · 8 issues/);
	assert.match(out, /● PLT-187/);
	assert.match(out, /× PLT-186/);
});

test('--epic, --state and --project filter', () => {
	const e = render(data, opts({ epic: 'ui' }));
	assert.match(e, /Core — UI refresh/);
	assert.doesNotMatch(e, /Data integrity|No epic/);

	const s = render(data, opts({ state: 'in progress' }));
	assert.match(s, /PLT-208/);
	assert.match(s, /PLT-185/);
	assert.doesNotMatch(s, /PLT-210|PLT-181|PLT-150/);

	const p = render(data, opts({ project: 'zeta' }));
	assert.match(p, /Other epic/);
	assert.doesNotMatch(p, /DoneWhen|No epic/);
});

test('nothing matches → friendly line', () => {
	assert.match(render(data, opts({ epic: 'nope' })), /No open issues match\./);
});

test('long titles truncate to the width with an ellipsis', () => {
	const long = { ...data, issues: [issue(1, 's-bl', 'p-ui', 'x'.repeat(300))], blockers: [] };
	const line = render(long, opts(), 100).split('\n').find((l) => l.includes('PLT-1'));
	assert.equal([...line].length, 100);
	assert.ok(line.endsWith('…'));
	assert.equal(truncate('abc', 5), 'abc');
	assert.equal(truncate('abcdef', 4), 'abc…');
});

test('glyphs by state', () => {
	assert.equal(glyph(states[0]), '◌');
	assert.equal(glyph(states[1]), '○');
	assert.equal(glyph(states[3]), '◐');
	assert.equal(glyph(states[4]), '⊘');
	assert.equal(glyph(states[5]), '◕');
	assert.equal(glyph(states[6]), '●');
	assert.equal(glyph(states[7]), '×');
});

test('parseArgs', () => {
	assert.deepEqual(parseArgs(['acme', '--all', '--state', 'In Review']), opts({ workspace: 'acme', all: true, state: 'In Review' }));
	assert.throws(() => parseArgs(['--epic']), /needs a value/);
	assert.throws(() => parseArgs(['--bogus']), /Unknown option/);
});

test('findConfig walks up to .claude/donewhen.json', () => {
	const root = mkdtempSync(join(tmpdir(), 'donewhen-'));
	mkdirSync(join(root, '.claude'));
	mkdirSync(join(root, 'a', 'b'), { recursive: true });
	writeFileSync(join(root, '.claude', 'donewhen.json'), '{"workspace":"platform"}');
	assert.deepEqual(findConfig(join(root, 'a', 'b')), { workspace: 'platform' });
});

// ---- run(): wiring and error messages, with a fake fetch ----

const json = (status, body) => ({ ok: status < 300, status, json: async () => body });
const fakeFetch = (routes, seen = []) => async (url, init) => {
	const path = new URL(url).pathname;
	seen.push({ path, ws: init.headers['X-Workspace'] });
	const r = routes[path];
	if (!r) return json(404, { error: 'not found' });
	return typeof r === 'function' ? r(init) : r;
};
const okRoutes = {
	'/api/states': json(200, states),
	'/api/initiatives': json(200, initiatives),
	'/api/projects': json(200, projects),
	'/api/issues': json(200, issues),
	'/api/blockers': json(200, blockers),
	'/api/workspaces': json(200, [{ id: 'ws-pp', slug: 'platform', keyPrefix: 'PLT', name: 'Platform' }])
};
const env = { DONEWHEN_TOKEN: 't', DONEWHEN_URL: 'http://donewhen.test' };
const noCfg = mkdtempSync(join(tmpdir(), 'donewhen-nocfg-'));

test('run: no server URL', async () => {
	assert.equal(
		await run([], { DONEWHEN_TOKEN: 't' }, noCfg),
		'Set DONEWHEN_URL (the address of your DoneWhen server, e.g. https://tracker.example.com)'
	);
});

test('run: no token', async () => {
	assert.equal(await run([], {}, noCfg), 'Set DONEWHEN_TOKEN (mint one: donewhen token <name> [workspace])');
});

test('run: happy path sends the workspace header', async () => {
	const seen = [];
	const out = await run(['platform'], env, noCfg, fakeFetch(okRoutes, seen));
	assert.match(out, /^Platform · 6 open/);
	assert.ok(seen.filter((s) => s.path === '/api/issues').every((s) => s.ws === 'platform'));
});

test('run: repo config supplies workspace and project', async () => {
	const root = mkdtempSync(join(tmpdir(), 'donewhen-cfg-'));
	mkdirSync(join(root, '.claude'));
	writeFileSync(join(root, '.claude', 'donewhen.json'), '{"workspace":"platform","project":"zeta"}');
	const seen = [];
	const out = await run([], env, root, fakeFetch(okRoutes, seen));
	assert.ok(seen.some((s) => s.ws === 'platform'));
	assert.match(out, /Other epic/);
	assert.doesNotMatch(out, /DoneWhen —/);
});

test('run: unreachable', async () => {
	const out = await run([], env, noCfg, async () => {
		throw new TypeError('fetch failed');
	});
	assert.equal(out, 'DoneWhen unreachable at http://donewhen.test — is the server running and reachable?');
});

test('run: 401', async () => {
	const out = await run([], env, noCfg, fakeFetch({ ...okRoutes, '/api/issues': json(401, { error: 'unauthorized' }) }));
	assert.equal(out, 'Token rejected — mint a new one');
});

test('run: 403 lists reachable workspaces', async () => {
	const out = await run(['nope'], env, noCfg, fakeFetch({ ...okRoutes, '/api/issues': json(403, { error: 'not a member of this workspace' }) }));
	assert.match(out, /^No access to workspace 'nope'/);
	assert.match(out, /platform\s+PLT\s+Platform/);
});

test('run: ambiguous workspace asks for one', async () => {
	const out = await run([], env, noCfg, fakeFetch({ ...okRoutes, '/api/issues': json(400, { error: 'workspace must be specified' }) }));
	assert.match(out, /^Several workspaces — pass one: \/tasks <workspace>/);
});

test('run: bad args give usage, not a stack trace', async () => {
	assert.match(await run(['--bogus'], env, noCfg), /^Unknown option --bogus\nUsage:/);
});

// ---- PLT-217: workspaces and use ----

test('parseArgs subcommands', () => {
	assert.equal(parseArgs(['workspaces']).cmd, 'workspaces');
	assert.deepEqual(parseArgs(['use', 'acme', '--project', 'x']), opts({ cmd: 'use', workspace: 'acme', project: 'x' }));
	assert.throws(() => parseArgs(['use']), /use needs a workspace/);
	assert.throws(() => parseArgs(['workspaces', 'acme']), /Unexpected argument/);
});

test('openCounts per workspace and project', () => {
	const c = openCounts(states, initiatives, projects, issues);
	assert.equal(c.open, 6);
	assert.deepEqual(c.projects, [
		{ name: 'Platform', open: 4 },
		{ name: 'Zeta', open: 1 }
	]);
});

test('renderWorkspaceTable marks the default and shows — for no projects', () => {
	const out = renderWorkspaceTable(
		[
			{ ws: { id: 'a', slug: 'platform', keyPrefix: 'PLT' }, open: 29, projects: [{ name: 'Platform', open: 29 }] },
			{ ws: { id: 'b', slug: 'unsorted', keyPrefix: 'UNS' }, open: 3, projects: [] }
		],
		'a'
	);
	const lines = out.split('\n');
	assert.match(lines[0], /^  Workspace\s+Prefix\s+Open\s+Projects$/);
	assert.match(lines[1], /^▸ platform\s+PLT\s+29  Platform \(29\)$/);
	assert.match(lines[2], /^  unsorted\s+UNS\s+3  —$/);
});

const two = [
	{ id: 'ws-pp', slug: 'platform', keyPrefix: 'PLT', name: 'Platform' },
	{ id: 'ws-glx', slug: 'acme', keyPrefix: 'ACM', name: 'Acme' }
];
// Per-workspace fake: the X-Workspace header picks the data set.
const multiFetch = (byWs) => async (url, init) => {
	const path = new URL(url).pathname;
	if (path === '/api/workspaces') return json(200, two);
	const ws = init.headers['X-Workspace'];
	const key = two.find((w) => w.id === ws || w.slug === ws)?.id;
	const d = byWs[key];
	if (!d) return json(403, { error: 'not a member of this workspace' });
	return json(200, d[path.slice(5)] ?? []);
};
const ppData = { states, initiatives, projects, issues, blockers };
const lumData = {
	states,
	initiatives: [{ id: 'li', name: 'Acme', position: 0 }, { id: 'lj', name: 'Acme Labs', position: 1 }],
	projects: [{ id: 'lp', name: 'Kernel', initiativeId: 'li' }],
	issues: [{ ...issue(7, 's-rdy', 'lp', 'Kernel thing'), key: 'ACM-7', workspaceId: 'ws-glx' }],
	blockers: []
};
const byWs = { 'ws-pp': ppData, 'ws-glx': lumData };
const repo = () => {
	const root = mkdtempSync(join(tmpdir(), 'donewhen-repo-'));
	return { root, home: mkdtempSync(join(tmpdir(), 'donewhen-home-')), gitRoot: () => root };
};

test('run workspaces: every workspace, default marked', async () => {
	const r = repo();
	mkdirSync(join(r.root, '.claude'));
	writeFileSync(join(r.root, '.claude', 'donewhen.json'), '{"workspace":"acme"}');
	const out = await run(['workspaces'], env, r.root, multiFetch(byWs), r);
	assert.match(out, /^  platform\s+PLT\s+6  Platform \(4\), Zeta \(1\)$/m);
	assert.match(out, /^▸ acme\s+ACM\s+1  Acme \(1\), Acme Labs \(0\)$/m);
});

test('run use: writes the git root config, keeps other keys, prints the view', async () => {
	const r = repo();
	mkdirSync(join(r.root, '.claude'));
	writeFileSync(join(r.root, '.claude', 'donewhen.json'), '{"workspace":"platform","project":"Zeta","keep":1}');
	const out = await run(['use', 'ACM'], env, r.root, multiFetch(byWs), r);
	const cfg = JSON.parse(readFileSync(join(r.root, '.claude', 'donewhen.json'), 'utf8'));
	assert.deepEqual(cfg, { workspace: 'acme', keep: 1 }); // project cleared
	assert.match(out, /→ Acme\n\nAcme · 1 open/);
	// next plain /tasks uses the new default
	assert.match(await run([], env, r.root, multiFetch(byWs), r), /^Acme · 1 open/);
});

test('run use --project: exact name wins, stored, view scoped', async () => {
	const r = repo();
	const out = await run(['use', 'acme', '--project', 'acme'], env, r.root, multiFetch(byWs), r);
	const cfg = JSON.parse(readFileSync(join(r.root, '.claude', 'donewhen.json'), 'utf8'));
	assert.deepEqual(cfg, { workspace: 'acme', project: 'Acme' });
	assert.match(out, /→ Acme › Acme/);
	assert.match(out, /Kernel/);
});

test('run use: refusals leave the file untouched', async () => {
	const r = repo();
	const f = join(r.root, '.claude', 'donewhen.json');
	assert.match(await run(['use', 'nope'], env, r.root, multiFetch(byWs), r), /^No access to workspace 'nope'\n  platform/);
	assert.match(await run(['use', 'acme', '--project', 'xyz'], env, r.root, multiFetch(byWs), r), /^No project matching 'xyz' in acme\n  Acme\n  Acme Labs/);
	assert.match(await run(['use', 'platform', '--project', 'a'], env, r.root, multiFetch(byWs), r), /^Several projects match 'a'/);
	assert.equal(existsSync(f), false);
});

test('run use outside a git repo writes the user-wide default', async () => {
	const r = repo();
	await run(['use', 'acme'], env, r.root, multiFetch(byWs), { home: r.home, gitRoot: () => null });
	const cfg = JSON.parse(readFileSync(join(r.home, '.config', 'donewhen', 'default.json'), 'utf8'));
	assert.equal(cfg.workspace, 'acme');
	// and a plain /tasks anywhere picks it up
	assert.match(await run([], env, r.root, multiFetch(byWs), { home: r.home }), /^Acme · 1 open/);
});

// ---- compatibility with the pre-rename (Raenil) names ----

test('compat: old RAENIL_TOKEN / RAENIL_URL still work, new names win', async () => {
	const old = { RAENIL_TOKEN: 't', RAENIL_URL: 'http://old.test' };
	const out = await run(['platform'], old, noCfg, fakeFetch(okRoutes));
	assert.match(out, /^Platform · 6 open/);
	const hosts = [];
	const both = { ...old, DONEWHEN_TOKEN: 't2', DONEWHEN_URL: 'http://new.test' };
	await run(['platform'], both, noCfg, async (url, init) => {
		hosts.push(new URL(url).host);
		assert.equal(init.headers.Authorization, 'Bearer t2');
		return fakeFetch(okRoutes)(url, init);
	});
	assert.ok(hosts.length > 0 && hosts.every((h) => h === 'new.test'));
});

test('compat: reads .claude/raenil.json and ~/.config/raenil/default.json; new files win', () => {
	const root = mkdtempSync(join(tmpdir(), 'compat-repo-'));
	const home = mkdtempSync(join(tmpdir(), 'compat-home-'));
	mkdirSync(join(root, '.claude'));
	writeFileSync(join(root, '.claude', 'raenil.json'), '{"workspace":"old-repo"}');
	assert.deepEqual(findConfig(root, home), { workspace: 'old-repo' });
	writeFileSync(join(root, '.claude', 'donewhen.json'), '{"workspace":"new-repo"}');
	assert.deepEqual(findConfig(root, home), { workspace: 'new-repo' });

	const bare = mkdtempSync(join(tmpdir(), 'compat-bare-'));
	mkdirSync(join(home, '.config', 'raenil'), { recursive: true });
	writeFileSync(join(home, '.config', 'raenil', 'default.json'), '{"workspace":"old-user"}');
	assert.deepEqual(findConfig(bare, home), { workspace: 'old-user' });
	mkdirSync(join(home, '.config', 'donewhen'), { recursive: true });
	writeFileSync(join(home, '.config', 'donewhen', 'default.json'), '{"workspace":"new-user"}');
	assert.deepEqual(findConfig(bare, home), { workspace: 'new-user' });
});

// ---- interactive mode: --json and outline ----

test('parseArgs: outline and --json', () => {
	assert.equal(parseArgs(['outline', 'platform']).cmd, 'outline');
	assert.equal(parseArgs(['outline', 'platform']).workspace, 'platform');
	assert.equal(parseArgs(['workspaces', '--json']).json, true);
	assert.equal(parseArgs(['platform']).json, false);
});

test('run workspaces --json: slug, prefix, open and projects, most open first, default named', async () => {
	const r = repo();
	mkdirSync(join(r.root, '.claude'));
	writeFileSync(join(r.root, '.claude', 'donewhen.json'), '{"workspace":"acme"}');
	const data = JSON.parse(await run(['workspaces', '--json'], env, r.root, multiFetch(byWs), r));
	assert.equal(data.current, 'acme');
	assert.deepEqual(data.workspaces.map((w) => w.slug), ['platform', 'acme']);
	assert.equal(data.workspaces[0].open, 6);
	assert.deepEqual(data.workspaces[1].projects[0], { name: 'Acme', open: 1 });
});

test('run outline: projects, epics by open count, states and the tickets that need attention', async () => {
	const r = repo();
	const o = JSON.parse(await run(['outline', 'platform'], env, r.root, multiFetch(byWs), r));
	assert.equal(o.open, 6);
	assert.ok(o.epics.length > 0 && o.epics.every((e) => e.open > 0));
	assert.deepEqual(o.epics.map((e) => e.open), [...o.epics.map((e) => e.open)].sort((a, b) => b - a));
	assert.ok(o.states.every((s) => s.open > 0));
	const rank = { Blocked: 0, 'In Review': 1, 'In Progress': 2 };
	assert.ok(o.hot.every((h) => h.state in rank));
	assert.deepEqual(o.hot.map((h) => rank[h.state]), [...o.hot.map((h) => rank[h.state])].sort((a, b) => a - b));
});

test('run --json: a failure is JSON too, never prose', async () => {
	const r = repo();
	const out = await run(['outline', 'nope'], env, r.root, multiFetch(byWs), r);
	assert.ok(JSON.parse(out).error);
});
