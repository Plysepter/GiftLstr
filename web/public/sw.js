const CACHE = 'giftlstr-v1';
// Build assets (hashed, immutable). Replaced at build time with the emitted
// asset manifest so every route chunk is available offline from first install.
const PRECACHE = [];

self.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(CACHE)
			.then((c) => c.addAll(['/', '/signin-oauth', ...PRECACHE]).catch(() => {}))
			.then(() => self.skipWaiting())
	);
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
			.then(() => self.clients.claim())
	);
});

// Cache-first for network requests, offline fallback everywhere else.
// Hashed build assets are immutable, so cache-first is fastest and always fresh;
// navigations (/...) and /api stay network-first with a cache fallback.
self.addEventListener('fetch', (event) => {
	const { request } = event;
	if (request.method !== 'GET') return;
	if (request.url.includes('/api/')) {
		event.respondWith(fetch(request));
		return;
	}
	if (request.destination === 'script' || request.destination === 'style' || request.destination === 'font') {
		event.respondWith(
			caches.match(request).then((cached) => {
				if (cached) return cached;
				return fetch(request).then((res) => {
					const copy = res.clone();
					caches.open(CACHE).then((c) => c.put(request, copy)).catch(() => {});
					return res;
				});
			})
		);
		return;
	}
	event.respondWith(
		fetch(request)
			.then((res) => {
				const copy = res.clone();
				caches.open(CACHE).then((c) => c.put(request, copy)).catch(() => {});
				return res;
			})
			.catch(() =>
				caches.match(request).then((hit) => hit || (request.mode === 'navigate' ? caches.match('/') : undefined))
			)
	);
});