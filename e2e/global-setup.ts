import { ALICE, BOB, CAROL, ADMIN, apiToken, MOCK_USERS } from './helpers.js';

// Provision users up-front so "admin" is guaranteed to be the first user ever
// created (the first user becomes an admin) regardless of test file order.
// The E2E DB is throw-away (see scripts/e2e-server.sh: rm -rf e2e/.data) and
// is seeded exclusively with mock data. We create enough mock users to
// exercise pagination on the lists view (20 per page).
export default async function globalSetup(): Promise<void> {
	const base = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:8080';
	console.log('[globalSetup] waiting for server...');
	// Wait for server to be ready (webServer url is /api/openapi.yaml)
	for (let i = 0; i < 30; i++) {
		try {
			const r = await fetch(`${base}/api/openapi.yaml`);
			if (r.ok) break;
		} catch {}
		await new Promise((r) => setTimeout(r, 500));
	}
	console.log('[globalSetup] provisioning mock users...');
	// Core users first so ADMIN remains the initial admin
	for (const who of [ADMIN, ALICE, BOB, CAROL]) {
		const res = await fetch(`${base}/api/me`, {
			headers: { Authorization: `Bearer ${apiToken(who)}` },
		});
		if (!res.ok) console.log(`[globalSetup] failed to provision ${who}: ${res.status} ${await res.text()}`);
	}
	// Mock users for pagination and general scale testing
	for (const who of MOCK_USERS) {
		const res = await fetch(`${base}/api/me`, {
			headers: { Authorization: `Bearer ${apiToken(who)}` },
		});
		if (!res.ok) console.log(`[globalSetup] failed to provision ${who}: ${res.status} ${await res.text()}`);
	}
	console.log(`[globalSetup] done, created ${MOCK_USERS.length} mock users`);
	// Verify
	try {
		const r = await fetch(`${base}/api/admin/users`, {
			headers: { Authorization: `Bearer ${apiToken(ADMIN)}` },
		});
		const j = await r.json();
		console.log(`[globalSetup] total users provisioned: ${j.users?.length}`);
	} catch (e) {
		console.log('[globalSetup] verify failed', e);
	}
}
