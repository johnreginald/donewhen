// Issue filters and their URL form, kept free of Svelte and the DOM so they can
// run under `npm test`. The same filters travel in the page URL
// (`?state=Ready,In+Review&priority=1,2&label=<id>&project=<id>&q=text`), in
// saved views and in GET /api/issues.

export const emptyFilters = () => ({ states: [], priorities: [], labels: [] });

// Query parameters that count as a filter, besides the layout's own.
const KEYS = ['state', 'priority', 'label', 'project', 'q'];

function list(v) {
	return (v || '')
		.split(',')
		.map((x) => x.trim())
		.filter(Boolean);
}

// hasFilterParams says whether a URL query string names any filter at all.
export function hasFilterParams(search) {
	const p = new URLSearchParams(search || '');
	return KEYS.some((k) => p.has(k));
}

// parseFilters reads a URL query string. Unknown values are kept: they just
// match nothing, and a view saved in another workspace degrades the same way.
export function parseFilters(search) {
	const p = new URLSearchParams(search || '');
	const priorities = list(p.get('priority'))
		.map(Number)
		.filter((n) => Number.isInteger(n) && n >= 0 && n <= 4);
	return {
		states: list(p.get('state')),
		priorities: [...new Set(priorities)],
		labels: list(p.get('label')),
		project: p.get('project') || '',
		q: p.get('q') || ''
	};
}

// One value as it appears in the query: spaces as "+", but commas stay the
// list separator, so a value never contains one.
const enc = (v) => encodeURIComponent(String(v).replace(/,/g, ' ')).replace(/%20/g, '+');

// serializeFilters is the inverse of parseFilters: a query string without the
// leading "?", empty when nothing is filtered. Order is fixed so equal filters
// give equal strings.
export function serializeFilters({ states = [], priorities = [], labels = [], project = '', q = '' } = {}) {
	const parts = [];
	if (states.length) parts.push('state=' + states.map(enc).join(','));
	if (priorities.length) parts.push('priority=' + [...priorities].sort((a, b) => a - b).join(','));
	if (labels.length) parts.push('label=' + labels.map(enc).join(','));
	if (project) parts.push('project=' + enc(project));
	if (q.trim()) parts.push('q=' + enc(q.trim()));
	return parts.join('&');
}

// matchesFilters applies the state, priority and label filters to one issue:
// OR within a filter, AND across them. states are the workspace's workflow
// states, so a filter can name a state or carry its id.
export function matchesFilters(issue, { states = [], priorities = [], labels = [] }, workflow = []) {
	if (states.length) {
		const st = workflow.find((s) => s.id === issue.stateId);
		const want = states.map((s) => s.toLowerCase());
		if (!st || !(want.includes(st.name.toLowerCase()) || want.includes(st.id.toLowerCase()))) return false;
	}
	if (priorities.length && !priorities.includes(issue.priority ?? 0)) return false;
	if (labels.length && !(issue.labels || []).some((l) => labels.includes(l.id))) return false;
	return true;
}

// textMatches is the search box: title or key contains the text, literally.
export function textMatches(issue, text) {
	const needle = (text || '').trim().toLowerCase();
	if (!needle) return true;
	return issue.title.toLowerCase().includes(needle) || issue.key.toLowerCase().includes(needle);
}

// toggle adds or removes one value from a list.
export function toggle(list, v) {
	return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
}

export function sameList(a, b) {
	return a.length === b.length && [...a].sort().join('\n') === [...b].sort().join('\n');
}
