// Render markdown to HTML and mermaid code fences to SVG.
import { marked } from 'marked';
import { safeLinkHref, LINK_REL } from './url.js';

let mermaidPromise;
let mermaidCounter = 0;

// The app is light-first; dark comes from an explicit data-theme on <html> or
// the OS preference (see app.css). Mermaid is themed once, at first use.
function prefersDark() {
	const t = document.documentElement.dataset.theme;
	if (t) return t === 'dark';
	return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
}

async function getMermaid() {
	if (!mermaidPromise) {
		mermaidPromise = import('mermaid')
			.then((m) => {
				m.default.initialize({
					startOnLoad: false,
					theme: prefersDark() ? 'dark' : 'neutral',
					securityLevel: 'strict',
					fontFamily: 'inherit'
				});
				return m.default;
			})
			.catch((e) => {
				// Don't cache a failed import (e.g. a chunk fetch aborted by
				// navigation) — let the next render retry.
				mermaidPromise = null;
				throw e;
			});
	}
	return mermaidPromise;
}

marked.setOptions({ gfm: true, breaks: true });

// renderMarkdown returns HTML with mermaid blocks left as <pre data-mermaid>
// placeholders that renderMermaid() then upgrades to SVG.
export function renderMarkdown(src) {
	if (!src) return '';
	const renderer = new marked.Renderer();
	const origCode = renderer.code.bind(renderer);
	renderer.code = (code, lang) => {
		// marked v14 may pass an object; normalize.
		const text = typeof code === 'object' ? code.text : code;
		const language = typeof code === 'object' ? code.lang : lang;
		if (language === 'mermaid') {
			const encoded = encodeURIComponent(text);
			return `<pre class="mermaid-block" data-mermaid="${encoded}"><code>${escapeHtml(
				text
			)}</code></pre>`;
		}
		return origCode(code, lang);
	};
	// Tickets and review guides are written by agents that read untrusted
	// code: raw HTML is shown as text, never run, and only safe link schemes
	// are followed.
	renderer.html = (t) => escapeHtml(typeof t === 'object' ? t.text || t.raw || '' : t || '');
	const origLink = renderer.link.bind(renderer);
	renderer.link = (...args) => {
		const tok = args[0];
		const href = String(typeof tok === 'object' ? tok.href : tok || '');
		const safe = safeLinkHref(href);
		if (safe !== href.trim()) {
			if (typeof tok === 'object') tok.href = safe;
			else args[0] = safe;
		}
		return String(origLink(...args)).replace(/^<a /, `<a rel="${LINK_REL}" `);
	};
	return marked.parse(src, { renderer });
}

function escapeHtml(s) {
	return s
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;');
}

// Mermaid is not safe to run twice at once: two renders overlapping — a
// page re-rendering on a live update while a diagram is still drawing —
// corrupt each other and draw "Syntax error in text" for a valid diagram.
// Every render takes its turn.
let queue = Promise.resolve();

// renderMermaid finds .mermaid-block nodes inside root and renders SVG, with a
// toggle to show the source.
export function renderMermaid(root) {
	const turn = queue.then(() => renderMermaidNow(root));
	queue = turn.catch(() => {});
	return turn;
}

async function renderMermaidNow(root) {
	if (!root || !root.isConnected) return;
	const blocks = root.querySelectorAll('.mermaid-block[data-mermaid]');
	if (!blocks.length) return;
	const mermaid = await getMermaid();
	for (const block of blocks) {
		const code = decodeURIComponent(block.getAttribute('data-mermaid'));
		block.removeAttribute('data-mermaid');
		const id = 'mmd-' + mermaidCounter++;
		try {
			const { svg } = await mermaid.render(id, code);
			const wrap = document.createElement('div');
			wrap.className = 'mermaid-rendered';
			wrap.innerHTML = svg;
			const toggle = document.createElement('button');
			toggle.className = 'mermaid-toggle';
			toggle.type = 'button';
			toggle.textContent = 'code';
			const expand = document.createElement('button');
			expand.className = 'mermaid-expand';
			expand.type = 'button';
			expand.textContent = 'Expand';
			expand.setAttribute('aria-label', 'Open diagram full-screen');
			const codeEl = document.createElement('pre');
			codeEl.className = 'mermaid-source';
			codeEl.style.display = 'none';
			codeEl.textContent = code;
			toggle.onclick = () => {
				const showing = codeEl.style.display !== 'none';
				codeEl.style.display = showing ? 'none' : 'block';
				wrap.style.display = showing ? 'block' : 'none';
				toggle.textContent = showing ? 'code' : 'diagram';
				expand.style.display = showing ? '' : 'none';
			};
			const container = document.createElement('div');
			container.className = 'mermaid-container';
			// The source rides on the block so the full-screen viewer can copy it.
			container.dataset.source = code;
			const actions = document.createElement('div');
			actions.className = 'mermaid-actions';
			actions.append(expand, toggle);
			container.append(actions, wrap, codeEl);
			block.replaceWith(container);
		} catch (e) {
			// Never a blank space: a visible error box with the source open.
			const box = document.createElement('div');
			box.className = 'mermaid-error';
			box.setAttribute('role', 'alert');
			const msg = document.createElement('div');
			msg.className = 'mermaid-error-msg';
			msg.textContent = 'Diagram error: ' + errorSummary(e);
			msg.title = errorDetail(e);
			const det = document.createElement('details');
			det.className = 'mermaid-error-details';
			const sum = document.createElement('summary');
			sum.textContent = 'details';
			const detPre = document.createElement('pre');
			detPre.textContent = errorDetail(e);
			det.append(sum, detPre);
			const codeEl = document.createElement('pre');
			codeEl.className = 'mermaid-source';
			codeEl.textContent = code;
			box.append(msg, det, codeEl);
			block.replaceWith(box);
		} finally {
			// A failed render leaves its error drawing at the end of the page.
			document.getElementById('d' + id)?.remove();
		}
	}
}

// errorDetail is the full parser message.
export function errorDetail(e) {
	return String((e && e.message) || e || 'Unknown error').trim();
}

// errorSummary condenses a Mermaid parse error to one line: the line number
// plus the cause, e.g. "Line 2: got 'GRAPH' (expecting 'SEMI', …)".
export function errorSummary(e) {
	const lines = errorDetail(e).split('\n').map((l) => l.trim()).filter(Boolean);
	if (!lines.length) return 'Unknown error';
	const first = lines[0];
	const last = lines[lines.length - 1];
	let out = first;
	const m = first.match(/line (\d+)/i);
	const g = last.match(/^(Expecting .*?),? got (.+)$/i);
	if (m && g) out = `Line ${m[1]}: got ${g[2]} (${g[1].toLowerCase().replace(/^expecting/, 'expecting')})`;
	else if (lines.length > 1) out = first.replace(/:$/, '') + ': ' + last;
	return out.length > 140 ? out.slice(0, 139) + '…' : out;
}

// checkMermaid parses one diagram with the app's Mermaid. Browser only —
// Mermaid needs a DOM.
export function checkMermaid(source) {
	const turn = queue.then(async () => {
		try {
			const mermaid = await getMermaid();
			await mermaid.parse(source);
			return { ok: true, error: '' };
		} catch (e) {
			return { ok: false, error: errorSummary(e), detail: errorDetail(e) };
		}
	});
	queue = turn.catch(() => {});
	return turn;
}

// mermaidBlocks lists the source of every ```mermaid fence in a document.
export function mermaidBlocks(src) {
	if (!src) return [];
	return marked
		.lexer(src)
		.filter((t) => t.type === 'code' && t.lang === 'mermaid')
		.map((t) => t.text);
}
