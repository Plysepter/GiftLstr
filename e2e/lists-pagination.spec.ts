import { test, expect } from '@playwright/test';
import { ADMIN, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

test.describe('lists pagination with mock data', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('API paginates correctly with 30+ mock users', async ({ page }) => {
		await login(page, ADMIN);
		await page.goto('/');
		// Use page.evaluate to call API with e2e token (more reliable than request fixture in E2E mode)
		// ADMIN sees all users (non-admin would only see connected, but pagination is tested with admin)
		const b1 = await page.evaluate(async () => {
			const res = await fetch('/api/lists?page=1', {
				headers: { Authorization: 'Bearer e2e:admin:Ada Root' },
			}).then((r) => r.json());
			return res;
		});
		expect(b1.users.length).toBe(20);
		expect(b1.hasMore).toBe(true);
		expect(b1.total).toBeGreaterThanOrEqual(30);
		expect(b1.page).toBe(1);

		// Page 2: remaining users, hasMore false (or true if more than 40)
		const b2 = await page.evaluate(async () => {
			const res = await fetch('/api/lists?page=2', {
				headers: { Authorization: 'Bearer e2e:admin:Ada Root' },
			}).then((r) => r.json());
			return res;
		});
		expect(b2.users.length).toBeGreaterThan(0);
		expect(b2.users.length).toBeLessThanOrEqual(20);
		// Ensure no overlap between pages
		const ids1 = new Set(b1.users.map((u: any) => u.id));
		const overlap = b2.users.filter((u: any) => ids1.has(u.id));
		expect(overlap.length).toBe(0);
		// Total should be consistent
		expect(b2.total).toBe(b1.total);
	});

	test('UI paginates via Prev/Next buttons', async ({ page }) => {
		await login(page, ADMIN);
		await page.goto('/#/lists');
		await expect(page.getByRole('heading', { name: 'Lists' })).toBeVisible();
		// Page 1: Next enabled, Prev disabled
		const nextBtn = page.getByRole('button', { name: 'Next' });
		const prevBtn = page.getByRole('button', { name: 'Prev' });
		await expect(nextBtn).toBeEnabled();
		await expect(prevBtn).toBeDisabled();
		await expect(page.getByText('Page 1')).toBeVisible();
		const cardsPage1 = page.locator('.card');
		await expect(cardsPage1.first()).toBeVisible();
		const count1 = await cardsPage1.count();
		expect(count1).toBe(20);

		// Go to page 2
		await nextBtn.click();
		await expect(page.getByText('Page 2')).toBeVisible();
		await expect(prevBtn).toBeEnabled();
		const cardsPage2 = page.locator('.card');
		// Page 2 should have remaining users (total - 20)
		const count2 = await cardsPage2.count();
		expect(count2).toBeGreaterThan(0);
		expect(count2).toBeLessThanOrEqual(20);
		// Ensure Next is disabled when on last page (or enabled if more than 40 users)
		// With 33 total, page 2 is last, so Next should be disabled
		// Check via API what hasMore should be for page 2
		const apiPage2 = await page.evaluate(async () => {
			const res = await fetch('/api/lists?page=2', {
				headers: { Authorization: 'Bearer e2e:admin:Ada Root' },
			}).then((r) => r.json());
			return res.hasMore;
		});
		if (!apiPage2) {
			await expect(nextBtn).toBeDisabled();
		}
		// Back to page 1
		await prevBtn.click();
		await expect(page.getByText('Page 1')).toBeVisible();
		await expect(cardsPage1.first()).toBeVisible();
	});
});
