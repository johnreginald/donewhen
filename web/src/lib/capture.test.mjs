import { test } from 'node:test';
import assert from 'node:assert/strict';
import { validateTitle, toggleLabel, hasDraft, TITLE_MAX } from './capture.js';

test('validateTitle trims and rejects empty or too long', () => {
	assert.deepEqual(validateTitle('  fix it  '), { title: 'fix it' });
	assert.ok(validateTitle('   ').error);
	assert.ok(validateTitle(undefined).error);
	assert.ok(validateTitle('x'.repeat(TITLE_MAX + 1)).error);
	assert.equal(validateTitle('x'.repeat(TITLE_MAX)).title.length, TITLE_MAX);
});

const groups = [
	{ id: 'g1', name: 'type', exclusive: true },
	{ id: 'g2', name: 'repo', exclusive: false }
];
const labels = [
	{ id: 'bug', groupId: 'g1' },
	{ id: 'feature', groupId: 'g1' },
	{ id: 'web', groupId: 'g2' },
	{ id: 'api', groupId: 'g2' }
];

test('toggleLabel enforces exclusive groups', () => {
	assert.deepEqual(toggleLabel([], 'bug', labels, groups), ['bug']);
	assert.deepEqual(toggleLabel(['bug'], 'feature', labels, groups), ['feature']);
	assert.deepEqual(toggleLabel(['bug'], 'bug', labels, groups), []);
});

test('toggleLabel stacks labels in a non-exclusive group', () => {
	assert.deepEqual(toggleLabel(['web', 'bug'], 'api', labels, groups), ['web', 'bug', 'api']);
});

test('hasDraft', () => {
	assert.equal(hasDraft('', '  '), false);
	assert.equal(hasDraft('a', ''), true);
	assert.equal(hasDraft('', 'notes'), true);
});
