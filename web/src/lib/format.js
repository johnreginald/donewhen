// Small display formatters shared across views.

// rel renders a timestamp as "5m ago", falling back to a date after a week.
export function rel(iso) {
	if (!iso) return '';
	const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
	if (s < 60) return 'just now';
	if (s < 3600) return Math.floor(s / 60) + 'm ago';
	if (s < 86400) return Math.floor(s / 3600) + 'h ago';
	if (s < 604800) return Math.floor(s / 86400) + 'd ago';
	return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

// tokens renders a count the way Paperclip does: 950, 27.4k, 1.2M.
export function tokens(n) {
	if (!n) return '0';
	if (n < 1000) return String(n);
	if (n < 1_000_000) return (n / 1000).toFixed(n < 10_000 ? 1 : 0).replace(/\.0$/, '') + 'k';
	return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
}

// duration renders the time between two timestamps: 42s, 3m 5s, 1h 2m.
export function duration(fromIso, toIso) {
	if (!fromIso) return '';
	const end = toIso ? new Date(toIso).getTime() : Date.now();
	const s = Math.max(0, Math.round((end - new Date(fromIso).getTime()) / 1000));
	if (s < 60) return s + 's';
	if (s < 3600) return Math.floor(s / 60) + 'm ' + (s % 60) + 's';
	return Math.floor(s / 3600) + 'h ' + Math.floor((s % 3600) / 60) + 'm';
}

// usd renders a dollar amount with cents, or more precision below a cent.
export function usd(n) {
	if (!n) return '$0.00';
	return n < 0.01 ? '$' + n.toFixed(4) : '$' + n.toFixed(2);
}

// One-line description of a history event, shared by the Activity timeline and the issue peek.
export function activityVerb(a) {
	switch (a.kind) {
		case 'created':
			return `created${a.to ? ' in ' + a.to : ''}`;
		case 'state_changed':
			return `${a.from} → ${a.to}`;
		case 'priority_changed':
			return `priority ${a.from} → ${a.to}`;
		case 'epic_changed':
			return `epic ${a.from} → ${a.to}`;
		case 'title_changed':
			return 'renamed the issue';
		case 'artifact_written':
			return `wrote artifact “${a.detail}”`;
		case 'epic_archived':
			return a.detail ? `archived epic “${a.detail}”` : 'archived an epic';
		case 'epic_unarchived':
			return a.detail ? `unarchived epic “${a.detail}”` : 'unarchived an epic';
		case 'gate_overridden':
			return `forced past the done-when gate to ${a.to}`;
		case 'deleted':
			return 'deleted';
		default: {
			const k = a.kind.replace(/_/g, ' ');
			return a.detail ? `${k}: ${a.detail}` : k;
		}
	}
}
