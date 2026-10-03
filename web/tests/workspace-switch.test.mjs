// Run with npm run test:browser. Every API response is fictional; no real data is changed.
import test, { before, after } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chromium } from 'playwright';

let server, browser;
const base = process.env.DONEWHEN_TEST_BASE_URL || 'http://127.0.0.1:4179';
before(async () => {
	if (!process.env.DONEWHEN_TEST_BASE_URL) {
		server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', 'preview', '--host', '127.0.0.1', '--port', '4179', '--strictPort'], { stdio: 'pipe' });
		await new Promise((resolve, reject) => {
			const timer = setTimeout(() => reject(new Error('Preview did not start')), 15000);
			server.once('exit', (code) => { clearTimeout(timer); reject(new Error(`Preview exited: ${code}`)); });
			server.stdout.on('data', (data) => { if (data.toString().includes(base)) { clearTimeout(timer); resolve(); } });
		});
	}
	browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
});
after(async () => { await browser?.close(); server?.kill(); });

const endpoints = { blocked: '/api/blocked', artifacts: '/api/documents', log: '/api/activity' };
const marker = (view, ws) => view === 'log' ? `ACTIVITY_${ws}` : `${view.toUpperCase()}_${ws}`;
async function setup(t, options = {}) {
	const page = await browser.newPage();
	page.setDefaultTimeout(5000);
	t.after(() => page.close());
	const errors = [], calls = [];
	page.on('pageerror', (e) => errors.push(e.message));
	t.after(() => assert.deepEqual(errors, []));
	await page.addInitScript(() => {
		localStorage.setItem('donewhen_log_last_visit:a', '2026-01-01T00:00:00Z');
		localStorage.setItem('donewhen_log_last_visit:b', '2026-02-01T00:00:00Z');
	});
	await page.route('**/api/**', async (route) => {
		const req = route.request(), path = new URL(req.url()).pathname, ws = req.headers()['x-workspace'] || 'a';
		calls.push({ path, ws });
		if (options.delay?.(path, ws)) await new Promise((r) => setTimeout(r, 450));
		if (options.fail?.(path, ws)) return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Test unavailable' }) });
		let body = [];
		if (path === '/api/auth/status') body = { authenticated: true, user: { email: 'test@local' } };
		else if (path === '/api/workspaces') body = ['a', 'b'].map((id) => ({ id, slug: id, name: `Workspace ${id.toUpperCase()}`, keyPrefix: id.toUpperCase(), role: 'owner' }));
		else if (path === '/api/config') body = { baseUrl: base };
		else if (path === '/api/states') body = [{ id: `${ws}-blocked`, name: 'Blocked', category: 'started' }];
		else if (path === '/api/inbox') body = { needsReview: [], waiting: [], recent: [] };
		else if (path === '/api/blocked') body = [{ id: `issue-${ws}`, key: `${ws.toUpperCase()}-1`, title: marker('blocked', ws), stateId: `${ws}-blocked`, since: new Date().toISOString(), reason: 'Waiting', actor: 'human', waitingOn: [] }];
		else if (path === '/api/documents' || path.startsWith('/api/documents/doc-')) {
			const doc = { id: `doc-${ws}`, title: marker('artifacts', ws), bodyMd: `DOCUMENT_BODY_${ws}`, type: 'reference', author: 'human', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), labels: [] };
			body = path === '/api/documents' ? [doc] : doc;
		}
		else if (path === '/api/activity') body = [{ id: `activity-${ws}`, issueId: `issue-${ws}`, issueKey: `${ws.toUpperCase()}-1`, kind: 'commented', actor: 'human', detail: marker('log', ws), createdAt: new Date().toISOString() }];
		else if (path === '/api/events') return route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': mock\n\n' });
		try { await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) }); } catch { /* request cancelled when its page is disposed */ }
	});
	return { page, calls };
}
async function visible(page, text) {
	await page.waitForFunction((text) => document.querySelector('main')?.innerText.includes(text), text);
}
async function absent(page, text) {
	assert.equal((await page.locator('main').innerText()).includes(text), false);
}
async function pick(page, ws) {
	await page.getByTitle('Switch workspace', { exact: true }).click();
	await page.getByRole('menuitemradio').filter({ hasText: `Workspace ${ws.toUpperCase()}` }).click();
}

for (const view of Object.keys(endpoints)) {
	test(`${view}: switch loads B without navigation and clears A`, async (t) => {
		const { page, calls } = await setup(t);
		await page.goto(`${base}/${view}`);
		await visible(page, marker(view, 'a'));
		await pick(page, 'b');
		await visible(page, marker(view, 'b'));
		await absent(page, marker(view, 'a'));
		assert.ok(calls.some((c) => c.path === endpoints[view] && c.ws === 'b'));
		assert.equal(new URL(page.url()).pathname, `/${view}`);
		if (view === 'log') {
			const stamp = await page.evaluate(() => localStorage.getItem('donewhen_log_last_visit:b'));
			assert.notEqual(stamp, '2026-02-01T00:00:00Z');
		}
	});
	test(`${view}: delayed A response cannot return after switching to B`, async (t) => {
		const { page } = await setup(t, { delay: (path, ws) => path === endpoints[view] && ws === 'a' });
		await page.goto(`${base}/${view}`);
		await page.getByTitle('Switch workspace', { exact: true }).waitFor();
		await pick(page, 'b');
		await visible(page, marker(view, 'b'));
		await page.waitForTimeout(600);
		await absent(page, marker(view, 'a'));
	});
	test(`${view}: failed B load never displays A's data`, async (t) => {
		const { page, calls } = await setup(t, { fail: (path, ws) => path === endpoints[view] && ws === 'b' });
		await page.goto(`${base}/${view}`);
		await visible(page, marker(view, 'a'));
		await pick(page, 'b');
		await page.waitForFunction(() => document.querySelector('main')?.innerText.includes('WORKSPACE B'));
		await page.waitForTimeout(250);
		assert.ok(calls.some((c) => c.path === endpoints[view] && c.ws === 'b'));
		await absent(page, marker(view, 'a'));
	});
	test(`${view}: rapid switches wait for the final workspace's metadata`, async (t) => {
		let slow = false;
		const { page } = await setup(t, { delay: (path, ws) => slow && path === '/api/states' && ws === 'a' });
		await page.goto(`${base}/${view}`);
		await visible(page, marker(view, 'a'));
		await pick(page, 'b');
		await visible(page, marker(view, 'b'));
		slow = true;
		await page.keyboard.press('Control+1');
		await page.keyboard.press('Control+2');
		await visible(page, marker(view, 'b'));
		await page.waitForTimeout(600);
		await absent(page, marker(view, 'a'));
	});
}

test('artifacts: preserves initial deep link but clears the old reader on switch', async (t) => {
	const { page, calls } = await setup(t);
	await page.goto(`${base}/artifacts?doc=doc-a&keep=1`);
	await visible(page, 'DOCUMENT_BODY_a');
	assert.equal(new URL(page.url()).searchParams.get('doc'), 'doc-a');
	await pick(page, 'b');
	await visible(page, marker('artifacts', 'b'));
	await absent(page, 'DOCUMENT_BODY_a');
	assert.equal(new URL(page.url()).searchParams.has('doc'), false);
	assert.equal(new URL(page.url()).searchParams.get('keep'), '1');
	assert.equal(calls.some((c) => c.path === '/api/documents/doc-a' && c.ws === 'b'), false);
});
