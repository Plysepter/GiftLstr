import { expect, test } from '@playwright/test';
import { ADMIN, ALICE, BOB, CAROL, apiToken, login } from './helpers.js';

test('API rejects unauthenticated requests', async ({ request }) => {
	const res = await request.get('/api/me');
	expect(res.status()).toBe(401);
});

test('UI shows the sign-in screen when not logged in', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('button', { name: 'Continue with Google' })).toBeVisible();
});

test.describe('authenticated', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	for (const who of [ADMIN, ALICE, BOB, CAROL]) {
		test(`provisions and signs in ${who.split(':')[0]} via API`, async ({ request }) => {
			const res = await request.get('/api/me', {
				headers: { Authorization: `Bearer e2e:${who}` },
			});
			expect(res.ok()).toBeTruthy();
			const body = await res.json();
			expect(body.user.displayName).toBe(who.split(':')[1]);
			// Each user owns exactly one active list, auto-created (a UUID id).
			expect(typeof body.activeListId).toBe('string');
			expect(body.activeListId.length).toBeGreaterThan(10);
		});
	}

	test('UI skips Clerk when an e2e token is present', async ({ page }) => {
		await login(page, ALICE);
		await page.goto('/');
		await expect(page.getByRole('heading', { name: /Alice[’']s Wishlist/ })).toBeVisible();
	});
});
