// Pure rules for the quick-capture dialog, kept free of Svelte and the DOM so
// they can run under `npm test`.

export const TITLE_MAX = 200;

// validateTitle returns the trimmed title, or the inline error to show.
export function validateTitle(raw) {
	const title = (raw ?? '').trim();
	if (!title) return { error: 'Give the issue a title.' };
	if ([...title].length > TITLE_MAX) return { error: `Keep the title under ${TITLE_MAX} characters.` };
	return { title };
}

// toggleLabel adds or removes one label id from the selection. Labels in an
// exclusive group replace the group's other pick instead of stacking.
export function toggleLabel(selected, id, labels, groups) {
	if (selected.includes(id)) return selected.filter((x) => x !== id);
	const label = labels.find((l) => l.id === id);
	if (!label) return selected;
	const group = groups.find((g) => g.id === label.groupId);
	if (!group?.exclusive) return [...selected, id];
	const rivals = new Set(labels.filter((l) => l.groupId === group.id).map((l) => l.id));
	return [...selected.filter((x) => !rivals.has(x)), id];
}

// hasDraft says whether closing the dialog would throw away typed text.
export function hasDraft(title, description) {
	return !!((title ?? '').trim() || (description ?? '').trim());
}
