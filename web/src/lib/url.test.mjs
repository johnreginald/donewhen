// Run with: npm test
import test from 'node:test';
import assert from 'node:assert/strict';
import { safeHref, safeLinkHref } from './url.js';

test('safeHref allows only absolute http(s) URLs', () => {
	assert.equal(safeHref('https://github.com/x/y/pull/1'), 'https://github.com/x/y/pull/1');
	assert.equal(safeHref(' http://localhost:3000/a '), 'http://localhost:3000/a');
	for (const bad of [
		'javascript:alert(1)',
		'JaVaScRiPt:alert(1)',
		'java\nscript:alert(1)',
		'data:text/html,<script>1</script>',
		'vbscript:x',
		'file:///etc/passwd',
		'/relative/path',
		'github.com/x/y',
		'https://',
		'',
		null,
		undefined,
		42
	]) {
		assert.equal(safeHref(bad), null, String(bad));
	}
});

test('safeLinkHref keeps http(s), mailto, anchors and relative links', () => {
	for (const ok of ['https://a.b/c', 'http://a.b', 'mailto:a@b.c', '#top', '/issue/PP-1', 'PP-1', '../x']) {
		assert.equal(safeLinkHref(ok), ok);
	}
	for (const bad of ['javascript:alert(1)', ' JAVASCRIPT:alert(1)', 'java\tscript:alert(1)', 'data:text/html,x', 'vbscript:x', 'file:///x', 'blob:abc']) {
		assert.equal(safeLinkHref(bad), '#', JSON.stringify(bad));
	}
});
