import test from 'node:test';
import assert from 'node:assert/strict';
import { ownsData, needsMetadata, stripWorkspaceParams } from './workspace_pages.js';

test('pages that load their own data are mounted again, also with a trailing slash', () => {
	for (const p of ['/inbox', '/blocked', '/blocked/', '/artifacts', '/artifacts/', '/log', '/settings/labels', '/settings/members', '/settings', '/some-new-page', '/']) {
		assert.equal(ownsData(p), true, p);
	}
});

test('store-only pages stay mounted', () => {
	for (const p of ['/board', '/board/', '/list', '/by-epic', '/by-epic/', '/links', '/tasks', '/issue/DW-1', '/issue/DW-1/']) {
		assert.equal(ownsData(p), false, p);
	}
});

test('a path that only starts with a store-only name is not store-only', () => {
	assert.equal(ownsData('/boarding'), true);
	assert.equal(ownsData('/listing'), true);
	assert.equal(ownsData('/issues-archive'), true);
});

test('only Inbox and Blocked wait for the metadata', () => {
	for (const p of ['/inbox', '/inbox/', '/blocked', '/blocked/']) assert.equal(needsMetadata(p), true, p);
	for (const p of ['/log', '/artifacts', '/settings/labels', '/board', '/', '']) assert.equal(needsMetadata(p), false, p);
});

test('stripWorkspaceParams removes doc and peek and keeps the rest', () => {
	const u = (s) => new URL(s, 'http://x.test');
	assert.equal(stripWorkspaceParams(u('/artifacts?doc=abc')), '/artifacts');
	assert.equal(stripWorkspaceParams(u('/artifacts?doc=abc&view=wide#top')), '/artifacts?view=wide#top');
	assert.equal(stripWorkspaceParams(u('/board?peek=DW-1&state=Ready')), '/board?state=Ready');
	assert.equal(stripWorkspaceParams(u('/artifacts?doc=a&peek=b')), '/artifacts');
});

test('stripWorkspaceParams gives null when there is nothing to remove', () => {
	const u = (s) => new URL(s, 'http://x.test');
	assert.equal(stripWorkspaceParams(u('/artifacts')), null);
	assert.equal(stripWorkspaceParams(u('/board?state=Ready#x')), null);
});
