// Live event stream, bound to one workspace.
//
// EventSource cannot send headers, so the workspace rides in the query string.
// Switching workspace closes this stream and opens a new one — see the layout.
//
// Status: 'connecting' → 'live' ⇄ 'reconnecting' → 'offline'. EventSource
// retries on its own while it is CONNECTING; once the browser gives up
// (readyState CLOSED) we reopen with a backoff. Anything that may have been
// missed while the stream was down (reconnect, `resync`, a tab that was hidden
// a while) calls onCatchUp so the caller can refetch.

export const BACKOFF_MS = [1000, 2000, 5000, 10000, 30000];
export const OFFLINE_AFTER_MS = 30000;
export const HIDDEN_CATCHUP_MS = 30000;

const TYPES = [
	'issue.created',
	'issue.updated',
	'issue.state_changed',
	'issue.deleted',
	'comment.added',
	'issue.blockers',
	'document.saved',
	'document.deleted'
];

export function connectSSE(workspace, { onEvent, onStatus, onCatchUp }) {
	let es = null;
	let closed = false;
	let opened = false; // has this stream been live before?
	let attempt = 0;
	let retryTimer = null;
	let offlineTimer = null;
	let hiddenAt = 0;
	let status = '';

	function setStatus(s) {
		if (s === status) return;
		status = s;
		onStatus?.(s);
	}

	function open() {
		if (closed) return;
		clearTimeout(retryTimer);
		es = new EventSource('/api/events?workspace=' + encodeURIComponent(workspace), {
			withCredentials: true
		});
		for (const t of TYPES) {
			es.addEventListener(t, (e) => {
				try {
					onEvent(JSON.parse(e.data));
				} catch {
					/* ignore */
				}
			});
		}
		es.addEventListener('resync', () => onCatchUp?.());
		es.onopen = () => {
			clearTimeout(offlineTimer);
			offlineTimer = null;
			attempt = 0;
			setStatus('live');
			if (opened) onCatchUp?.();
			opened = true;
		};
		es.onerror = () => {
			if (closed) return;
			if (status !== 'offline') setStatus('reconnecting');
			offlineTimer ||= setTimeout(() => setStatus('offline'), OFFLINE_AFTER_MS);
			// The browser keeps retrying by itself while CONNECTING. Once it has
			// given up the stream is dead, so reopen it ourselves with a backoff.
			if (es.readyState === 2) {
				es.close();
				const delay = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
				attempt++;
				retryTimer = setTimeout(open, delay);
			}
		};
	}

	// Back on the network, or the tab came forward: do not wait out a backoff.
	function reviveNow() {
		if (closed || status === 'live') return;
		es?.close();
		attempt = 0;
		open();
	}
	function onVisibility() {
		if (document.visibilityState === 'hidden') {
			hiddenAt = Date.now();
			return;
		}
		if (hiddenAt && Date.now() - hiddenAt > HIDDEN_CATCHUP_MS) onCatchUp?.();
		hiddenAt = 0;
		if (status === 'offline') reviveNow();
	}
	function onOnline() {
		if (status === 'offline' || status === 'reconnecting') reviveNow();
	}

	if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility);
	if (typeof window !== 'undefined') window.addEventListener('online', onOnline);

	setStatus('connecting');
	open();
	return () => {
		closed = true;
		clearTimeout(retryTimer);
		clearTimeout(offlineTimer);
		if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
		if (typeof window !== 'undefined') window.removeEventListener('online', onOnline);
		es && es.close();
	};
}
