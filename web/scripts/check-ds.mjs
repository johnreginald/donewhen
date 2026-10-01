// Design-system drift check. No dependencies.
// Scans src/app.css and the <style> blocks of every .svelte file under src.
// Fails (exit 1) on: raw colours outside app.css token declarations,
// raw px font-size, raw px border-radius (except 50% / 999px / 9999px).
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
const radiusDecl = /border(?:-[a-z]+){0,2}-radius\s*:\s*([^;}]*)/;

const findings = [];
for (const file of walk(src)) {
	const isCss = file.endsWith('.css');
	const rel = relative(root, file);
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
	console.error(
		`\ncheck:ds failed: ${findings.length} finding(s) (${n('raw font-size')} font-size, ${n('raw border-radius')} border-radius, ${n('raw colour')} colour). Use a token, or add /* ds-ok: <reason> */ on the line.`
	);
	process.exit(1);
}
console.log('check:ds ok');
