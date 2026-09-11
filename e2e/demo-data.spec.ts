import { expect, test } from '@playwright/test';
import { ADMIN, ALICE, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

let aliceId = 0;
let danaId = 0;

test.beforeAll(async ({ request }) => {
	const admin = await (await request.get('/api/admin/users', { headers: auth(ADMIN) })).json();
	aliceId = admin.users.find((u) => u.username === 'alice').id;
	danaId = admin.users.find((u) => u.username === 'dana').id;
});

test.describe('demo dataset', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('the seeded accounts come from the canonical dataset', async ({ page }) => {
		// The curated demo accounts are served without a session so the
		// signed-out login page can render the picker.
		const res = await page.request.get('/api/e2e/demo-users');
		expect(res.ok()).toBeTruthy();
		const body = await res.json();
		const usernames = body.users.map((u: { username: string }) => u.username);
		for (const want of ['admin', 'alice', 'bob', 'carol', 'dana', 'eric', 'faye', 'george', 'helen', 'ivan']) {
			expect(usernames).toContain(want);
		}
		// Mock accounts exist only to exercise pagination; they are not demo accounts.
		expect(usernames).not.toContain('mock01');
	});

	test('seeded lists and claims are in place on users the other specs never touch', async ({ request }) => {
		// Dana's seeded list kept its items and eric's claim on them.
		const dana = await (await request.get(`/api/users/${danaId}/list`, { headers: auth('dana:Dana Date') })).json();
		const titles = dana.items.map((i: { title: string }) => i.title);
		expect(titles).toContain('Stand mixer');
		expect(titles).toContain('Watercolor set');
		const watercolor = dana.items.find((i: { title: string }) => i.title === 'Watercolor set');
		expect(watercolor.claimedByName).toBe('Eric Elderberry');

		// Helen is intentionally NOT connected to alice or dana: no access.
		const helenOnAlice = await request.get(`/api/users/${aliceId}/list`, { headers: auth('helen:Helen Honeydew') });
		expect(helenOnAlice.status()).toBe(403);
		const helenOnDana = await request.get(`/api/users/${danaId}/list`, { headers: auth('helen:Helen Honeydew') });
		expect(helenOnDana.status()).toBe(403);
	});

	test('alice is connected to exactly the six seeded users', async ({ request }) => {
		const lists = await (await request.get('/api/lists?page=1', { headers: auth(ALICE) })).json();
		expect(lists.total).toBe(6);
		const names = lists.users.map((u: { displayName: string }) => u.displayName);
		expect(names).toEqual(
			expect.arrayContaining(['Bob Banana', 'Carol Cherry', 'Dana Date', 'Eric Elderberry', 'Faye Fig', 'George Gooseberry']),
		);
		// helen and ivan are not among alice's connections.
		expect(names).not.toContain('Helen Honeydew');
		expect(names).not.toContain('Ivan Kiwi');
	});

	test('the admin account is seeded as admin', async ({ request }) => {
		const admin = await (await request.get('/api/me', { headers: auth(ADMIN) })).json();
		expect(admin.user.isAdmin).toBe(true);
	});

	test('login page offers the demo accounts and one-click sign-in works', async ({ page }) => {
		await page.goto('/');
		// The normal sign-in screen is still shown.
		await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();

		// Seeded demo accounts are listed for the picker.
		await expect(page.getByRole('button', { name: 'Ada Root · admin' })).toBeVisible();
		const aliceBtn = page.getByRole('button', { name: 'Alice Apple' });
		await expect(aliceBtn).toBeVisible();

		// Picking an account signs in with the matching e2e token.
		await aliceBtn.click();
		await expect(page.getByRole('heading', { name: /Alice[’']s Wishlist/ })).toBeVisible({ timeout: 10000 });
		const token = await page.evaluate(() => localStorage.getItem('e2e_token'));
		expect(token).toBe('e2e:alice:Alice Apple');
	});
});