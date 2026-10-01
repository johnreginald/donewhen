<script>
	import { api } from '$lib/api.js';
	import { states, archiveProject } from '$lib/store.js';
	import { archiveTarget, showToast } from '$lib/ui.js';

	// Confirm step for "Archive epic": counts the epic's open issues first so the
	// warning says how many will disappear from the everyday views.
	let openCount = $state(null); // null while counting
	let busy = $state(false);

	$effect(() => {
		const t = $archiveTarget;
		openCount = null;
		if (!t) return;
		let live = true;
		api
			.issues({ project: t.id })
			.then((list) => {
				if (!live) return;
				const closed = new Set(
					$states.filter((s) => s.category === 'completed' || s.category === 'canceled').map((s) => s.id)
				);
				openCount = (list || []).filter((i) => !closed.has(i.stateId)).length;
			})
			.catch(() => {
				if (live) openCount = 0;
			});
		return () => (live = false);
	});

	function close() {
		archiveTarget.set(null);
	}

	async function confirm() {
		const t = $archiveTarget;
		if (!t || busy) return;
		busy = true;
		try {
			await archiveProject(t.id);
			showToast(`${t.name} archived`);
			close();
		} catch (e) {
			showToast(e.message || 'Failed to archive', 'error');
		} finally {
			busy = false;
		}
	}

	function onKey(e) {
		if (e.key === 'Escape') {
			e.preventDefault();
			close();
		}
	}
</script>

{#if $archiveTarget}
	<div class="backdrop" role="presentation" onclick={close}></div>
	<div class="modal" role="alertdialog" aria-modal="true" aria-labelledby="arch-title" tabindex="-1" onkeydown={onKey}>
		<h2 id="arch-title">Archive "{$archiveTarget.name}"?</h2>
		{#if openCount === null}
			<p>Counting open issues…</p>
		{:else if openCount > 0}
			<p class="warn">
				{openCount} open {openCount === 1 ? 'issue' : 'issues'} will be hidden. Archive anyway?
			</p>
		{:else}
			<p>The epic and its issues will be hidden from the everyday views. You can unarchive it later.</p>
		{/if}
		<div class="foot">
			<button class="btn ghost" onclick={close}>Cancel</button>
			<button class="btn primary" onclick={confirm} disabled={busy || openCount === null}>
				{busy ? 'Archiving…' : openCount ? 'Archive anyway' : 'Archive'}
			</button>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		z-index: 80;
	}
	.modal {
		position: fixed;
		top: 20vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(420px, 92vw);
		padding: var(--s5);
		background: var(--surface);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-2);
		z-index: 81;
	}
	h2 {
		margin: 0 0 var(--s2);
		font-size: var(--t-md);
		color: var(--ink);
	}
	p {
		margin: 0 0 var(--s4);
		font-size: var(--t-base);
		color: var(--ink-2);
		line-height: 1.45;
	}
	p.warn {
		color: var(--ink);
	}
	.foot {
		display: flex;
		justify-content: flex-end;
		gap: var(--s2);
	}
</style>
