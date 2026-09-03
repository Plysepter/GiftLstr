import { expect, test } from '@playwright/test';
import { ALICE, ADMIN, BOB, CAROL, apiToken, login } from './helpers.js';

let aliceId = 0;
let itemId = 0;
const title = `E2E Claimable ${Date.now()}`;

test.beforeAll(async ({ request }) => {
	const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });
	const me = await (await request.get('/api/me', { headers: auth(BOB) })).json();
	const admin = await (
		await request.get('/api/admin/users', { headers: auth('admin:Ada Root') })
	).json();
	aliceId = admin.users.find((u) => u.username === 'alice').id;
	// Connect bob and carol to alice so they can view her list.
	await request.put('/api/connections', {
		headers: auth('admin:Ada Root'),
		data: { a: me.user.id, b: aliceId, add: true },
	});
	const carolMe = await (await request.get('/api/me', { headers: auth(CAROL) })).json();
	await request.put('/api/connections', {
		headers: auth('admin:Ada Root'),
		data: { a: carolMe.user.id, b: aliceId, add: true },
	});
	const res = await request.post('/api/my/items', {
		headers: auth(ALICE),
		data: { title, url: 'https://example.com/x' },
	});
	itemId = (await res.json()).id;
});

test.describe('claiming on another user list', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('connected user can claim and unclaim via the UI', async ({ page }) => {
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const card = page.locator('.card', { hasText: title });
		await card.getByRole('button', { name: 'Claim', exact: true }).click();
		await expect(card.getByRole('button', { name: 'Unclaim' })).toBeVisible();

		// Lists page shows the claimed-items badge.
		await page.goto('/#/lists');
		await expect(
			page.locator('.card', { hasText: 'Alice Apple' }).getByText('1 claimed'),
		).toBeVisible();

		// Back on the list, release the claim again.
		await page.goto(`/#/list/${aliceId}`);
		const released = page.locator('.card', { hasText: title });
		await released.getByRole('button', { name: 'Unclaim' }).click();
		await expect(released.getByRole('button', { name: 'Claim', exact: true })).toBeVisible();
	});

	test('duplicate claims by another user are rejected with 409', async ({ request }) => {
		const authBob = { Authorization: `Bearer ${apiToken(BOB)}` };
		expect((await request.put(`/api/items/${itemId}/claim`, { headers: authBob })).ok()).toBeTruthy();
		const res = await request.put(`/api/items/${itemId}/claim`, {
			headers: { Authorization: `Bearer ${apiToken(CAROL)}` },
		});
		expect(res.status()).toBe(409);
		await request.delete(`/api/items/${itemId}/claim`, { headers: authBob });
	});

	test('owner cannot claim their own item (403)', async ({ request }) => {
		const res = await request.put(`/api/items/${itemId}/claim`, {
			headers: { Authorization: `Bearer ${apiToken(ALICE)}` },
		});
		expect(res.status()).toBe(403);
	});

	test('unconnected user cannot view the list (403)', async ({ request }) => {
		await request.get('/api/me', {
			headers: { Authorization: `Bearer e2e:eve:Eve Elderberry` },
		});
		const admin = await (
			await request.get('/api/admin/users', { headers: { Authorization: `Bearer e2e:admin:Ada Root` } })
		).json();
		const eve = admin.users.find((u) => u.username === 'eve');
		// Ensure no leftover connection from previous runs.
		await request.put('/api/connections', {
			headers: { Authorization: `Bearer e2e:admin:Ada Root` },
			data: { a: eve.id, b: aliceId, add: false },
		});
		const res = await request.get(`/api/users/${aliceId}/list`, {
			headers: { Authorization: `Bearer e2e:eve:Eve Elderberry` },
		});
		expect(res.status()).toBe(403);
	});

	test('soft-deleted item stays visible for its claimer, marked removed', async ({ page, request }) => {
		const authBob = { Authorization: `Bearer ${apiToken(BOB)}` };
		await test.step('bob claims the item', async () => {
			await login(page, BOB);
			await page.goto(`/#/list/${aliceId}`);
			const card = page.locator('.card', { hasText: title });
			await card.getByRole('button', { name: 'Claim', exact: true }).click();
			await expect(card.getByRole('button', { name: 'Unclaim' })).toBeVisible();
		});
		await test.step('alice removes it from her list', async () => {
			const res = await request.delete(`/api/my/items/${itemId}`, {
				headers: { Authorization: `Bearer ${apiToken(ALICE)}` },
			});
			expect(res.ok()).toBeTruthy();
		});
		await test.step('bob still sees it marked as removed', async () => {
			await page.reload();
			const card = page.locator('.card', { hasText: title });
			await expect(card.getByText('removed by owner')).toBeVisible();
			await expect(card.getByRole('button', { name: 'Unclaim' })).toBeVisible();
		});
		await request.delete(`/api/items/${itemId}/claim`, { headers: authBob });
	});

	test('owner no longer sees claimers; admins reveal them via the toggle', async ({ page, request }) => {
		test.setTimeout(60000);
		await request.put(`/api/items/${itemId}/claim`, {
			headers: { Authorization: `Bearer ${apiToken(BOB)}` },
		});

		// The owner no longer sees who claimed items on their own list: no
		// reveal toggle and no claimer names anywhere.
		await login(page, ALICE);
		await page.goto('/');
		await page.waitForTimeout(1000);
		await expect(page.getByRole('button', { name: /who claimed items/ })).toHaveCount(0);
		await expect(page.locator('body')).not.toContainText('claimed by');

		// An admin viewing another user's list opts in via the reveal toggle.
		// Switching users changes only localStorage (login uses an init script),
		// so reload to apply the admin session.
		await login(page, ADMIN);
		await page.goto(`/#/list/${aliceId}`);
		await page.reload();
		const reveal = page.getByRole('button', { name: 'View who claimed items' });
		const hide = page.getByRole('button', { name: 'Hide who claimed items' });
		await expect(reveal).toBeVisible();
		await expect(page.getByText(/claimed by/)).toHaveCount(0);

		await reveal.click();
		await expect(hide).toBeVisible();
		const card = page.locator('.card', { hasText: title }).first();
		await expect(card.getByText('claimed by Bob Banana')).toBeVisible({ timeout: 10000 });

		await request.delete(`/api/items/${itemId}/claim`, {
			headers: { Authorization: `Bearer ${apiToken(BOB)}` },
		});
	});
});
