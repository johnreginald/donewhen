// Run with: npm test
import test from 'node:test';
import assert from 'node:assert/strict';
import { connectSSE, BACKOFF_MS, OFFLINE_AFTER_MS } from './sse.js';
import { coalesce, belongsInView } from './live.js';

// A stand-in for the browser's EventSource that the test drives by hand.
class FakeES {
	static all = [];
	constructor(url) {
		this.url = url;
		this.readyState = 0;
		this.handlers = {};
		FakeES.all.push(this);
	}
	addEventListener(t, fn) {
		(this.handlers[t] ||= []).push(fn);
	}
	close() {
		this.readyState = 2;
	}
	emit(t, data) {
		for (const fn of this.handlers[t] || []) fn({ data: JSON.stringify(data) });
	}
	open() {
		this.readyState = 1;
		this.onopen();
	}
	fail(closed) {
		this.readyState = closed ? 2 : 0;
		this.onerror();
	}
}

function setup(t) {
	FakeES.all = [];
	globalThis.EventSource = FakeES;
	t.mock.timers.enable({ apis: ['setTimeout', 'Date'] });
	const log = { status: [], events: [], catchUp: 0 };
	const close = connectSSE('ws-b', {
		onEvent: (e) => log.events.push(e),
		onStatus: (s) => log.status.push(s),
		onCatchUp: () => log.catchUp++
	});
	t.after(() => {
		close();
		delete globalThis.EventSource;
	});
	return log;
}

test('stream URL carries the workspace and exactly one stream opens', (t) => {
	setup(t);
	assert.equal(FakeES.all.length, 1);
	assert.equal(FakeES.all[0].url, '/api/events?workspace=ws-b');
});

test('first open does not catch up; a reconnect does', (t) => {
	const log = setup(t);
	FakeES.all[0].open();
	assert.equal(log.catchUp, 0);
	FakeES.all[0].fail(false);
	assert.equal(log.status.at(-1), 'reconnecting');
	FakeES.all[0].open();
	assert.equal(log.status.at(-1), 'live');
	assert.equal(log.catchUp, 1);
});

test('resync event triggers one catch-up', (t) => {
	const log = setup(t);
	FakeES.all[0].open();
	FakeES.all[0].emit('resync', {});
	assert.equal(log.catchUp, 1);
});

test('events are passed through', (t) => {
	const log = setup(t);
	FakeES.all[0].emit('issue.updated', { type: 'issue.updated' });
	assert.equal(log.events.length, 1);
});

test('a closed stream reopens with backoff 1s, 2s, 5s, 10s, then 30s', (t) => {
	setup(t);
	let want = 1;
	for (const ms of BACKOFF_MS) {
		FakeES.all.at(-1).fail(true);
		t.mock.timers.tick(ms - 1);
		assert.equal(FakeES.all.length, want, `not yet at ${ms}`);
		t.mock.timers.tick(1);
		want++;
		assert.equal(FakeES.all.length, want, `reopened at ${ms}`);
	}
	// stays at 30s
	FakeES.all.at(-1).fail(true);
	t.mock.timers.tick(30000);
	assert.equal(FakeES.all.length, want + 1);
});

test('status goes offline after 30s without a connection, live again on open', (t) => {
	const log = setup(t);
	FakeES.all[0].open();
	FakeES.all[0].fail(false);
	t.mock.timers.tick(OFFLINE_AFTER_MS);
	assert.equal(log.status.at(-1), 'offline');
	FakeES.all[0].open();
	assert.equal(log.status.at(-1), 'live');
	assert.equal(log.catchUp, 1);
});

test('coalesce: calls during a run share it', async () => {
	let runs = 0;
	let release;
	const gate = new Promise((r) => (release = r));
	const f = coalesce(async () => {
		runs++;
		await gate;
	});
	const a = f();
	const b = f();
	assert.equal(a, b);
	release();
	await a;
	await f();
	assert.equal(runs, 2);
});

test('belongsInView: workspace and filters', () => {
	const projects = [
		{ id: 'p1', initiativeId: 'i1' },
		{ id: 'p2', initiativeId: 'i2' }
	];
	const iss = { workspaceId: 'a', projectId: 'p1', labels: [{ id: 'l1' }] };
	assert.equal(belongsInView(iss, { workspaceId: 'a', projects }), true);
	assert.equal(belongsInView(iss, { workspaceId: 'b', projects }), false);
	assert.equal(belongsInView(iss, { workspaceId: 'a', project: 'p2', projects }), false);
	assert.equal(belongsInView(iss, { workspaceId: 'a', project: 'p1', projects }), true);
	assert.equal(belongsInView(iss, { workspaceId: 'a', initiative: 'i2', projects }), false);
	assert.equal(belongsInView(iss, { workspaceId: 'a', initiative: 'i1', projects }), true);
	assert.equal(belongsInView(iss, { workspaceId: 'a', label: 'l2', projects }), false);
	assert.equal(belongsInView(iss, { workspaceId: 'a', label: 'l1', projects }), true);
});
