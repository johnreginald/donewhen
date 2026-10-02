// Design-system drift check. No dependencies.
// Scans src/app.css and the <style> blocks of every .svelte file under src.
// Fails (exit 1) on: raw colours outside app.css token declarations,
// raw px font-size, raw px border-radius (except 50% / 999px / 9999px),
// font-family that is not a token, font-weight outside 400/500/600.
// Also fails (rule checks, by selector) on: `.btn` in app.css without `height: 30px`;
// `.input`/`.select`/`.dd-btn` without `border-radius: var(--r-sm)`; `.dd-btn`
// without `height: 30px`; a `.settings-panel .input` rule (it restyled inputs as
// headers); a menu or popover (selector with `menu`/`.pop`/`.bmenu`) with
// `border-radius: var(--r)` or a `--line-strong` border (menus use `--r-lg` + `--line`).
// A line containing `/* ds-ok: <reason> */` is exempt.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const src = join(root, 'src');

function walk(dir, out = []) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) walk(p, out);
		else if (name.endsWith('.svelte') || p === join(src, 'app.css')) out.push(p);
	}
	return out;
}

// Returns [{line, text}] of the CSS lines to check for a file.
function cssLines(file) {
	const lines = readFileSync(file, 'utf8').split('\n');
	if (file.endsWith('.css')) return lines.map((text, i) => ({ line: i + 1, text }));
	const out = [];
	let inStyle = false;
	lines.forEach((text, i) => {
		if (/<style[\s>]/.test(text)) inStyle = true;
		if (inStyle) out.push({ line: i + 1, text });
		if (/<\/style>/.test(text)) inStyle = false;
	});
	return out;
}

const colour = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?)\(/g;
const fontDecl = /(?<![\w-])(font-size|font)\s*:\s*([^;}]*)/;
const familyDecl = /font-family\s*:\s*([^;}]*)/;
const weightDecl = /font-weight\s*:\s*([^;}]*)/;
const radiusDecl = /border(?:-[a-z]+){0,2}-radius\s*:\s*([^;}]*)/;

const findings = [];

// Rule checks: parse `selector { body }` blocks and test them by selector.
// Returns [{line, sel, body}]; line is where the selector starts.
function ruleBlocks(file) {
	const text = cssLines(file).map((l) => l.text).join('\n');
	const first = cssLines(file)[0]?.line ?? 1;
	const out = [];
	for (const m of text.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, ' ')).matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
		const sel = m[1].trim();
		const line = first + text.slice(0, m.index + m[0].indexOf(sel)).split('\n').length - 1;
		out.push({ line, sel, body: m[2] });
	}
	return out;
}
const hasDecl = (body, re) => re.test(body);
function checkRules(file, rel) {
	const lines = readFileSync(file, 'utf8').split('\n');
	for (const { line, sel, body } of ruleBlocks(file)) {
		if (/\/\*\s*ds-ok:/.test(lines[line - 1] ?? '')) continue;
		const sels = sel.split(',').map((x) => x.trim());
		const only = (name) => sels.includes(name);
		const bad = (msg) => findings.push(`${rel}:${line} ${sel.replace(/\s+/g, ' ')} (${msg})`);
		if (file.endsWith('.css') && only('.btn') && !hasDecl(body, /(?<![\w-])height\s*:\s*30px/)) bad('rule: .btn needs height: 30px');
		if (file.endsWith('.css') && only('.dd-btn') && !hasDecl(body, /(?<![\w-])height\s*:\s*30px/)) bad('rule: .dd-btn needs height: 30px');
		for (const n of ['.input', '.select', '.dd-btn']) {
			if (only(n) && !hasDecl(body, /border-radius\s*:\s*var\(--r-sm\)/)) bad(`rule: ${n} needs border-radius: var(--r-sm)`);
		}
		if (sels.some((x) => /^\.settings-panel\s+\.input\b/.test(x))) bad('rule: .settings-panel .input restyles inputs as headers');
		if (sels.some((x) => /menu|\.pop$|\.bmenu/i.test(x)) && /box-shadow\s*:\s*var\(--shadow-2\)/.test(body)) {
			if (/border-radius\s*:\s*var\(--r\)\s*;/.test(body)) bad('rule: menu needs border-radius: var(--r-lg)');
			if (/(?<![\w-])border\s*:[^;]*--line-strong/.test(body)) bad('rule: menu needs a --line border');
		}
	}
}
for (const file of walk(src)) {
	const isCss = file.endsWith('.css');
	const rel = relative(root, file);
	checkRules(file, rel);
	for (const { line, text } of cssLines(file)) {
		if (/\/\*\s*ds-ok:/.test(text)) continue;
		const code = text.replace(/\/\*.*?\*\//g, '');
		// custom-property declarations in app.css are the token blocks
		const isToken = isCss && /^\s*--[\w-]+\s*:/.test(code);
		if (isToken) continue;
		for (const m of code.matchAll(colour)) findings.push(`${rel}:${line} ${m[0]} (raw colour)`);
		const f = code.match(fontDecl);
		if (f) {
			// in the `font:` shorthand, "/<px>" is the line-height, not a size
			const val = f[1] === 'font' ? f[2].replace(/\/\s*[^\s/]+/, '') : f[2];
			for (const v of val.matchAll(/\d*\.?\d+px/g)) findings.push(`${rel}:${line} ${v[0]} (raw font-size)`);
		}
		const fam = code.match(familyDecl);
		if (fam && !/^\s*(var\(--(font|mono|serif)\)|inherit)\s*$/.test(fam[1])) findings.push(`${rel}:${line} ${fam[1].trim()} (raw font-family)`);
		const wt = code.match(weightDecl);
		if (wt && !/^\s*(400|500|600|inherit|normal)\s*$/.test(wt[1])) findings.push(`${rel}:${line} ${wt[1].trim()} (font-weight)`);
		const r = code.match(radiusDecl);
		if (r) {
			for (const v of r[1].matchAll(/(\d*\.?\d+)px/g)) {
				if (v[1] === '999' || v[1] === '9999') continue;
				findings.push(`${rel}:${line} ${v[0]} (raw border-radius)`);
			}
		}
	}
}

if (findings.length) {
	console.error(findings.join('\n'));
	const n = (k) => findings.filter((x) => x.includes(`(${k})`)).length;
	const rules = findings.filter((x) => x.includes('(rule:')).length;
	console.error(
		`\ncheck:ds failed: ${findings.length} finding(s) (${n('raw font-size')} font-size, ${n('raw border-radius')} border-radius, ${n('raw colour')} colour, ${n('raw font-family')} font-family, ${n('font-weight')} font-weight, ${rules} rule). Use a token, or add /* ds-ok: <reason> */ on the line.`
	);
	process.exit(1);
}
console.log('check:ds ok');
