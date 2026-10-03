// Run with: npm test
// Error handling in the API client and the load guards in the store.
import test from 'node:test';
import assert from 'node:assert/strict';
import { get } from 'svelte/store';
import { safeNext } from './url.js';

globalThis.document = { cookie: '' };
globalThis.localStorage = {
	data: {},
	getItem(k) {
		return this.data[k] ?? null;
	},
	setItem(k, v) {
		this.data[k] = String(v);
	},
	removeItem(k) {
		delete this.data[k];
	}
};
const assigned = [];
globalThis.location = { pathname: '/board', search: '?q=1', assign: (u) => assigned.push(u) };

let handler = () => ({ status: 200, body: {} });
globalThis.fetch = async (url, init = {}) => {
	const r = await handler(url, init);
	if (r === 'down') throw new TypeError('fetch failed');
	return { status: r.status, ok: r.status < 400, statusText: 'x', json: async () => r.body };
};

const { api, setNotifier } = await import('./api.js');
const store = await import('./store.js');

test('safeNext only returns same-site paths', () => {
	assert.equal(safeNext('?next=%2Fboard%3Fx%3D1'), '/board?x=1');
	assert.equal(safeNext('?next=https%3A%2F%2Fevil.test'), '/');
	assert.equal(safeNext('?next=%2F%2Fevil.test'), '/');
	assert.equal(safeNext('?next=%2Flogin'), '/');
	assert.equal(safeNext(''), '/');
});

test('a network failure is not a logout: it throws network, notifies, keeps the page', async () => {
	const seen = [];
	setNotifier((m) => seen.push(m));
	handler = () => 'down';
	await assert.rejects(api.issues(), (e) => e.network === true && e.status === 0);
	assert.equal(seen.length, 1);
	assert.equal(assigned.length, 0);
	setNotifier(null);
});

test('a 5xx is an error with its status and does not redirect', async () => {
	handler = () => ({ status: 503, body: { error: 'down for maintenance' } });
	await assert.rejects(api.issues(), (e) => e.status === 503 && !e.network);
	assert.equal(assigned.length, 0);
});

test('a wrong password on login is a 401 without a redirect', async () => {
	handler = () => ({ status: 401, body: { error: 'bad' } });
	await assert.rejects(api.login('a@b.c', 'x'), (e) => e.status === 401);
	assert.equal(assigned.length, 0);
});

test('a 401 on any other call goes to /login?next=<current path>', async () => {
	handler = () => ({ status: 401, body: {} });
	await assert.rejects(api.issues(), (e) => e.status === 401);
	assert.deepEqual(assigned, ['/login?next=' + encodeURIComponent('/board?q=1')]);
});

test('switching workspace twice fast keeps only the last workspace', async () => {
	const { workspaces, issues, allIssues, switchWorkspace } = store;
	workspaces.set([
		{ id: 'a', slug: 'a' },
		{ id: 'b', slug: 'b' },
		{ id: 'c', slug: 'c' }
	]);
	const latency = { a: 40, b: 5, c: 5 };
	handler = async (url, init) => {
		const ws = init.headers?.['X-Workspace'];
		const path = url.replace('/api', '');
		await new Promise((r) => setTimeout(r, latency[ws] || 0));
		if (path.startsWith('/issues')) return { status: 200, body: [{ id: 'iss-' + ws, key: ws, position: 0 }] };
		if (path.startsWith('/workspaces/')) return { status: 200, body: {} };
		return { status: 200, body: [] };
	};
	const first = switchWorkspace('a'); // slow
	const second = switchWorkspace('b'); // fast, started later
	await Promise.all([first, second]);
	assert.deepEqual(get(issues).map((i) => i.key), ['b']);
	assert.deepEqual(get(allIssues).map((i) => i.key), ['b']);
	assert.equal(get(store.switching), false);
});

test('loadMeta shows what loaded when one part fails, and names the failure', async () => {
	const seen = [];
	setNotifier((m) => seen.push(m));
	handler = (url) => {
		const path = url.replace('/api', '');
		if (path.startsWith('/labels')) return { status: 500, body: { error: 'boom' } };
		if (path.startsWith('/states')) return { status: 200, body: [{ id: 's1', name: 'Todo' }] };
		return { status: 200, body: [] };
	};
	await store.loadMeta();
	assert.deepEqual(get(store.states).map((s) => s.id), ['s1']);
	assert.deepEqual(seen, ["Couldn't load labels"]);
	// every part failing is an error, so boot can show the retry screen
	handler = () => ({ status: 500, body: { error: 'down' } });
	await assert.rejects(store.loadMeta(), /down/);
	setNotifier(null);
});

test('a late inbox response cannot replace the new workspace badge', async () => {
	store.workspaces.set([{ id: 'a', slug: 'a' }, { id: 'b', slug: 'b' }]);
	handler = () => ({ status: 200, body: [] });
	await store.switchWorkspace('a');
	let release;
	handler = (url, init) => {
		if (url === '/api/inbox') {
			if (init.headers['X-Workspace'] === 'a') return new Promise((resolve) => { release = resolve; });
			return { status: 200, body: { needsReview: [{}], waiting: [] } };
		}
		return { status: 200, body: [] };
	};
	const old = store.refreshInbox();
	await store.switchWorkspace('b');
	await store.refreshInbox();
	assert.equal(get(store.inboxCount), 1);
	release({ status: 200, body: { needsReview: Array(7).fill({}), waiting: [] } });
	await old;
	assert.equal(get(store.inboxCount), 1);
});

test('switching workspace clears the old inbox badge before activation completes', async () => {
	store.workspaces.set([{ id: 'a', slug: 'a' }]);
	store.inboxCount.set(7);
	let release;
	handler = (url) => url.includes('/activate')
		? new Promise((resolve) => { release = resolve; })
		: { status: 200, body: [] };
	const switching = store.switchWorkspace('a');
	const count = get(store.inboxCount);
	release({ status: 200, body: {} });
	await switching;
	assert.equal(count, 0);
});

test('workspace switching stays pending until the new metadata is loaded', async () => {
	store.workspaces.set([{ id: 'a', slug: 'a' }]);
	let release, started;
	const metadataStarted = new Promise((resolve) => { started = resolve; });
	handler = (url) => {
		if (url === '/api/states') {
			started();
			return new Promise((resolve) => { release = resolve; });
		}
		return { status: 200, body: [] };
	};
	const pending = store.switchWorkspace('a');
	await metadataStarted;
	const loadingMetadata = get(store.switching);
	release({ status: 200, body: [] });
	await pending;
	assert.equal(loadingMetadata, true);
	assert.equal(get(store.switching), false);
});
