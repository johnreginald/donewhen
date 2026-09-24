// Thin REST client. Sends cookies; adds the CSRF header on mutations, and names
// the active workspace on every request — the server scopes everything to it.

function getCookie(name) {
	const m = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
	return m ? decodeURIComponent(m[1]) : '';
}

const WS_KEY = 'raenil_workspace';

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

async function request(method, path, body) {
	const headers = {};
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (method !== 'GET') headers['X-CSRF-Token'] = getCookie('raenil_csrf');
	const wsp = getWorkspace();
	if (wsp) headers['X-Workspace'] = wsp;
	const res = await fetch('/api' + path, {
		method,
		headers,
		credentials: 'include',
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
	if (res.status === 401) {
		const err = new Error('unauthorized');
		err.status = 401;
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
		throw err;
	}
	return data;
}

export const api = {
	get: (p) => request('GET', p),
	post: (p, b) => request('POST', p, b ?? {}),
	patch: (p, b) => request('PATCH', p, b ?? {}),
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
	projects: (initiative) =>
		request('GET', '/projects' + (initiative ? `?initiative=${initiative}` : '')),
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
	dashboard: (tz) => request('GET', '/dashboard' + (tz ? `?tz=${encodeURIComponent(tz)}` : '')),
	// agents, runner hosts, queued jobs
	agents: () => request('GET', '/agents'),
	agent: (id) => request('GET', `/agents/${id}`),
	createAgent: (b) => request('POST', '/agents', b),
	updateAgent: (id, b) => request('PATCH', `/agents/${id}`, b),
	deleteAgent: (id) => request('DELETE', `/agents/${id}`),
	agentRuns: (id) => request('GET', `/agents/${id}/runs`),
	testAgent: (id) => request('POST', `/agents/${id}/test`, {}),
	hosts: () => request('GET', '/hosts'),
	jobs: (q = {}) => {
		const p = new URLSearchParams(Object.entries(q).filter(([, v]) => v));
		const s = p.toString();
		return request('GET', '/jobs' + (s ? `?${s}` : ''));
	},
	job: (id) => request('GET', `/jobs/${id}`),
	enqueueJob: (b) => request('POST', '/jobs', b),
	// runs — each agent attempt
	runs: (q = {}) => {
		const p = new URLSearchParams(Object.entries(q).filter(([, v]) => v));
		const s = p.toString();
		return request('GET', '/runs' + (s ? `?${s}` : ''));
	},
	run: (id) => request('GET', `/runs/${id}`),
	issueRuns: (id) => request('GET', `/issues/${id}/runs`),
	// coverage
	missingDocs: () => request('GET', '/issues/missing-docs'),
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
