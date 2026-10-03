// Which pages reload when the active workspace changes (DW-100).
//
// A page that loads its own data (Inbox, Blocked, Artifacts, Log, Settings, Labels, ...) must be
// mounted again for the new workspace, or it keeps showing the old one. The pages below read only
// the stores that switchWorkspace clears and reloads, so they stay mounted. The rule is inverted on
// purpose: a new page reloads by default and is safe without anybody remembering to list it.

const STORE_ONLY = ['/board', '/list', '/by-epic', '/links', '/tasks', '/issue'];
// These pages need the workflow states, so they wait until the switch has loaded the metadata.
const NEEDS_METADATA = ['/inbox', '/blocked'];

// "/blocked/" is "/blocked". The app ignores a trailing slash (trailingSlash: 'ignore').
function normalize(pathname) {
	const p = String(pathname || '').replace(/\/+$/, '');
	return p || '/';
}

const under = (p, base) => p === base || p.startsWith(base + '/');

// ownsData: the page loads data itself, so it is mounted again for the new workspace.
export function ownsData(pathname) {
	const p = normalize(pathname);
	return !STORE_ONLY.some((s) => under(p, s));
}

// needsMetadata: the page cannot work before the new workspace's states are loaded. It shows a
// short placeholder during the switch. Every other page mounts at once.
export function needsMetadata(pathname) {
	const p = normalize(pathname);
	return NEEDS_METADATA.some((s) => under(p, s));
}

// URL parameters that name something of the workspace that was left: an Artifacts document and an
// issue peek. They would open the wrong thing, or nothing, in the new workspace.
const WORKSPACE_PARAMS = ['doc', 'peek'];

// stripWorkspaceParams returns the path, query and hash without those parameters, or null when the
// URL has none of them. The argument is a URL object (SvelteKit's $page.url).
export function stripWorkspaceParams(url) {
	const params = new URLSearchParams(url.search);
	let found = false;
	for (const k of WORKSPACE_PARAMS) {
		if (params.has(k)) {
			params.delete(k);
			found = true;
		}
	}
	if (!found) return null;
	const q = params.toString();
	return url.pathname + (q ? '?' + q : '') + (url.hash || '');
}
