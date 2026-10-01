// The done-when gate (PP-203). The server refuses a move into In Review or Done
// with 409 and { code: 'criteria_incomplete' | 'criteria_missing', state, open }.

// gateFailure returns the refusal details when err is one, else null.
export function gateFailure(err) {
	if (!err || err.status !== 409) return null;
	if (err.code !== 'criteria_incomplete' && err.code !== 'criteria_missing') return null;
	return { code: err.code, state: err.state || '', open: err.open || [] };
}

// gateSummary is one short line for a toast.
export function gateSummary(g, key = '') {
	const what = key ? `${key} can't move to ${g.state}` : `Can't move to ${g.state}`;
	if (g.code === 'criteria_missing') return `${what}: it has no done-when criteria.`;
	const n = g.open.length;
	return `${what}: ${n} done-when item${n === 1 ? '' : 's'} not ticked.`;
}
