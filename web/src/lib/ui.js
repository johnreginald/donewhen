import { writable } from 'svelte/store';

export const panelIssueId = writable(null); // id/key of issue open in the drawer
export const paletteOpen = writable(false);
export const toast = writable(null);
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
