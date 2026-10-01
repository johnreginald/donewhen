// Thin REST client. Sends cookies; adds the CSRF header on mutations, and names
// the active workspace on every request — the server scopes everything to it.

function getCookie(name) {
	const m = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
	return m ? decodeURIComponent(m[1]) : '';
}

const WS_KEY = 'donewhen_workspace';

// The active workspace lives in localStorage so a reload lands in the same
// place, and so this module can read it without importing the store (which
// imports this file).
export function getWorkspace() {
	try {
		return localStorage.getItem(WS_KEY) || '';
	} catch {
		return '';
	}
}

export function setWorkspace(slug) {
	try {
		if (slug) localStorage.setItem(WS_KEY, slug);
		else localStorage.removeItem(WS_KEY);
	} catch {
		/* private mode: fall back to the server's default */
	}
}

// The shell registers a notifier (a toast) once boot has finished; before that
// a network failure must reach boot() as an error, not as a toast.
let notifier = null;
export function setNotifier(fn) {
	notifier = fn;
}
export function notify(message) {
	notifier?.(message);
}

// Calls that are allowed to answer 401 without it meaning "session expired".
const PUBLIC_PATHS = ['/auth/login', '/auth/setup', '/auth/status'];
let leaving = false;

// expireSession sends the user to the login page, which brings them back to
// the page they were on.
export function expireSession() {
	if (leaving || typeof location === 'undefined' || location.pathname === '/login') return;
	leaving = true;
	location.assign('/login?next=' + encodeURIComponent(location.pathname + location.search));
}

async function request(method, path, body, wsOverride) {
	const headers = {};
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (method !== 'GET') headers['X-CSRF-Token'] = getCookie('donewhen_csrf');
	const wsp = wsOverride || getWorkspace();
	if (wsp) headers['X-Workspace'] = wsp;
	let res;
	try {
		res = await fetch('/api' + path, {
			method,
			headers,
			credentials: 'include',
			body: body !== undefined ? JSON.stringify(body) : undefined
		});
	} catch {
		// The server could not be reached at all (offline, restarting). Not a
		// logout: the caller keeps its data and the shell says so.
		const err = new Error("Can't reach DoneWhen");
		err.network = true;
		err.status = 0;
		notify(err.message);
		throw err;
	}
	if (res.status === 401) {
		const err = new Error('unauthorized');
		err.status = 401;
		if (!PUBLIC_PATHS.some((p) => path.startsWith(p))) expireSession();
		throw err;
	}
	if (res.status === 403) {
		// The stored workspace is no longer ours (revoked, renamed, deleted).
		// Drop it so the next request falls back to a workspace we do have,
		// and let the caller re-pick rather than logging out.
		const data = await res.json().catch(() => null);
		const err = new Error((data && data.error) || 'forbidden');
		err.status = 403;
		if (wsp) {
			setWorkspace('');
			err.workspaceRejected = true;
		}
		throw err;
	}
	if (res.status === 204) return null;
	const data = await res.json().catch(() => null);
	if (!res.ok) {
		const err = new Error((data && data.error) || res.statusText);
		err.status = res.status;
		// A refused done-when gate (PP-203) says which criteria are open.
		if (data && data.code) {
			err.code = data.code;
			err.state = data.state;
			err.open = data.open;
		}
		throw err;
	}
	return data;
}

export const api = {
	// wsOverride lets a caller read another of the user's workspaces without
	// switching the active one — used for the per-workspace counts in the
	// workspace switcher, which must show every membership at once.
	get: (p, wsOverride) => request('GET', p, undefined, wsOverride),
	post: (p, b) => request('POST', p, b ?? {}),
	patch: (p, b) => request('PATCH', p, b ?? {}),
	put: (p, b) => request('PUT', p, b ?? {}),
	del: (p) => request('DELETE', p),

	// workspaces — the tenancy boundary
	workspaces: () => request('GET', '/workspaces'),
	createWorkspace: (b) => request('POST', '/workspaces', b),
	updateWorkspace: (id, b) => request('PATCH', `/workspaces/${id}`, b),
	activateWorkspace: (id) => request('POST', `/workspaces/${id}/activate`, {}),
	members: (id) => request('GET', `/workspaces/${id}/members`),
	addMember: (id, b) => request('POST', `/workspaces/${id}/members`, b),
	removeMember: (id, userId) => request('DELETE', `/workspaces/${id}/members/${userId}`),

	// auth
	me: () => request('GET', '/me'),
	authStatus: () => request('GET', '/auth/status'),
	setup: (email, password) => request('POST', '/auth/setup', { email, password }),
	login: (email, password) => request('POST', '/auth/login', { email, password }),
	logout: () => request('POST', '/auth/logout', {}),
	config: () => request('GET', '/config'),

	// metadata
	states: () => request('GET', '/states'),
	labels: () => request('GET', '/labels'),
	labelGroups: () => request('GET', '/label-groups'),
	createLabel: (b) => request('POST', '/labels', b),

	// initiatives / projects
	initiatives: () => request('GET', '/initiatives'),
	saveInitiative: (b) => request('POST', '/initiatives', b),
	updateInitiative: (id, b) => request('PATCH', `/initiatives/${id}`, b),
	deleteInitiative: (id) => request('DELETE', `/initiatives/${id}`),
	// archived: '' (default: active only) | '1' (archived only) | 'all'
	projects: (initiative, archived) => {
		const p = new URLSearchParams();
		if (initiative) p.set('initiative', initiative);
		if (archived) p.set('archived', archived);
		const s = p.toString();
		return request('GET', '/projects' + (s ? `?${s}` : ''));
	},
	project: (id) => request('GET', `/projects/${id}`),
	archiveProject: (id) => request('POST', `/projects/${id}/archive`, {}),
	unarchiveProject: (id) => request('POST', `/projects/${id}/unarchive`, {}),
	saveProject: (b) => request('POST', '/projects', b),
	updateProject: (id, b) => request('PATCH', `/projects/${id}`, b),
	deleteProject: (id) => request('DELETE', `/projects/${id}`),

	// issues
	issues: (q = {}) => {
		const p = new URLSearchParams(Object.entries(q).filter(([, v]) => v));
		const s = p.toString();
		return request('GET', '/issues' + (s ? `?${s}` : ''));
	},
	issue: (id) => request('GET', `/issues/${id}`),
	createIssue: (b) => request('POST', '/issues', b),
	updateIssue: (id, b) => request('PATCH', `/issues/${id}`, b),
	deleteIssue: (id) => request('DELETE', `/issues/${id}`),
	// One request for a board drag: new column plus the neighbours at the drop point.
	moveIssue: (id, b) => request('POST', `/issues/${id}/move`, b),
	comments: (id) => request('GET', `/issues/${id}/comments`),
	addComment: (id, bodyMd) => request('POST', `/issues/${id}/comments`, { bodyMd }),
	issueActivity: (id) => request('GET', `/issues/${id}/activity`),
	activity: (q = {}) => {
		const p = new URLSearchParams(Object.entries(q).filter(([, v]) => v));
		const s = p.toString();
		return request('GET', '/activity' + (s ? `?${s}` : ''));
	},
	// dev links + commits
	commits: (id) => request('GET', `/issues/${id}/commits`),
	addCommit: (id, b) => request('POST', `/issues/${id}/commits`, b),
	setDev: (id, b) => request('PATCH', `/issues/${id}/dev`, b),
	// done-when criteria
	criteria: (id) => request('GET', `/issues/${id}/criteria`),
	addCriterion: (id, body) => request('POST', `/issues/${id}/criteria`, { body }),
	updateCriterion: (id, b) => request('PATCH', `/criteria/${id}`, b),
	deleteCriterion: (id) => request('DELETE', `/criteria/${id}`),
	// coverage
	missingDocs: () => request('GET', '/issues/missing-docs'),
	// every Blocked issue with its reason, oldest first
	blocked: () => request('GET', '/blocked'),
	// inbox — review queue + recent AI activity
	inbox: () => request('GET', '/inbox'),
	inboxSeen: () => request('POST', '/inbox/seen', {}),

	// documents
	documents: (filter = {}) => {
		const p = new URLSearchParams(Object.entries(filter).filter(([, v]) => v));
		const s = p.toString();
		return request('GET', '/documents' + (s ? `?${s}` : ''));
	},
	document: (id) => request('GET', `/documents/${id}`),
	saveDocument: (b) => request('POST', '/documents', b),
	deleteDocument: (id) => request('DELETE', `/documents/${id}`),

	// tokens
	tokens: () => request('GET', '/tokens'),
	createToken: (name, workspace) => request('POST', '/tokens', { name, workspace }),
	deleteToken: (id) => request('DELETE', `/tokens/${id}`),

	// push
	pushSubscribe: (sub) => request('POST', '/push/subscribe', sub),
	pushUnsubscribe: (endpoint) => request('POST', '/push/unsubscribe', { endpoint })
};
