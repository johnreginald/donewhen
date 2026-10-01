import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	SHORTCUTS,
	groupShortcuts,
	isTypingTarget,
	stepIndex,
	wrapIndex,
	createChord,
	actionFor
} from './shortcuts.js';

test('isTypingTarget covers fields and contenteditable', () => {
	assert.equal(isTypingTarget({ tagName: 'INPUT' }), true);
	assert.equal(isTypingTarget({ tagName: 'TEXTAREA' }), true);
	assert.equal(isTypingTarget({ tagName: 'SELECT' }), true);
	assert.equal(isTypingTarget({ tagName: 'DIV', isContentEditable: true }), true);
	assert.equal(isTypingTarget({ tagName: 'BUTTON' }), false);
	assert.equal(isTypingTarget(null), false);
});

test('j three times from nothing focuses the third item', () => {
	let i = -1;
	for (let n = 0; n < 3; n++) i = stepIndex(i, 5, 1);
	assert.equal(i, 2);
});

test('stepIndex clamps at both ends and handles empty lists', () => {
	assert.equal(stepIndex(4, 5, 1), 4);
	assert.equal(stepIndex(0, 5, -1), 0);
	assert.equal(stepIndex(-1, 5, -1), 0);
	assert.equal(stepIndex(-1, 0, 1), -1);
});

test('wrapIndex wraps menu arrows', () => {
	assert.equal(wrapIndex(2, 3, 1), 0);
	assert.equal(wrapIndex(0, 3, -1), 2);
	assert.equal(wrapIndex(0, 0, 1), -1);
});

test('chord is live inside its window and one-shot', () => {
	const c = createChord(1000);
	c.arm(100);
	assert.equal(c.armed, true);
	assert.equal(c.take(900), true);
	assert.equal(c.take(901), false); // already used
	c.arm(0);
	assert.equal(c.take(1500), false); // too late
	assert.equal(c.armed, false);
});

test('registry: the overlay lists every shortcut, ids are unique', () => {
	const groups = groupShortcuts();
	assert.equal(groups.reduce((n, g) => n + g.items.length, 0), SHORTCUTS.length);
	const ids = SHORTCUTS.filter((s) => s.id).map((s) => s.id);
	assert.equal(new Set(ids).size, ids.length);
	for (const s of SHORTCUTS) assert.ok(s.keys.length && s.label && s.group);
});

test('registry has the spec keys', () => {
	for (const id of ['next', 'prev', 'menu-status', 'menu-priority', 'menu-label', 'menu-epic', 'capture', 'help', 'back', 'go-board', 'go-inbox', 'go-list']) {
		assert.ok(actionFor(id), id);
	}
	assert.deepEqual(actionFor('go-inbox').keys, ['G', 'I']);
});
