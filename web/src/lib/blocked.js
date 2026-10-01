// Pure rules for moving an issue to Blocked, kept free of Svelte and the DOM so
// they can run under `npm test`. The server enforces the same rule (PP-207).

export const REASON_MAX = 500;

// needsReason says whether a move from one state to another must carry a
// reason: it enters Blocked from somewhere else.
export function needsReason(states, fromId, toId) {
	const blocked = states.find((s) => s.name.toLowerCase() === 'blocked');
	return !!blocked && toId === blocked.id && fromId !== blocked.id;
}

// checkReason returns the trimmed reason, or the inline error to show.
export function checkReason(raw) {
	const reason = (raw ?? '').trim();
	if (!reason) return { error: 'Say why it is blocked.' };
	if ([...reason].length > REASON_MAX) return { error: `Keep the reason under ${REASON_MAX} characters.` };
	return { reason };
}
