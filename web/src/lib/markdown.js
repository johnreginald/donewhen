// Render markdown to HTML and mermaid code fences to SVG.
import { marked } from 'marked';

let mermaidPromise;
let mermaidCounter = 0;

async function getMermaid() {
	if (!mermaidPromise) {
		mermaidPromise = import('mermaid').then((m) => {
			m.default.initialize({
				startOnLoad: false,
				theme: 'dark',
				securityLevel: 'strict',
				fontFamily: 'inherit'
			});
			return m.default;
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
	return marked.parse(src, { renderer });
}

function escapeHtml(s) {
	return s
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;');
}

// renderMermaid finds .mermaid-block nodes inside root and renders SVG, with a
// toggle to show the source.
export async function renderMermaid(root) {
	if (!root) return;
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
			toggle.textContent = 'code';
			const codeEl = document.createElement('pre');
			codeEl.className = 'mermaid-source';
			codeEl.style.display = 'none';
			codeEl.textContent = code;
			toggle.onclick = () => {
				const showing = codeEl.style.display !== 'none';
				codeEl.style.display = showing ? 'none' : 'block';
				wrap.style.display = showing ? 'block' : 'none';
				toggle.textContent = showing ? 'code' : 'diagram';
			};
			const container = document.createElement('div');
			container.className = 'mermaid-container';
			container.append(toggle, wrap, codeEl);
			block.replaceWith(container);
		} catch (e) {
			block.innerHTML = `<code>mermaid error: ${escapeHtml(String(e))}</code>`;
		}
	}
}
