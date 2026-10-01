<script>
	import { onMount } from 'svelte';
	import { appConfig } from '$lib/store.js';
	import { showToast } from '$lib/ui.js';
	import { pushSupported, enablePush, disablePush, currentSubscription } from '$lib/push.js';

	let pushOn = $state(false);
	let pushBusy = $state(false);
	let pushPerm = $state('');
	let pushSupp = $state(true);
	let pushErr = $state('');

	function refreshPushState() {
		try {
			pushSupp = pushSupported();
			pushPerm = typeof Notification !== 'undefined' ? Notification.permission : 'n/a';
		} catch {
			/* ignore */
		}
	}

	onMount(async () => {
		try {
			pushOn = !!(await currentSubscription());
		} catch (e) {
			pushErr = e?.message || String(e);
		}
		refreshPushState();
	});

	async function togglePush() {
		pushBusy = true;
		try {
			if (pushOn) {
				await disablePush();
				pushOn = false;
				showToast('Push disabled');
			} else {
				const r = await enablePush($appConfig.vapidPublicKey);
				if (r.ok) {
					pushOn = true;
					pushErr = '';
					showToast('Push enabled');
				} else {
					pushErr = r.reason || 'unknown';
					if (r.reason === 'blocked') {
						showToast(
							'Notifications are blocked for this site. Allow them in the browser menu (site permissions), then retry.',
							'error'
						);
					} else if (r.reason === 'unsupported') {
						showToast("This browser can't do Web Push here.", 'error');
					} else {
						showToast('Push failed: ' + r.reason, 'error');
					}
				}
			}
		} catch (e) {
			pushErr = e?.message || String(e);
			showToast('Push failed: ' + pushErr, 'error');
		} finally {
			refreshPushState();
			pushBusy = false;
		}
	}
</script>

<div class="settings-wrap">
	<div class="settings-head">
		<h1 class="settings-h1">Notifications</h1>
		<p class="settings-lede">Web Push to this browser, and any device you've enabled it on.</p>
	</div>

	<section class="settings-panel">
		<div class="settings-panel-h">Web Push</div>
		{#if !$appConfig.pushEnabled}
			<div class="settings-note">
				Web Push is not configured on the server (set VAPID keys). In-app updates still work while
				the tab is open.
			</div>
		{:else if !pushSupp}
			<div class="settings-note">
				This browser can't do Web Push here. In-app updates still work while the tab is open.
			</div>
		{:else}
			<div class="settings-row">
				<span>Push notifications to this device</span>
				<button
					type="button"
					class="settings-switch"
					class:on={pushOn}
					role="switch"
					aria-checked={pushOn}
					disabled={pushBusy}
					onclick={togglePush}><i></i></button
				>
			</div>
		{/if}
		<div class="settings-hint settings-mono">
			subscribed: {pushOn ? 'yes' : 'no'} · permission: {pushPerm || '—'} · supported: {pushSupp
				? 'yes'
				: 'no'}
			{#if pushErr} · last error: {pushErr}{/if}
		</div>
	</section>
</div>
