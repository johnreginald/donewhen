<script>
	import { onMount } from 'svelte';

	// Full-screen viewer for one rendered Mermaid diagram: wheel / pinch zoom,
	// drag pan, Fit, + / −, Copy source. Esc or a click on the backdrop closes.
	let { svg = '', source = '', onclose } = $props();

	let dialog = $state(null);
	let stage = $state(null);
	let content = $state(null);
	let scale = $state(1);
	let x = $state(0);
	let y = $state(0);
	let natW = 0;
	let natH = 0;
	let copied = $state(false);
	let ready = $state(false);

	const MIN = 0.1;
	const MAX = 12;
	const STEP = 1.25;

	// Move the overlay to <body> so no ancestor transform or overflow clips it.
	function portal(node) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}

	function measure() {
		const el = content?.querySelector('svg');
		if (!el) return;
		const vb = el.viewBox?.baseVal;
		if (vb && vb.width && vb.height) {
			natW = vb.width;
			natH = vb.height;
		} else {
			const r = el.getBoundingClientRect();
			natW = r.width || 800;
			natH = r.height || 600;
		}
		el.removeAttribute('style');
		el.setAttribute('width', String(natW));
		el.setAttribute('height', String(natH));
	}

	function fit() {
		if (!stage || !natW) return;
		const r = stage.getBoundingClientRect();
		const pad = 32;
		const s = Math.min((r.width - pad * 2) / natW, (r.height - pad * 2) / natH, 4);
		scale = Math.max(MIN, s);
		x = (r.width - natW * scale) / 2;
		y = (r.height - natH * scale) / 2;
	}

	// Zoom keeping the point (cx, cy) in stage coordinates fixed.
	function zoomAt(factor, cx, cy) {
		const next = Math.min(MAX, Math.max(MIN, scale * factor));
		const k = next / scale;
		x = cx - (cx - x) * k;
		y = cy - (cy - y) * k;
		scale = next;
	}

	function zoomCenter(factor) {
		const r = stage.getBoundingClientRect();
		zoomAt(factor, r.width / 2, r.height / 2);
	}

	function onwheel(e) {
		e.preventDefault();
		const r = stage.getBoundingClientRect();
		const f = Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0016));
		zoomAt(f, e.clientX - r.left, e.clientY - r.top);
	}

	// Pointer gestures: one pointer pans, two pinch.
	const pointers = new Map();
	let moved = false;
	let pinch = null;

	function ptrdown(e) {
		stage.setPointerCapture?.(e.pointerId);
		pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
		if (pointers.size === 1) moved = false;
		if (pointers.size === 2) {
			moved = true;
			pinch = pinchState();
		}
	}

	function pinchState() {
		const [a, b] = [...pointers.values()];
		return { d: Math.hypot(a.x - b.x, a.y - b.y), cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2 };
	}

	function ptrmove(e) {
		const p = pointers.get(e.pointerId);
		if (!p) return;
		if (pointers.size === 2 && pinch) {
			pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
			const n = pinchState();
			const r = stage.getBoundingClientRect();
			if (pinch.d > 0) zoomAt(n.d / pinch.d, n.cx - r.left, n.cy - r.top);
			x += n.cx - pinch.cx;
			y += n.cy - pinch.cy;
			pinch = n;
			return;
		}
		const dx = e.clientX - p.x;
		const dy = e.clientY - p.y;
		if (!moved && Math.hypot(dx, dy) < 3) return;
		moved = true;
		x += dx;
		y += dy;
		pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
	}

	function ptrup(e) {
		pointers.delete(e.pointerId);
		pinch = null;
	}

	// A click that is not the end of a drag, and not on the diagram, is a
	// click on the backdrop.
	function stageclick(e) {
		if (moved) {
			moved = false;
			return;
		}
		// Pointer capture retargets the click to the stage, so test by position.
		const r = content?.querySelector('svg')?.getBoundingClientRect();
		if (r && e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom) return;
		onclose?.();
	}

	async function copy() {
		try {
			await navigator.clipboard.writeText(source);
		} catch {
			const ta = document.createElement('textarea');
			ta.value = source;
			ta.style.position = 'fixed';
			ta.style.opacity = '0';
			dialog.appendChild(ta);
			ta.select();
			document.execCommand('copy');
			ta.remove();
		}
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}

	function keydown(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			onclose?.();
		} else if (e.key === 'Tab') {
			const f = [...dialog.querySelectorAll('button:not([disabled])')];
			if (!f.length) return;
			const first = f[0];
			const last = f[f.length - 1];
			if (!dialog.contains(document.activeElement)) {
				e.preventDefault();
				first.focus();
			} else if (e.shiftKey && document.activeElement === first) {
				e.preventDefault();
				last.focus();
			} else if (!e.shiftKey && document.activeElement === last) {
				e.preventDefault();
				first.focus();
			}
		} else if (e.key === '+' || e.key === '=') {
			zoomCenter(STEP);
		} else if (e.key === '-' || e.key === '_') {
			zoomCenter(1 / STEP);
		} else if (e.key === '0') {
			fit();
		}
	}

	onMount(() => {
		const prev = document.activeElement;
		const overflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		stage.addEventListener('wheel', onwheel, { passive: false });
		document.addEventListener('keydown', keydown, true);
		measure();
		fit();
		ready = true;
		dialog.focus();
		return () => {
			stage?.removeEventListener('wheel', onwheel);
			document.removeEventListener('keydown', keydown, true);
			document.body.style.overflow = overflow;
			if (prev instanceof HTMLElement && prev.isConnected) prev.focus();
		};
	});
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
	class="dv-backdrop"
	use:portal
	onclick={(e) => e.target === e.currentTarget && onclose?.()}
>
	<div
		class="dv"
		bind:this={dialog}
		role="dialog"
		aria-modal="true"
		aria-label="Diagram viewer"
		tabindex="-1"
	>
		<div class="dv-bar">
			<button type="button" onclick={() => zoomCenter(1 / STEP)} aria-label="Zoom out">−</button>
			<span class="dv-zoom">{Math.round(scale * 100)}%</span>
			<button type="button" onclick={() => zoomCenter(STEP)} aria-label="Zoom in">+</button>
			<button type="button" onclick={fit}>Fit</button>
			<button type="button" onclick={copy}>{copied ? 'Copied' : 'Copy source'}</button>
			<button type="button" class="dv-close" onclick={() => onclose?.()} aria-label="Close">Close</button>
		</div>
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div
			class="dv-stage"
			bind:this={stage}
			onpointerdown={ptrdown}
			onpointermove={ptrmove}
			onpointerup={ptrup}
			onpointercancel={ptrup}
			onclick={stageclick}
		>
			<div
				class="dv-content"
				bind:this={content}
				style:transform="translate({x}px, {y}px) scale({scale})"
				style:opacity={ready ? 1 : 0}
			>
				{@html svg}
			</div>
		</div>
	</div>
</div>

<style>
	.dv-backdrop {
		position: fixed;
		inset: 0;
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--s6);
		background: color-mix(in oklch, var(--ink) 45%, transparent);
	}
	.dv {
		position: relative;
		width: 100%;
		height: 100%;
		display: flex;
		flex-direction: column;
		background: var(--surface);
		color: var(--ink);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		overflow: hidden;
		outline: none;
		font-family: var(--font);
	}
	.dv-bar {
		display: flex;
		align-items: center;
		gap: var(--s2);
		padding: var(--s2) var(--s3);
		border-bottom: 1px solid var(--line);
		background: var(--surface);
		flex-wrap: wrap;
	}
	.dv-bar button {
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--ink-2);
		border-radius: var(--r-sm);
		font: inherit;
		font-size: var(--t-sm);
		min-width: 32px;
		height: 30px;
		padding: 0 var(--s3);
		cursor: pointer;
		transition:
			background var(--dur) var(--ease),
			color var(--dur) var(--ease);
	}
	.dv-bar button:hover {
		background: var(--hover);
		color: var(--ink);
	}
	.dv-bar button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 1px;
	}
	.dv-zoom {
		min-width: 44px;
		text-align: center;
		font-size: var(--t-sm);
		color: var(--ink-3);
		font-variant-numeric: tabular-nums;
	}
	.dv-close {
		margin-left: auto;
	}
	.dv-stage {
		position: relative;
		flex: 1;
		min-height: 0;
		overflow: hidden;
		background: var(--surface);
		touch-action: none;
		cursor: grab;
		user-select: none;
	}
	.dv-stage:active {
		cursor: grabbing;
	}
	.dv-content {
		position: absolute;
		top: 0;
		left: 0;
		transform-origin: 0 0;
		will-change: transform;
		transition: opacity var(--dur) var(--ease);
	}
	.dv-content :global(svg) {
		display: block;
		max-width: none;
	}
	@media (max-width: 640px) {
		.dv-backdrop {
			padding: 0;
		}
		.dv {
			border: 0;
			border-radius: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.dv-bar button,
		.dv-content {
			transition: none;
		}
	}
</style>
