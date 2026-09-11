const CACHE = 'giftlstr-v2';
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

/** Only successful, complete responses may be cached. Storing a truncated or
 * erroring response (e.g. a mid-deploy restart) would poison the cache and
 * serve "corrupt or truncated" assets until the cache is cleared. */
function cacheIfComplete(request, res) {
	if (res.ok && res.type === 'basic') {
		const copy = res.clone();
		caches
			.open(CACHE)
			.then((c) => c.put(request, copy))
			.catch(() => {});
	}
	return res;
}

self.addEventListener('fetch', (event) => {
	const { request } = event;
	if (request.method !== 'GET') return;
	// Never cache API traffic: the app must observe real status codes (token
	// exchange, discovery, data).
	if (request.url.includes('/api/')) {
		event.respondWith(fetch(request));
		return;
	}
	// The shell and its OAuth callback (/signin-oauth) are pre-cached at
	// install. Navigations stay network-first so each deploy is picked up
	// immediately, with the cached shell as offline fallback. Callback URLs
	// (which carry OAuth state/code) are never written to the cache.
	if (request.destination === 'document') {
		event.respondWith(fetch(request).catch(() => caches.match('/')));
		return;
	}
	// Immutable hashed assets: cache-first for speed; fill on miss only from
	// complete 2xx responses.
	event.respondWith(
		caches.match(request).then(
			(cached) => cached || fetch(request).then((res) => cacheIfComplete(request, res))
		)
	);
});