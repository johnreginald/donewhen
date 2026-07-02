<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';

	let setupRequired = $state(false);
	let email = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);

	onMount(async () => {
		try {
			const s = await api.authStatus();
			if (s.authenticated) {
				goto('/');
				return;
			}
			setupRequired = s.setupRequired;
		} catch {
			/* ignore */
		}
	});

	async function submit(e) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			if (setupRequired) await api.setup(email, password);
			else await api.login(email, password);
			location.href = '/';
		} catch (err) {
			error = err.message || 'failed';
		} finally {
			busy = false;
		}
	}
</script>

<div class="wrap">
	<form class="card" onsubmit={submit}>
		<div class="brand"><span class="logo">R</span> Raenil</div>
		<h1>{setupRequired ? 'Create your account' : 'Sign in'}</h1>
		{#if setupRequired}
			<p class="faint">First run — set up the single account for this tracker.</p>
		{/if}
		<label>Email</label>
		<input class="input" type="email" bind:value={email} autocomplete="username" required />
		<label>Password</label>
		<input
			class="input"
			type="password"
			bind:value={password}
			autocomplete={setupRequired ? 'new-password' : 'current-password'}
			required
		/>
		{#if error}<div class="error">{error}</div>{/if}
		<button class="btn primary" type="submit" disabled={busy}>
			{busy ? '…' : setupRequired ? 'Create account' : 'Sign in'}
		</button>
	</form>
</div>

<style>
	.wrap {
		height: 100%;
		display: grid;
		place-items: center;
		padding: 20px;
	}
	.card {
		width: min(360px, 100%);
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 26px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		box-shadow: var(--shadow);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 8px;
		font-weight: 600;
	}
	.logo {
		width: 26px;
		height: 26px;
		border-radius: 6px;
		background: var(--accent);
		color: #fff;
		display: grid;
		place-items: center;
		font-weight: 700;
	}
	h1 {
		font-size: 18px;
		margin: 8px 0 2px;
	}
	label {
		font-size: 12px;
		color: var(--text-dim);
		margin-top: 8px;
	}
	.error {
		color: #fca5a5;
		font-size: 13px;
		margin-top: 4px;
	}
	.btn.primary {
		margin-top: 16px;
		justify-content: center;
		padding: 9px;
	}
</style>
