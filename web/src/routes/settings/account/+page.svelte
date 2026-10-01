<script>
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';
	import { me } from '$lib/store.js';
	import { theme, setTheme } from '$lib/theme.js';
	import { showToast } from '$lib/ui.js';

	let signingOut = $state(false);

	// Code review: logout had no try/catch. A failed logout must not strand the
	// user believing they're signed out when the server never cleared the cookie.
	async function signOut() {
		signingOut = true;
		try {
			await api.logout();
			goto('/login', { replaceState: true });
		} catch (e) {
			showToast('Could not sign out: ' + (e?.message || e), 'error');
		} finally {
			signingOut = false;
		}
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">Account</h1>
		<p class="settings-lede">Your sign-in and how Raenil looks to you.</p>
	</div>

	<section class="settings-panel">
		<div class="settings-panel-h">Profile</div>
		<div class="settings-field">
			<label for="acct-email">Email</label>
			<input id="acct-email" class="input settings-input-wide" value={$me?.email || ''} disabled />
			<span class="settings-hint">Contact an owner to change the address on your account.</span>
		</div>
	</section>

	<section class="settings-panel">
		<div class="settings-panel-h">Appearance</div>
		<div class="settings-field">
			<label id="theme-label" for="theme-seg">Theme</label>
			<div id="theme-seg" class="settings-seg" role="radiogroup" aria-labelledby="theme-label">
				{#each [['system', 'System'], ['light', 'Light'], ['dark', 'Dark']] as [v, label] (v)}
					<button
						type="button"
						class:on={$theme === v}
						role="radio"
						aria-checked={$theme === v}
						onclick={() => setTheme(v)}>{label}</button
					>
				{/each}
			</div>
			<span class="settings-hint">System follows your OS setting. Saved on this device.</span>
		</div>
	</section>

	<section class="settings-panel">
		<div class="settings-panel-h">Session</div>
		<div class="settings-row">
			<span>Signed in as <b>{$me?.email}</b></span>
			<button class="btn danger" onclick={signOut} disabled={signingOut}>
				{signingOut ? 'Signing out…' : 'Sign out'}
			</button>
		</div>
	</section>
</div>
