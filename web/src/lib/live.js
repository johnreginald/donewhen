// Pure helpers for the live stream, kept free of Svelte and the DOM so they can
// run under `npm test`.

// coalesce wraps an async fn so at most one call is in flight: a call made while
// one is running gets that run's promise instead of starting another.
export function coalesce(fn) {
	let running = null;
	return () => {
		if (!running) {
			running = Promise.resolve()
				.then(fn)
				.finally(() => {
					running = null;
				});
		}
		return running;
	};
}

// belongsInView says whether an issue pushed by the server belongs in the list
// the user is looking at: the right workspace, and inside the epic / project /
// label filter that list was fetched with.
export function belongsInView(issue, { workspaceId, project, initiative, label, projects }) {
	if (!issue) return false;
	if (workspaceId && issue.workspaceId && issue.workspaceId !== workspaceId) return false;
	if (initiative) {
		const p = (projects || []).find((x) => x.id === issue.projectId);
		if (!p || p.initiativeId !== initiative) return false;
	} else if (project && issue.projectId !== project) return false;
	if (label && !(issue.labels || []).some((l) => l.id === label)) return false;
	return true;
}
