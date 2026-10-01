// Svelte action: hide a pinned toolbar while the user scrolls down, show it
// again on scroll up or near the top. The scroller is any sibling inside the
// same parent, so the action needs no reference to it.
export function autohide(node) {
	const host = node.parentElement;
	const last = new WeakMap();
	let hidden = false;
	let lock = 0;
	node.setAttribute('data-autohide', '');

	function set(h) {
		if (h === hidden || performance.now() < lock) return;
		hidden = h;
		if (h) node.style.setProperty('--ah', node.offsetHeight + 'px');
		node.classList.toggle('ah-hidden', h);
		lock = performance.now() + 250;
	}
	function onScroll(e) {
		const t = e.target;
		if (!(t instanceof HTMLElement) || node.contains(t)) return;
		const top = t.scrollTop;
		const prev = last.get(t) ?? top;
		last.set(t, top);
		const dy = top - prev;
		if (top < 24) set(false);
		else if (dy > 8) set(true);
		else if (dy < -8) set(false);
	}
	host.addEventListener('scroll', onScroll, true);
	return {
		destroy() {
			host.removeEventListener('scroll', onScroll, true);
		}
	};
}
