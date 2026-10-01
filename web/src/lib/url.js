// Link safety. Stored links (PR, commit, repo) and rendered markdown links end
// up in href attributes, so a javascript: or data: value would run script. The
// server only stores http(s); these helpers are the second layer, for values
// that predate that check.

// Browsers ignore tabs, newlines and other C0 controls inside a URL scheme
// ("java\nscript:"), so strip them before looking at it.
// eslint-disable-next-line no-control-regex
const CONTROL = /[\u0000- \u007f]/g;

// safeHref returns the URL when it is an absolute http(s) URL, else null.
// A null result means "render the text, not a link".
export function safeHref(url) {
	if (typeof url !== 'string') return null;
	const v = url.trim();
	if (!v) return null;
	try {
		const u = new URL(v);
		return (u.protocol === 'http:' || u.protocol === 'https:') && u.hostname ? v : null;
	} catch {
		return null;
	}
}

// safeLinkHref is the markdown variant: it also lets through mailto:, anchors
// and relative links, because a ticket may legitimately point at /issue/PP-1.
// Any other scheme returns '#'.
export function safeLinkHref(href) {
	const v = String(href ?? '').trim();
	const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(v.replace(CONTROL, ''));
	if (!scheme) return v;
	const s = scheme[1].toLowerCase();
	return s === 'http' || s === 'https' || s === 'mailto' ? v : '#';
}

// safeNext reads the `?next=` the login page was sent with and returns a path
// on this site to go back to, or '/'. Only a plain absolute path passes, so a
// crafted link cannot bounce the user to another origin after login.
export function safeNext(search) {
	const n = new URLSearchParams(search || '').get('next') || '';
	// eslint-disable-next-line no-control-regex
	if (!n.startsWith('/') || n.startsWith('//') || n.startsWith('/\\') || /[\u0000-\u001f\u007f]/.test(n)) return '/';
	if (n === '/login' || n.startsWith('/login?')) return '/';
	return n;
}

// Every rendered link opts out of leaking the opener and the referrer.
export const LINK_REL = 'noopener noreferrer';
