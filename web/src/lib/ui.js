import { writable, get } from 'svelte/store';
import { goto } from '$app/navigation';
import { states } from './store.js';
import { needsReason } from './blocked.js';

export const paletteOpen = writable(false);
export const navOpen = writable(false); // the sidebar, on a phone
export const composer = writable(null); // { kind: 'issue'|'project'|'initiative', prefill }
export const archiveTarget = writable(null); // an epic awaiting archive confirmation
export const quickCapture = writable(false); // the "C" one-line issue composer
export const shortcutHelp = writable(false); // the "?" keyboard-shortcut overlay
export const blockPrompt = writable(null); // { label, resolve }: the "why is it blocked?" dialog
export const issueMenu = writable(null); // { kind: 'status'|'priority'|'label'|'epic', key }: the keyboard menus
export const connectionLost = writable(false); // true while the SSE stream is erroring
// '' | 'connecting' | 'live' | 'reconnecting' | 'offline' — the sidebar dot
export const streamStatus = writable('');
export const toasts = writable([]); // stack of { id, message, kind, actionLabel, onAction }
// liveEvent is the latest server event, for views that follow one thing (a
// ticket's runs) rather than the issue list the layout already keeps current.
export const liveEvent = writable(null);

// onLive calls fn for each new server event. Use it instead of an $effect on
// $liveEvent: a handler that updates state it also reads would re-run itself
// inside an effect. It skips the event already current when it subscribes.
// Returns the unsubscribe, so onMount(() => onLive(...)) cleans up.
export function onLive(fn) {
	let first = true;
	return liveEvent.subscribe((ev) => {
		if (first) {
			first = false;
			return;
		}
		if (ev) fn(ev);
	});
}

// openComposer opens the create modal for the given entity, with optional
// prefilled fields (e.g. a column's stateId when creating from that lane).
export function openComposer(kind, prefill = {}) {
	composer.set({ kind, prefill });
}
// askArchive opens the archive confirmation for an epic.
export function askArchive(project) {
	archiveTarget.set(project);
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

// showToast pushes a toast onto the stack; each dismisses itself on its own
// timer, so several can be visible at once (e.g. a live-event toast landing
// while an error toast is still up). opts.actionLabel + opts.onAction add a
// button (e.g. "Retry") to the toast, per the design's action link.
let toastSeq = 0;
export function showToast(message, kind = 'info', opts = {}) {
	const id = ++toastSeq;
	toasts.update((list) => [...list, { id, message, kind, actionLabel: opts.actionLabel, onAction: opts.onAction }]);
	setTimeout(() => dismissToast(id), 4000);
	return id;
}
export function dismissToast(id) {
	toasts.update((list) => list.filter((t) => t.id !== id));
}

// askBlockedReason opens the reason dialog and resolves with the trimmed
// reason, or null when the user backs out. label names what is being blocked.
export function askBlockedReason(label) {
	return new Promise((resolve) => {
		// A second prompt replaces the first, which counts as cancelled.
		get(blockPrompt)?.resolve(null);
		blockPrompt.set({ label, resolve });
	});
}

// blockedReasonFor is what every state change calls first. '' means no reason
// is needed, a string is the reason to send, and null means the user cancelled.
export async function blockedReasonFor(fromStateId, toStateId, label) {
	if (!needsReason(get(states), fromStateId, toStateId)) return '';
	return askBlockedReason(label);
}

// openIssue navigates to the full detail page (replaces the old right-side drawer).
export function openIssue(idOrKey) {
	goto('/issue/' + encodeURIComponent(idOrKey));
}
export function closeIssue() {
	goto('/');
}
