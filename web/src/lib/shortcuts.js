// The keyboard shortcuts: one registry that the "?" overlay renders and the
// layout's key handler follows, plus the small pure helpers behind them. No
// Svelte or DOM imports, so it runs under `npm test`.

// Each entry: the keys as shown, what it does, and where it applies. `id`
// names the action the layout dispatches; entries without one are handled by
// the control they belong to (a form, a menu, a list).
export const SHORTCUTS = [
	{ group: 'Global', keys: ['⌘', 'K'], label: 'Search & command palette' },
	{ group: 'Global', keys: ['Ctrl', '1–9'], label: 'Switch workspace' },
	{ group: 'Global', id: 'capture', keys: ['C'], label: 'New issue (quick capture)' },
	{ group: 'Global', id: 'help', keys: ['?'], label: 'This help' },
	{ group: 'Go to', id: 'go-board', keys: ['G', 'B'], label: 'Board', to: '/board' },
	{ group: 'Go to', id: 'go-inbox', keys: ['G', 'I'], label: 'Inbox', to: '/inbox' },
	{ group: 'Go to', id: 'go-list', keys: ['G', 'L'], label: 'List', to: '/list' },
	{ group: 'Board, List and Inbox', id: 'next', keys: ['J'], label: 'Focus next issue' },
	{ group: 'Board, List and Inbox', id: 'prev', keys: ['K'], label: 'Focus previous issue' },
	{ group: 'Board, List and Inbox', keys: ['↵'], label: 'Open the focused issue' },
	{ group: 'On an issue (focused or open)', id: 'menu-status', keys: ['S'], label: 'Change status', menu: 'status' },
	{ group: 'On an issue (focused or open)', id: 'menu-priority', keys: ['P'], label: 'Change priority', menu: 'priority' },
	{ group: 'On an issue (focused or open)', id: 'menu-label', keys: ['L'], label: 'Change labels', menu: 'label' },
	{ group: 'On an issue (focused or open)', id: 'menu-epic', keys: ['E'], label: 'Change epic', menu: 'epic' },
	{ group: 'On an issue (focused or open)', id: 'back', keys: ['Esc'], label: 'Back to the list (issue page)' },
	{ group: 'In a menu', keys: ['↑', '↓'], label: 'Move selection' },
	{ group: 'In a menu', keys: ['↵'], label: 'Choose' },
	{ group: 'In a menu', keys: ['Esc'], label: 'Close' },
	{ group: 'In a form', keys: ['⌘', '↵'], label: 'Save' },
	{ group: 'In a form', keys: ['Esc'], label: 'Cancel' }
];

// groupShortcuts folds the registry into its groups, in order of first use.
export function groupShortcuts(list = SHORTCUTS) {
	const out = [];
	for (const s of list) {
		let g = out.find((x) => x.name === s.group);
		if (!g) out.push((g = { name: s.group, items: [] }));
		g.items.push(s);
	}
	return out;
}

// isTypingTarget: a shortcut must never fire while the user is writing.
// Takes anything with tagName and isContentEditable.
export function isTypingTarget(el) {
	if (!el) return false;
	const tag = el.tagName;
	return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || !!el.isContentEditable;
}

// stepIndex moves the focus through a list of n items. From nothing focused
// (-1) the first press lands on the first item, whichever way it points.
export function stepIndex(current, n, dir) {
	if (n <= 0) return -1;
	if (current < 0) return 0;
	return dir > 0 ? Math.min(n - 1, current + 1) : Math.max(0, current - 1);
}

// wrapIndex is the menu's arrow movement: it wraps at both ends.
export function wrapIndex(current, n, dir) {
	if (n <= 0) return -1;
	return (((current + dir) % n) + n) % n;
}

// createChord tracks a two-key sequence such as "g then b". arm() starts it,
// take() says whether the next key still falls inside the window (and ends it).
export function createChord(windowMs = 1200) {
	let armedAt = -Infinity;
	return {
		arm(now) {
			armedAt = now;
		},
		take(now) {
			const live = now - armedAt <= windowMs;
			armedAt = -Infinity;
			return live;
		},
		get armed() {
			return armedAt > -Infinity;
		}
	};
}

// actionFor looks a registry entry up by id.
export function actionFor(id) {
	return SHORTCUTS.find((s) => s.id === id);
}
