// Splits a line into `code`, **bold** and plain parts. No HTML is parsed: the caller
// renders each part as text. A backticked span wins, so `**x**` stays code.
const RE = /(`[^`]+`|\*\*[^*\n]+\*\*)/g;

export function parseInline(text) {
	return String(text ?? '')
		.split(RE)
		.filter(Boolean)
		.map((p) => {
			if (p.length > 2 && p.startsWith('`') && p.endsWith('`')) return { kind: 'code', text: p.slice(1, -1) };
			if (p.length > 4 && p.startsWith('**') && p.endsWith('**')) return { kind: 'bold', text: p.slice(2, -2) };
			return { kind: 'text', text: p };
		});
}
