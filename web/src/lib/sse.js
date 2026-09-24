// Live event stream. Reconnects automatically (EventSource does this natively).
//
// The stream is bound to one workspace: EventSource cannot send headers, so the
// workspace rides in the query string. Switching workspace closes and reopens
// it — see the layout.

import { getWorkspace } from './api.js';

export function connectSSE(onEvent) {
	let es;
	let closed = false;

	function open() {
		if (closed) return;
		es = new EventSource('/api/events', { withCredentials: true });
		const types = [
			'issue.created',
			'issue.updated',
			'issue.state_changed',
			'issue.deleted',
			'comment.added',
			'run.started',
			'run.finished',
			'job.updated',
			'host.updated',
			'agent.saved',
			'interaction.created',
			'interaction.updated'
		];
		for (const t of types) {
			es.addEventListener(t, (e) => {
				try {
					onEvent(JSON.parse(e.data));
				} catch {
					/* ignore */
				}
			});
		}
		es.onerror = () => {
			// EventSource retries on its own; nothing to do.
		};
	}

	open();
	return () => {
		closed = true;
		es && es.close();
	};
}
