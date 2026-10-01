import { test } from 'node:test';
import assert from 'node:assert/strict';
import { needsReason, checkReason, REASON_MAX } from './blocked.js';

const states = [
	{ id: 'a', name: 'In Progress' },
	{ id: 'b', name: 'Blocked' },
	{ id: 'c', name: 'Done' }
];

test('needsReason only when entering Blocked from another state', () => {
	assert.equal(needsReason(states, 'a', 'b'), true);
	assert.equal(needsReason(states, 'b', 'b'), false);
	assert.equal(needsReason(states, 'b', 'a'), false);
	assert.equal(needsReason(states, 'a', 'c'), false);
	assert.equal(needsReason([], 'a', 'b'), false);
});

test('checkReason trims and rejects empty or too long', () => {
	assert.deepEqual(checkReason('  waiting on keys '), { reason: 'waiting on keys' });
	assert.ok(checkReason('   ').error);
	assert.ok(checkReason(undefined).error);
	assert.ok(checkReason('x'.repeat(REASON_MAX + 1)).error);
	assert.equal(checkReason('x'.repeat(REASON_MAX)).reason.length, REASON_MAX);
});
