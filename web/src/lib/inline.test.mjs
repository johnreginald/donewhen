import test from 'node:test';
import assert from 'node:assert/strict';
import { parseInline } from './inline.js';

test('plain text stays one part', () => {
	assert.deepEqual(parseInline('hello'), [{ kind: 'text', text: 'hello' }]);
});

test('code and bold are split out', () => {
	assert.deepEqual(parseInline('has **Continue with GitHub** and `make test` ok'), [
		{ kind: 'text', text: 'has ' },
		{ kind: 'bold', text: 'Continue with GitHub' },
		{ kind: 'text', text: ' and ' },
		{ kind: 'code', text: 'make test' },
		{ kind: 'text', text: ' ok' }
	]);
});

test('bold marks inside code stay code', () => {
	assert.deepEqual(parseInline('`**x**`'), [{ kind: 'code', text: '**x**' }]);
});

test('an unmatched pair of stars stays plain', () => {
	assert.deepEqual(parseInline('a ** b'), [{ kind: 'text', text: 'a ** b' }]);
});

test('empty and missing input give no parts', () => {
	assert.deepEqual(parseInline(''), []);
	assert.deepEqual(parseInline(undefined), []);
});
