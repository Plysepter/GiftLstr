import { expect, test, type Page } from '@playwright/test';
import { ALICE, BOB, CAROL, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

/**
 * Signs in as `who` and navigates to `hash`. Because changing only the URL hash
 * does not reload the document, the new e2e token from `login()` would not take
 * effect — so we reload after navigating to force the fresh session.
 */
async function gotoAs(page: Page, who: string, hash: string): Promise<void> {
	await login(page, who);
	await page.goto(hash);
	await page.reload();
}

test.describe('share-code connections', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('profile share toggle generates a code, and another user connects via the Lists page', async ({ page, request }) => {
		// Provision alice (code owner) and bob/carol (joiners).
		await request.get('/api/me', { headers: auth(ALICE) });
		await request.get('/api/me', { headers: auth(BOB) });
		await request.get('/api/me', { headers: auth(CAROL) });

		// 1) Alice enables sharing on her profile -> a code is shown.
		await gotoAs(page, ALICE, '/#/profile');
		const shareToggle = page.getByLabel('Enable others to connect to me');
		await shareToggle.check();
		const codeEl = page.getByLabel('Your share code');
		await expect(codeEl).toBeVisible();
		const shareCode = (await codeEl.textContent()) ?? '';
		expect(shareCode).toMatch(/^[A-Z2-9]{8}$/); // 8-char code from a non-ambiguous alphabet

		// 2) Bob connects to alice using her code on the Lists page.
		await gotoAs(page, BOB, '/#/lists');
		await page.getByRole('textbox', { name: 'Share code' }).fill(shareCode);
		await page.getByRole('button', { name: 'Connect with share code' }).click();
		await expect(page.getByText(`Connected to Alice Apple!`)).toBeVisible();

		// 3) The connection is visible to alice: bob now appears on her Lists page.
		await gotoAs(page, ALICE, '/#/lists');
		await expect(page.getByText('Bob Banana')).toBeVisible();
	});

	test('disabling the share toggle hides the code and future connections fail', async ({ page, request }) => {
		await request.get('/api/me', { headers: auth(ALICE) });
		await request.get('/api/me', { headers: auth(CAROL) });

		// Enable then disable.
		await gotoAs(page, ALICE, '/#/profile');
		const toggle = page.getByLabel('Enable others to connect to me');
		await toggle.check();
		await expect(page.getByLabel('Your share code')).toBeVisible();
		await toggle.uncheck();
		await expect(page.getByLabel('Your share code')).not.toBeVisible();

		// Using an invalid code errors visibly.
		await gotoAs(page, CAROL, '/#/lists');
		await page.getByRole('textbox', { name: 'Share code' }).fill('ZZZZZZZZ');
		await page.getByRole('button', { name: 'Connect with share code' }).click();
		await expect(page.getByText(/could not connect|no user found|not found/i)).toBeVisible();
	});

	test('users cannot connect to themselves with their own code', async ({ page, request }) => {
		await request.get('/api/me', { headers: auth(ALICE) });

		await login(page, ALICE);
		await page.goto('/#/profile');
		await page.getByLabel('Enable others to connect to me').check();
		const codeEl = page.getByLabel('Your share code');
		await expect(codeEl).toBeVisible();
		const code = (await codeEl.textContent()) ?? '';

		await page.goto('/#/lists');
		await page.getByRole('textbox', { name: 'Share code' }).fill(code);
		await page.getByRole('button', { name: 'Connect with share code' }).click();
		await expect(page.getByText(/own code|cannot connect.*yourself/)).toBeVisible();
	});
});
