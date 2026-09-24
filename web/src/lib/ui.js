import { writable } from 'svelte/store';
import { goto } from '$app/navigation';

export const paletteOpen = writable(false);
export const navOpen = writable(false); // the sidebar, on a phone
export const composer = writable(null); // { kind: 'issue'|'project'|'initiative', prefill }
export const toast = writable(null);
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

// openIssue navigates to the full detail page (replaces the old right-side drawer).
export function openIssue(idOrKey) {
	goto('/issue/' + encodeURIComponent(idOrKey));
}
export function closeIssue() {
	goto('/');
}
