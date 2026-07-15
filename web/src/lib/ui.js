import { writable } from 'svelte/store';

export const panelIssueId = writable(null); // id/key of issue open in the drawer
export const paletteOpen = writable(false);
export const composer = writable(null); // { kind: 'issue'|'project'|'initiative', prefill }
export const toast = writable(null);

// openComposer opens the create modal for the given entity, with optional
// prefilled fields (e.g. a column's stateId when creating from that lane).
export function openComposer(kind, prefill = {}) {
	composer.set({ kind, prefill });
}
export function closeComposer() {
	composer.set(null);
}
export const flashIssueId = writable(null); // id of an issue that just changed (live ring)

let flashTimer;
export function flashIssue(id) {
	flashIssueId.set(id);
	clearTimeout(flashTimer);
	flashTimer = setTimeout(() => flashIssueId.set(null), 3500);
}

let toastTimer;
export function showToast(message, kind = 'info') {
	toast.set({ message, kind });
	clearTimeout(toastTimer);
	toastTimer = setTimeout(() => toast.set(null), 3500);
}

export function openIssue(idOrKey) {
	panelIssueId.set(idOrKey);
}
export function closeIssue() {
	panelIssueId.set(null);
}
