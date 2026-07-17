import { writable, get } from 'svelte/store';
import { api } from './api.js';

export const states = writable([]);
export const projects = writable([]);
export const initiatives = writable([]);
export const labels = writable([]);
export const issues = writable([]);
export const appConfig = writable({});
export const me = writable(null);
export const activeProject = writable(''); // '' = all (epic-level filter)
export const activeInitiative = writable(''); // '' = all (Project-level filter)
export const activeLabel = writable(''); // '' = all (label filter)
export const inboxCount = writable(0); // needs-review queue size (sidebar badge)

export async function loadMeta() {
	const [st, pr, ini, lb, cfg] = await Promise.all([
		api.states(),
		api.projects(),
		api.initiatives(),
		api.labels(),
		api.config()
	]);
	states.set(st || []);
	projects.set(pr || []);
	initiatives.set(ini || []);
	labels.set(lb || []);
	appConfig.set(cfg || {});
}

export async function loadIssues() {
	const initiative = get(activeInitiative);
	const project = get(activeProject);
	const label = get(activeLabel);
	const f = {};
	if (initiative) f.initiative = initiative;
	else if (project) f.project = project;
	if (label) f.label = label;
	const list = await api.issues(f);
	issues.set(list || []);
}

// applyEvent reconciles a live SSE event into the issues store.
export function applyEvent(ev) {
	if (!ev) return;
	if (ev.type === 'issue.deleted') {
		issues.update((l) => l.filter((i) => i.id !== ev.issueId));
		return;
	}
	if (ev.issue) {
		issues.update((l) => {
			const idx = l.findIndex((i) => i.id === ev.issue.id);
			if (idx >= 0) {
				const copy = [...l];
				copy[idx] = ev.issue;
				return copy;
			}
			return [...l, ev.issue];
		});
	}
}

export function stateById(id) {
	return get(states).find((s) => s.id === id);
}

export function projectById(id) {
	return get(projects).find((p) => p.id === id);
}

export const PRIORITIES = [
	{ value: 0, label: 'No priority' },
	{ value: 1, label: 'Urgent' },
	{ value: 2, label: 'High' },
	{ value: 3, label: 'Medium' },
	{ value: 4, label: 'Low' }
];
