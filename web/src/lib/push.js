// Web Push registration against the service worker + backend.
import { api } from './api.js';

function urlBase64ToUint8Array(base64String) {
	const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
	const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
	const raw = atob(base64);
	const arr = new Uint8Array(raw.length);
	for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
	return arr;
}

export function pushSupported() {
	return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
}

export async function registerServiceWorker() {
	if (!('serviceWorker' in navigator)) return null;
	try {
		return await navigator.serviceWorker.register('/sw.js');
	} catch {
		return null;
	}
}

export async function currentSubscription() {
	if (!pushSupported()) return null;
	const reg = await navigator.serviceWorker.ready;
	return reg.pushManager.getSubscription();
}

export async function enablePush(vapidPublicKey) {
	if (!pushSupported() || !vapidPublicKey) return { ok: false, reason: 'unsupported' };
	// Already hard-blocked at the browser/site level: requestPermission() would
	// resolve to 'denied' silently with no prompt, so tell the user to reset it.
	if (Notification.permission === 'denied') return { ok: false, reason: 'blocked' };
	try {
		const permission = await Notification.requestPermission();
		if (permission !== 'granted') return { ok: false, reason: 'blocked' };
		const reg = await navigator.serviceWorker.ready;
		let sub = await reg.pushManager.getSubscription();
		if (!sub) {
			sub = await reg.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: urlBase64ToUint8Array(vapidPublicKey)
			});
		}
		await api.pushSubscribe(sub.toJSON());
		return { ok: true };
	} catch (e) {
		// Samsung Internet / Android often reject subscribe() here — surface why.
		return { ok: false, reason: (e && e.message) || String(e) };
	}
}

export async function disablePush() {
	const sub = await currentSubscription();
	if (sub) {
		await api.pushUnsubscribe(sub.endpoint).catch(() => {});
		await sub.unsubscribe().catch(() => {});
	}
	return { ok: true };
}
