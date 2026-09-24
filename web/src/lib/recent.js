// Recently opened tasks, per workspace, for the sidebar — Paperclip's
// "Recent tasks". Kept in this browser only: it is a convenience, not a record.

import { writable } from 'svelte/store';

const KEY = 'raenil.recent';
const MAX = 6;

function load() {
	try {
		return JSON.parse(localStorage.getItem(KEY) || '{}') || {};
	} catch {
		return {};
	}
}

// recent maps a workspace slug to its recently opened tasks, newest first.
export const recent = writable(load());

export function touchRecent(workspace, issue) {
	if (!workspace || !issue?.key) return;
	recent.update((all) => {
		const list = [{ key: issue.key, title: issue.title }, ...(all[workspace] || []).filter((x) => x.key !== issue.key)];
		const next = { ...all, [workspace]: list.slice(0, MAX) };
		try {
			localStorage.setItem(KEY, JSON.stringify(next));
		} catch {
			/* private mode: keep it for this session only */
		}
		return next;
	});
}
