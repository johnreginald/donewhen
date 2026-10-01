import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	parseFilters,
	serializeFilters,
	hasFilterParams,
	matchesFilters,
	textMatches,
	toggle,
	sameList
} from './filters.js';

const workflow = [
	{ id: 's1', name: 'Ready' },
	{ id: 's2', name: 'In Review' },
	{ id: 's3', name: 'Done' }
];

test('parseFilters reads the spec example', () => {
	assert.deepEqual(parseFilters('?state=Ready,In+Review&priority=1,2'), {
		states: ['Ready', 'In Review'],
		priorities: [1, 2],
		labels: [],
		project: '',
		q: ''
	});
});

test('parseFilters drops bad priorities and blanks', () => {
	const f = parseFilters('priority=1,x,9,1,,0&state=,Ready,');
	assert.deepEqual(f.priorities, [1, 0]);
	assert.deepEqual(f.states, ['Ready']);
});

test('serialize then parse round-trips', () => {
	const f = { states: ['In Review', 'Ready'], priorities: [2, 1], labels: ['l1'], project: 'p1', q: '50% done' };
	const s = serializeFilters(f);
	assert.equal(s, 'state=In+Review,Ready&priority=1,2&label=l1&project=p1&q=50%25+done');
	assert.deepEqual(parseFilters(s), { ...f, priorities: [1, 2] });
});

test('serialize is empty without filters and ignores blank text', () => {
	assert.equal(serializeFilters({}), '');
	assert.equal(serializeFilters({ q: '   ' }), '');
});

test('a comma inside a value cannot split the list', () => {
	const s = serializeFilters({ states: ['a,b'] });
	assert.deepEqual(parseFilters(s).states, ['a b']);
});

test('hasFilterParams', () => {
	assert.equal(hasFilterParams('?state=Ready'), true);
	assert.equal(hasFilterParams('?foo=1'), false);
	assert.equal(hasFilterParams(''), false);
});

test('matchesFilters: OR within, AND across', () => {
	const i = { stateId: 's1', priority: 1, labels: [{ id: 'l1' }] };
	assert.equal(matchesFilters(i, { states: ['ready'] }, workflow), true);
	assert.equal(matchesFilters(i, { states: ['Done', 'In Review'] }, workflow), false);
	assert.equal(matchesFilters(i, { states: ['s1'] }, workflow), true); // by id
	assert.equal(matchesFilters(i, { priorities: [1, 2] }, workflow), true);
	assert.equal(matchesFilters(i, { priorities: [3] }, workflow), false);
	assert.equal(matchesFilters(i, { labels: ['l2', 'l1'] }, workflow), true);
	assert.equal(matchesFilters(i, { labels: ['l2'] }, workflow), false);
	assert.equal(matchesFilters(i, { states: ['Ready'], priorities: [2] }, workflow), false);
	assert.equal(matchesFilters({ stateId: 's1' }, { priorities: [0] }, workflow), true); // no priority = 0
	assert.equal(matchesFilters(i, {}, workflow), true);
});

test('textMatches is literal', () => {
	const i = { title: 'Raise to 50% now', key: 'PP-9' };
	assert.equal(textMatches(i, '50%'), true);
	assert.equal(textMatches(i, '50x'), false);
	assert.equal(textMatches(i, 'pp-9'), true);
	assert.equal(textMatches(i, ''), true);
});

test('toggle and sameList', () => {
	assert.deepEqual(toggle(['a'], 'b'), ['a', 'b']);
	assert.deepEqual(toggle(['a', 'b'], 'a'), ['b']);
	assert.equal(sameList(['a', 'b'], ['b', 'a']), true);
	assert.equal(sameList(['a'], ['a', 'b']), false);
});
