<script>
	// Login, plus first-run setup. First run is a real two-step flow because
	// both steps already have a working endpoint: POST /auth/setup creates the
	// owner account AND grants a session (see internal/api/auth.go
	// handleSetup → s.grantSession), so the already-authenticated caller can
	// immediately POST /workspaces for step 2. No backend change needed — see
	// PP-215.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api.js';

	let phase = $state('loading'); // loading | login | setup1 | setup2

	let email = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let error = $state('');
	let rateLimited = $state(false);
	let busy = $state(false);
	let cooldownTimer;

	let wsName = $state('');
	let wsPrefix = $state('');
	let wsErr = $state('');

	onMount(() => {
		(async () => {
			try {
				const s = await api.authStatus();
				if (s.authenticated) {
					goto('/');
					return;
				}
				phase = s.setupRequired ? 'setup1' : 'login';
			} catch {
				phase = 'login';
			}
		})();
		return () => clearTimeout(cooldownTimer);
	});

	async function submitLogin(e) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			await api.login(email, password);
			location.href = '/';
		} catch (err) {
			if (err.status === 429) {
				rateLimited = true;
				error = 'Too many attempts — slow down.';
				clearTimeout(cooldownTimer);
				cooldownTimer = setTimeout(() => (rateLimited = false), 8000);
			} else if (err.status === 401) {
				error = 'Wrong email or password.';
			} else {
				error = err.message || 'Something went wrong — try again.';
			}
		} finally {
			busy = false;
		}
	}

	async function submitSetup1(e) {
		e.preventDefault();
		error = '';
		if (password.length < 8) {
			error = 'Password must be at least 8 characters.';
			return;
		}
		if (password !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}
		busy = true;
		try {
			await api.setup(email, password);
			password = '';
			confirmPassword = '';
			phase = 'setup2';
		} catch (err) {
			error =
				err.status === 409
					? 'This tracker is already set up. Sign in instead.'
					: err.message || 'Could not create the account.';
		} finally {
			busy = false;
		}
	}

	async function submitSetup2(e) {
		e.preventDefault();
		wsErr = '';
		if (!wsName.trim() || !wsPrefix.trim()) {
			wsErr = 'Name and key prefix are required.';
			return;
		}
		busy = true;
		try {
			await api.createWorkspace({ name: wsName.trim(), keyPrefix: wsPrefix.toUpperCase() });
			location.href = '/';
		} catch (err) {
			wsErr = err.message || 'Could not create the workspace.';
		} finally {
			busy = false;
		}
	}
</script>

{#if phase === 'loading'}
	<div class="auth-wrap">
		<div class="auth-brand">DoneWhen</div>
	</div>
{:else if phase === 'login'}
	<div class="auth-wrap">
		<div class="auth-brand">DoneWhen</div>
		<form class="auth-card" onsubmit={submitLogin}>
			<h1>Sign in</h1>
			<div class="settings-field">
				<label for="login-email">Email</label>
				<input
					id="login-email"
					class="input settings-input-wide"
					type="email"
					bind:value={email}
					autocomplete="username"
					required
				/>
			</div>
			<div class="settings-field">
				<label for="login-password">Password</label>
				<input
					id="login-password"
					class="input settings-input-wide"
					class:errb={!!error}
					type="password"
					bind:value={password}
					autocomplete="current-password"
					required
				/>
				{#if error}<span class="settings-err">{error}</span>{/if}
			</div>
			<button
				class="btn primary settings-btn-full"
				type="submit"
				disabled={busy || rateLimited}
				style="margin-top:8px"
			>
				{busy ? '…' : 'Sign in'}
			</button>
			{#if rateLimited}
				<span class="settings-hint" style="text-align:center">Try again in a few seconds.</span>
			{/if}
		</form>
	</div>
{:else if phase === 'setup1'}
	<div class="auth-wrap">
		<div class="auth-brand">DoneWhen</div>
		<div class="auth-steps">
			<span class="auth-stepdot on"></span><span class="auth-stepdot"></span>
			<span class="auth-stepn">Step 1 of 2</span>
		</div>
		<form class="auth-card" onsubmit={submitSetup1}>
			<h1>Create the owner account</h1>
			<p class="sub">First run — this becomes the workspace owner.</p>
			<div class="settings-field">
				<label for="su-email">Email</label>
				<input
					id="su-email"
					class="input settings-input-wide"
					type="email"
					placeholder="you@example.com"
					bind:value={email}
					autocomplete="username"
					required
				/>
			</div>
			<div class="settings-field">
				<label for="su-password">Password</label>
				<input
					id="su-password"
					class="input settings-input-wide"
					type="password"
					placeholder="········"
					bind:value={password}
					autocomplete="new-password"
					required
				/>
				<span class="settings-hint">At least 8 characters.</span>
			</div>
			<div class="settings-field">
				<label for="su-confirm">Confirm password</label>
				<input
					id="su-confirm"
					class="input settings-input-wide"
					type="password"
					placeholder="········"
					bind:value={confirmPassword}
					autocomplete="new-password"
					required
				/>
			</div>
			{#if error}<span class="settings-err">{error}</span>{/if}
			<button class="btn primary settings-btn-full" type="submit" disabled={busy} style="margin-top:8px">
				{busy ? '…' : 'Continue'}
			</button>
		</form>
	</div>
{:else if phase === 'setup2'}
	<div class="auth-wrap">
		<div class="auth-brand">DoneWhen</div>
		<div class="auth-steps">
			<span class="auth-stepdot on"></span><span class="auth-stepdot on"></span>
			<span class="auth-stepn">Step 2 of 2</span>
		</div>
		<form class="auth-card" onsubmit={submitSetup2}>
			<h1>Create your first workspace</h1>
			<p class="sub">Issues are keyed off this prefix.</p>
			<div class="settings-field">
				<label for="ws2-name">Name</label>
				<input
					id="ws2-name"
					class="input settings-input-wide"
					placeholder="My Workspace"
					bind:value={wsName}
					required
				/>
			</div>
			<div class="settings-field">
				<label for="ws2-prefix">Key prefix</label>
				<input
					id="ws2-prefix"
					class="input settings-input-short"
					placeholder="PP"
					bind:value={wsPrefix}
					maxlength="6"
					required
				/>
			</div>
			{#if wsPrefix.trim()}
				<div class="settings-hint settings-mono">
					New issues will look like {wsPrefix.toUpperCase()}-1, {wsPrefix.toUpperCase()}-2, …
				</div>
			{/if}
			{#if wsErr}<span class="settings-err">{wsErr}</span>{/if}
			<button class="btn primary settings-btn-full" type="submit" disabled={busy} style="margin-top:8px">
				{busy ? '…' : 'Create workspace'}
			</button>
		</form>
	</div>
{/if}
