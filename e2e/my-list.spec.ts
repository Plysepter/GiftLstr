import { expect, test } from '@playwright/test';
import { ALICE, apiToken, login } from './helpers.js';

test.beforeEach(async ({ request }) => {
	// Ensure alice exists.
	await request.get('/api/me', { headers: { Authorization: `Bearer ${apiToken(ALICE)}` } });
});

test.describe('my list (owner)', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('owner can add an item to their list', async ({ page }) => {
		await login(page, ALICE);
		await page.goto('/');
		await page.getByLabel('Gift idea').click();
		await page.getByLabel('Gift idea').fill('E2E Teapot');
		const addBtn = page.getByRole('button', { name: 'Add item' });
		await expect(addBtn).toBeEnabled({ timeout: 5000 });
		console.log('Add button enabled, clicking');
		const [resp] = await Promise.all([
			page.waitForResponse((r) => r.url().includes('/api/my/items') && r.request().method() === 'POST', { timeout: 10000 }).catch(() => null),
			addBtn.click(),
		]);
		if (resp) console.log('POST /api/my/items status', resp.status(), await resp.text().then((t) => t.slice(0, 500)).catch(() => 'no body'));
		else console.log('No POST response seen');
		await page.waitForTimeout(1500);
		const apiList = await page.evaluate(async () => {
			const auth = { Authorization: 'Bearer e2e:alice:Alice Apple' };
			const me = await fetch('/api/me', { headers: auth }).then((r) => r.json());
			const res = await fetch(`/api/users/${me.user.id}/list`, { headers: auth }).then((r) => r.json());
			return res.items;
		});
		console.log('API list items after add', JSON.stringify(apiList).slice(0, 500));
		await expect(page.locator('.card').first()).toBeVisible({ timeout: 10000 });
		await expect(page.locator('.card', { hasText: 'E2E Teapot' })).toBeVisible({ timeout: 5000 });

		// Persists across reload.
		await page.reload();
		await expect(page.locator('.card', { hasText: 'E2E Teapot' })).toBeVisible();
	});

	test('owner can soft-delete an item', async ({ page }) => {
		await login(page, ALICE);
		page.on('dialog', (d) => void d.accept());
		await page.goto('/');
		await page.getByPlaceholder('Gift idea').click();
		await page.getByPlaceholder('Gift idea').fill('E2E Removable');
		await page.getByRole('button', { name: 'Add item' }).click();
		const card = page.locator('.card', { hasText: 'E2E Removable' });
		await expect(card).toBeVisible();
		// Open actions on mobile / hover
		await card.click();
		await card.getByRole('button', { name: /Remove/ }).click();
		await expect(card.getByText('removed', { exact: true })).toBeVisible();
	});

	test('adding an item without a title fails with 400', async ({ request }) => {
		const res = await request.post('/api/my/items', {
			headers: { Authorization: `Bearer ${apiToken(ALICE)}` },
			data: {},
		});
		expect(res.status()).toBe(400);
	});
});
