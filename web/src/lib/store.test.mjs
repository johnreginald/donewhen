// Run with: npm test
// Drives the real store against a stubbed fetch.
import test from 'node:test';
import assert from 'node:assert/strict';
import { get } from 'svelte/store';

globalThis.document = { cookie: '' };
const calls = [];
let respond = () => ({ status: 200, body: {} });
globalThis.fetch = async (url, init = {}) => {
	calls.push({ url, method: init.method || 'GET', body: init.body ? JSON.parse(init.body) : undefined });
	const r = respond(url, init);
	return {
		status: r.status,
		ok: r.status < 400,
		statusText: 'x',
		json: async () => r.body
	};
};

const store = await import('./store.js');
const { issues, allIssues, activeWorkspace, activeProject, projects, applyEvent, moveIssueTo, inboxTotal } = store;

const mk = (id, stateId, position, extra = {}) => ({
	id,
	key: 'T-' + id,
	title: id,
	stateId,
	position,
	workspaceId: 'ws1',
	projectId: 'p1',
	labels: [],
	...extra
});

function reset(list) {
	calls.length = 0;
	activeWorkspace.set({ id: 'ws1' });
	activeProject.set('');
	projects.set([{ id: 'p1' }, { id: 'p2' }]);
	issues.set(list);
	allIssues.set(list);
}

test('a move sends exactly one request, with the neighbours', async () => {
	const col = Array.from({ length: 30 }, (_, i) => mk('c' + i, 's1', i));
	reset(col);
	respond = (url) => ({ status: 200, body: mk('c5', 's2', 3.5) });
	await moveIssueTo('c5', 's2', col[3], col[4]);
	assert.equal(calls.length, 1);
	assert.equal(calls[0].method, 'POST');
	assert.match(calls[0].url, /\/issues\/c5\/move$/);
	assert.deepEqual(calls[0].body, { state: 's2', after: 'c3', before: 'c4' });
	assert.equal(get(issues).find((i) => i.id === 'c5').stateId, 's2');
});

test('a failed move puts the card back and does not edit store objects in place', async () => {
	const col = [mk('a', 's1', 1), mk('b', 's1', 2), mk('c', 's1', 3)];
	reset(col);
	const before = get(issues).find((i) => i.id === 'a');
	respond = () => ({ status: 500, body: { error: 'boom' } });
	await assert.rejects(moveIssueTo('a', 's2', col[1], col[2]), /boom/);
	const after = get(issues).find((i) => i.id === 'a');
	assert.equal(after.stateId, 's1');
	assert.equal(after.position, 1);
	assert.equal(before.stateId, 's1'); // the original object was never mutated
	assert.equal(calls.length, 1); // no refetch afterwards
});

test('live events for a card mid-move are ignored', async () => {
	const col = [mk('a', 's1', 1)];
	reset(col);
	let release;
	respond = () => ({ status: 200, body: mk('a', 's2', 9) });
	const gate = new Promise((r) => (release = r));
	const realFetch = globalThis.fetch;
	globalThis.fetch = async (...args) => {
		await gate;
		return realFetch(...args);
	};
	const p = moveIssueTo('a', 's2', null, null);
	applyEvent({ type: 'issue.updated', workspaceId: 'ws1', issue: mk('a', 's1', 1, { title: 'stale' }) });
	assert.equal(get(issues)[0].stateId, 's2'); // optimistic move survived the echo
	release();
	await p;
	globalThis.fetch = realFetch;
	assert.equal(get(issues)[0].position, 9);
});

test('events: other workspace ignored; outside the epic filter not added', () => {
	reset([mk('a', 's1', 1)]);
	applyEvent({ type: 'issue.created', workspaceId: 'ws2', issue: mk('x', 's1', 1, { workspaceId: 'ws2' }) });
	assert.equal(get(issues).length, 1);
	activeProject.set('p1');
	applyEvent({ type: 'issue.created', workspaceId: 'ws1', issue: mk('y', 's1', 2, { projectId: 'p2' }) });
	assert.equal(get(issues).length, 1);
	assert.equal(get(allIssues).length, 2); // the unfiltered list still tracks it
	applyEvent({ type: 'issue.created', workspaceId: 'ws1', issue: mk('z', 's1', 3) });
	assert.equal(get(issues).length, 2);
	// moving an issue out of the filtered epic removes it from the list
	applyEvent({ type: 'issue.updated', workspaceId: 'ws1', issue: mk('z', 's1', 3, { projectId: 'p2' }) });
	assert.equal(get(issues).length, 1);
});

test('inbox formula is needsReview + waiting', () => {
	assert.equal(inboxTotal({ needsReview: [1, 2], waiting: [3] }), 3);
	assert.equal(inboxTotal(null), 0);
});
