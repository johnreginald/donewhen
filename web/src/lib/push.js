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
	const permission = await Notification.requestPermission();
	if (permission !== 'granted') return { ok: false, reason: 'denied' };
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
}

export async function disablePush() {
	const sub = await currentSubscription();
	if (sub) {
		await api.pushUnsubscribe(sub.endpoint).catch(() => {});
		await sub.unsubscribe().catch(() => {});
	}
	return { ok: true };
}
