// Thin REST client. Sends cookies; adds the CSRF header on mutations.

function getCookie(name) {
	const m = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
	return m ? decodeURIComponent(m[1]) : '';
}

async function request(method, path, body) {
	const headers = {};
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (method !== 'GET') headers['X-CSRF-Token'] = getCookie('raenil_csrf');
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

	// auth
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
	projects: (initiative) =>
		request('GET', '/projects' + (initiative ? `?initiative=${initiative}` : '')),
	saveProject: (b) => request('POST', '/projects', b),

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
	createToken: (name) => request('POST', '/tokens', { name }),
	deleteToken: (id) => request('DELETE', `/tokens/${id}`),

	// push
	pushSubscribe: (sub) => request('POST', '/push/subscribe', sub),
	pushUnsubscribe: (endpoint) => request('POST', '/push/unsubscribe', { endpoint })
};
