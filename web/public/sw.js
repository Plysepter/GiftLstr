// Cache version. The build plugin replaces the literal version token below with
// a per-build id so a deploy flushes stale hashed assets from the previous version.
const CACHE = 'giftlstr-v1';
// Build assets (hashed, immutable). Replaced at build time with the emitted
// asset manifest so every route chunk is available offline from first install.
const PRECACHE = [];

// Read-only API responses are kept for offline viewing. Cached copies older
// than this are treated as a miss (and pruned) instead of served.
const CACHE_MAX_AGE = 7 * 24 * 60 * 60 * 1000;
const CACHED_AT_HEADER = 'giftlstr-cached-at';

// API endpoints whose GET responses may be served offline: the signed-in
// user's profile (lets the app boot without a network), the shared-lists
// index, and every list page (own or shared) including archive views.
function isApiRead(pathname) {
	if (pathname === '/api/me') return true;
	if (pathname === '/api/lists') return true;
	if (pathname === '/api/archive') return true;
	if (/^\/api\/archive\/[^/]+$/.test(pathname)) return true;
	if (/^\/api\/users\/[^/]+\/list$/.test(pathname)) return true;
	return false;
}

function base64UrlDecode(s) {
	const b64 = s.replace(/-/g, '+').replace(/_/g, '/');
	const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4);
	return atob(padded);
}

// Cached read responses are keyed per account, not per URL, so two accounts on
// one browser cannot reach each other's offline data. The OAuth access token's
// `sub` claim is stable across refreshes (the token itself rotates), so it is
// a reliable owner key for the timeout window. Unparseable or missing bearer
// tokens get no offline caching at all.
function userKey(request) {
	const auth = request.headers.get('authorization');
	if (!auth) return null;
	const m = /^Bearer\s+(.+)$/i.exec(auth);
	if (!m) return null;
	const parts = m[1].split('.');
	if (parts.length !== 3) return null;
	try {
		const payload = JSON.parse(base64UrlDecode(parts[1]));
		return typeof payload.sub === 'string' && payload.sub ? payload.sub : null;
	} catch {
		return null;
	}
}

/** The cache key for a read request, scoped to the signed-in account. */
function keyedRequest(request, sub) {
	const glue = request.url.includes('?') ? '&' : '?';
	return new Request(request.url + glue + 'giftlstr-user=' + encodeURIComponent(sub), {
		method: 'GET',
	});
}

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

// Signing out must not leave another account's list data reachable offline.
self.addEventListener('message', (event) => {
	if (event.data && event.data.type === 'giftlstr-clear') {
		event.waitUntil(
			caches
				.keys()
				.then((keys) => Promise.all(keys.map((k) => caches.delete(k))))
		);
	}
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

/** Stamps an API read response with its cached-at time before storing it. */
function cacheApiRead(request, res) {
	if (!res.ok || res.type !== 'basic') return;
	const copy = res.clone();
	const headers = new Headers(copy.headers);
	headers.set(CACHED_AT_HEADER, String(Date.now()));
	const stored = new Response(copy.body, {
		status: copy.status,
		statusText: copy.statusText,
		headers,
	});
	caches
		.open(CACHE)
		.then((c) => c.put(request, stored))
		.catch(() => {});
}

function isFresh(res) {
	const t = Number(res.headers.get(CACHED_AT_HEADER) || 0);
	return t > 0 && Date.now() - t < CACHE_MAX_AGE;
}

/** Network-first for reads: a fresh copy wins and refreshes the cache; when
 * the network fails a cached copy from the last week is served (offline).
 * Copies are scoped to the account that fetched them (see userKey). */
async function serveApiRead(request) {
	const sub = userKey(request);
	const cacheKey = sub ? keyedRequest(request, sub) : null;
	try {
		const res = await fetch(request);
		if (cacheKey) cacheApiRead(cacheKey, res);
		return res;
	} catch {
		if (!cacheKey) return Response.error();
		const cached = await caches.match(cacheKey);
		if (cached && isFresh(cached)) return cached;
		if (cached) {
			const c = await caches.open(CACHE);
			c.delete(cacheKey);
		}
		// No usable copy: surface a network error so the app shows its offline state.
		return Response.error();
	}
}

self.addEventListener('fetch', (event) => {
	const { request } = event;
	if (request.method !== 'GET') return;
	const url = new URL(request.url);
	if (url.origin !== self.location.origin) return;
	// GET /api/ reads that are safe to replay offline are served from cache
	// when the network is down. Everything else under /api/ stays
	// network-only (mutations, token exchange, metadata) so the app observes
	// real status codes and offline writes keep queueing.
	if (url.pathname.startsWith('/api/')) {
		if (isApiRead(url.pathname)) {
			event.respondWith(serveApiRead(request));
		}
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