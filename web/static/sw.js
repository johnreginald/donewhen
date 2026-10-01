// DoneWhen service worker: Web Push delivery + notification click routing.

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));

// Pass-through fetch handler (required for installability; no caching to avoid
// serving stale app assets).
self.addEventListener('fetch', () => {});

self.addEventListener('push', (event) => {
	let data = {};
	try {
		data = event.data ? event.data.json() : {};
	} catch {
		data = { title: 'DoneWhen', body: event.data ? event.data.text() : '' };
	}
	const title = data.title || 'DoneWhen';
	event.waitUntil(
		self.registration.showNotification(title, {
			body: data.body || '',
			tag: data.tag || undefined,
			renotify: !!data.tag,
			icon: '/icon-192.png',
			badge: '/badge-96.png',
			data: { url: data.url || '/' }
		})
	);
});

self.addEventListener('notificationclick', (event) => {
	event.notification.close();
	const url = (event.notification.data && event.notification.data.url) || '/';
	event.waitUntil(
		self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
			for (const client of list) {
				if ('focus' in client) {
					client.navigate(url);
					return client.focus();
				}
			}
			return self.clients.openWindow(url);
		})
	);
});
