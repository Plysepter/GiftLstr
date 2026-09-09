import { expect, test } from '@playwright/test';
import { login, apiToken } from './helpers.js';

// A brand-new user that global-setup does NOT provision, so the first /api/me
// call creates the account and marks it isNew=true.
const NEW_USER = 'neww:Wren Newguy';
const SKIP_USER = 'neww-skip:Wren Newguy';
const AUTH = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

test.describe('first run', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('a brand-new user is prompted for their birthday on their list page and can set it', async ({ page, request }) => {
		await login(page, NEW_USER);
		await page.goto('/');
		// Lands on the my-list page with the first-run birthday modal open.
		const dialog = page.getByRole('dialog', { name: 'Set your birthday' });
		await expect(dialog).toBeVisible();
		await expect(dialog.getByText("What’s your birthday?")).toBeVisible();

		await dialog.getByLabel('Birth month').selectOption('05');
		await dialog.getByLabel('Birth day').selectOption('15');
		await dialog.getByRole('button', { name: 'Save' }).click();
		await expect(dialog).not.toBeVisible();

		// The birthday is saved server-side.
		const me = await (await request.get('/api/me', { headers: AUTH(NEW_USER) })).json();
		expect(me.user.birthday).toBe('05-15');
	});

	test('a brand-new user can pick an additional reset holiday with their birthday', async ({ page, request }) => {
		const who = 'neww-holiday:Wren Newguy';
		const AUTH2 = (name: string) => ({ Authorization: `Bearer ${apiToken(name)}` });
		await login(page, who);
		await page.goto('/');
		const dialog = page.getByRole('dialog', { name: 'Set your birthday' });
		await expect(dialog).toBeVisible();

		await dialog.getByLabel('Birth month').selectOption('06');
		await dialog.getByLabel('Birth day').selectOption('10');
		await dialog.getByLabel('Additional reset holiday').selectOption('fathers');
		await dialog.getByRole('button', { name: 'Save' }).click();
		await expect(dialog).not.toBeVisible();

		const me = await (await request.get('/api/me', { headers: AUTH2(who) })).json();
		expect(me.user.birthday).toBe('06-10');
		expect(me.user.holidayReset).toBe('fathers');
	});

	test('a brand-new user can skip the birthday prompt', async ({ page }) => {
		await login(page, SKIP_USER);
		await page.goto('/');
		const dialog = page.getByRole('dialog', { name: 'Set your birthday' });
		await expect(dialog).toBeVisible();
		await dialog.getByRole('button', { name: 'Skip' }).click();
		await expect(dialog).not.toBeVisible();
	});

	test('the default theme style is aurora', async ({ page }) => {
		await login(page, NEW_USER);
		await page.goto('/#/profile');
		// Aurora comes first and is highlighted as the active style.
		const aurora = page.getByRole('button', { name: /Aurora/ });
		await expect(aurora).toBeVisible();
		await expect(aurora).toHaveClass(/on/);
		const first = page.getByRole('radiogroup', { name: 'Theme style' }).getByRole('button').first();
		await expect(first).toContainText('Aurora');
	});
});
