<script>
	import { activeWorkspace } from '$lib/store.js';
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';

	let members = $state([]);
	let loading = $state(true);
	let confirmId = $state('');

	let email = $state('');
	let role = $state('member');
	let addErr = $state('');
	let adding = $state(false);

	const canAdmin = $derived(['owner', 'admin'].includes($activeWorkspace?.role));
	const ownerCount = $derived(members.filter((m) => m.role === 'owner').length);

	async function load() {
		if (!$activeWorkspace) return;
		loading = true;
		try {
			members = (await api.members($activeWorkspace.id)) || [];
		} catch (e) {
			showToast('Could not load members: ' + (e?.message || e), 'error');
			members = [];
		} finally {
			loading = false;
		}
	}

	let loadedFor = $state(null);
	$effect(() => {
		const id = $activeWorkspace?.id;
		if (!id || loadedFor === id) return;
		loadedFor = id;
		load();
	});

	// Mirrors the server's own guard (internal/store/workspaces.go RemoveMember):
	// a workspace can never be left without an owner.
	function isLastOwner(m) {
		return m.role === 'owner' && ownerCount <= 1;
	}

	async function addMember() {
		addErr = '';
		if (!email.trim()) {
			addErr = 'Email is required.';
			return;
		}
		adding = true;
		try {
			await api.addMember($activeWorkspace.id, { email: email.trim(), role });
			email = '';
			role = 'member';
			await load();
			showToast('Member added');
		} catch (e) {
			addErr = e.message || 'Could not add member.';
		} finally {
			adding = false;
		}
	}

	// Code review: remove had no confirmation. Two clicks: arm, then confirm.
	async function removeMember(userId) {
		if (confirmId !== userId) {
			confirmId = userId;
			return;
		}
		confirmId = '';
		try {
			await api.removeMember($activeWorkspace.id, userId);
			await load();
			showToast('Member removed');
		} catch (e) {
			showToast(e.message || 'Could not remove member.', 'error');
		}
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">Members</h1>
		<p class="settings-lede">
			{$activeWorkspace?.name} · {members.length} member{members.length === 1 ? '' : 's'}.
		</p>
	</div>

	<section class="settings-panel">
		<div class="settings-panel-h">Members<span class="n">{members.length}</span></div>
		{#if loading}
			<p class="settings-hint">Loading…</p>
		{:else}
			{#each members as m (m.userId)}
				<div class="settings-row">
					<div>
						<div>{m.email}</div>
					</div>
					<span class="pill">{m.role}</span>
					{#if canAdmin}
						{#if confirmId === m.userId}
							<span class="settings-confirm">
								Remove {m.email}?
								<button class="btn danger settings-btn-sm" onclick={() => removeMember(m.userId)}>Confirm</button>
								<button class="btn ghost settings-btn-sm" onclick={() => (confirmId = '')}>Cancel</button>
							</span>
						{:else if isLastOwner(m)}
							<span class="settings-hint">can't remove — last owner</span>
						{:else}
							<button class="btn ghost settings-btn-sm" onclick={() => removeMember(m.userId)}>Remove</button>
						{/if}
					{/if}
				</div>
			{:else}
				<p class="settings-hint">No members listed.</p>
			{/each}
		{/if}
	</section>

	{#if canAdmin}
		<section class="settings-panel">
			<div class="settings-panel-h">Add a member</div>
			<div class="settings-inline">
				<input class="input" style="width:240px" placeholder="existing account email" bind:value={email} />
				<select class="select settings-input-short" bind:value={role}>
					<option value="member">member</option>
					<option value="admin">admin</option>
					<option value="owner">owner</option>
				</select>
				<button class="btn primary" onclick={addMember} disabled={adding}>{adding ? 'Adding…' : 'Add'}</button>
			</div>
			{#if addErr}<span class="settings-err">{addErr}</span>{/if}
			<span class="settings-hint">
				The account must already exist — create it with <span class="settings-mono">donewhen user &lt;email&gt; &lt;pass&gt;</span>.
			</span>
		</section>
	{/if}
</div>
