// Per-device theme choice: 'system' (follow the OS), 'light' or 'dark'.
// app.html applies the stored choice before first paint; this module changes
// it at runtime. Stored in localStorage so each device keeps its own.
import { writable } from 'svelte/store';

const KEY = 'raenil.theme';
export const THEMES = ['system', 'light', 'dark'];

export function getTheme() {
	try {
		const t = localStorage.getItem(KEY);
		return THEMES.includes(t) ? t : 'system';
	} catch {
		return 'system';
	}
}

export function applyTheme(t) {
	const root = document.documentElement;
	if (t === 'light' || t === 'dark') root.dataset.theme = t;
	else delete root.dataset.theme;
}

export const theme = writable(typeof localStorage === 'undefined' ? 'system' : getTheme());

export function setTheme(t) {
	if (!THEMES.includes(t)) t = 'system';
	try {
		if (t === 'system') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, t);
	} catch {
		/* private mode: still applies for this page */
	}
	applyTheme(t);
	theme.set(t);
}
