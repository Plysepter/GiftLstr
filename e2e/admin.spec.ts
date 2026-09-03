import { expect, test } from '@playwright/test';
import { ADMIN, ALICE, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

test.describe('admin connections', () => {
	test('non-admin cannot access admin endpoints (403)', async ({ request }) => {
		const res = await request.get('/api/admin/users', { headers: auth(ALICE) });
		expect(res.status()).toBe(403);
	});

	test('admin can add and remove a connection via the UI', async ({ page, request }) => {
		test.setTimeout(60000);
		const adminUsers = await (
			await request.get('/api/admin/users', { headers: auth(ADMIN) })
		).json();
		const alice = adminUsers.users.find((u) => u.username === 'alice');
		await request.get('/api/me', { headers: auth('dave:Dave Durian') });
		const refreshed = await (
			await request.get('/api/admin/users', { headers: auth(ADMIN) })
		).json();
		const dave = refreshed.users.find((u) => u.username === 'dave');

		await login(page, ADMIN);
		await page.goto('/#/admin');
		await expect(page.getByRole('heading', { name: 'Admin', exact: true })).toBeVisible();

		const usersSection = page.locator('section', { has: page.getByRole('heading', { name: 'Users' }) });
		const aliceCard = usersSection.locator('.card', { hasText: alice.displayName }).first();
		await expect(aliceCard).toBeVisible();

		// Open the manage-connections modal and search for a new connection there.
		await aliceCard.getByRole('button', { name: 'Manage connections' }).click();
		const aliceModal = page.getByRole('dialog', { name: `Connections for ${alice.displayName}` });
		await expect(aliceModal).toBeVisible();
		const searchInput = aliceModal.getByPlaceholder('Search to connect…');
		await searchInput.fill(dave.displayName);
		// wait for suggestion
		const suggestion = aliceModal.getByRole('button', { name: new RegExp(dave.displayName) });
		await expect(suggestion).toBeVisible({ timeout: 10000 });
		await suggestion.click();

		// Dave can now view Alice's list via API (poll until the UI call lands).
		await expect
			.poll(async () =>
				(await request.get(`/api/users/${alice.id}/list`, { headers: auth('dave:Dave Durian') })).status(),
			)
			.toBe(200);

		// Also wait for UI to reflect the new connection (admin users reload)
		const daveCard = usersSection.locator('.card', { hasText: dave.displayName }).first();
		await expect(daveCard).toBeVisible({ timeout: 10000 });
		await expect(daveCard.getByText('1 connections')).toBeVisible({ timeout: 10000 });

		// Close the connections modal before interacting with the cards again.
		const aliceClose = aliceModal.getByRole('button', { name: 'Close' });
		if (await aliceClose.count() > 0) await aliceClose.click();
		await expect(aliceModal).toBeHidden();

		// Open modal for Dave to remove connection (now a <dialog class="edit-dlg"> like Edit Item)
		await daveCard.scrollIntoViewIfNeeded();
		await daveCard.getByRole('button', { name: 'Manage connections' }).click();
		const daveModal = page.getByRole('dialog', { name: `Connections for ${dave.displayName}` });
		await expect(daveModal).toBeVisible();
		await expect(daveModal.getByRole('heading', { name: new RegExp(dave.displayName) })).toBeVisible();
		const removeBtn = daveModal.locator('li', { hasText: 'Alice Apple' }).getByRole('button', { name: 'Remove' });
		if (await removeBtn.count() > 0) {
			await removeBtn.click();
		} else {
			await daveModal.getByRole('button', { name: 'Remove' }).first().click();
		}
		// Confirm the removal in the confirmation dialog.
		const confirmDlg = page.getByRole('dialog', { name: 'Remove connection?' });
		await expect(confirmDlg).toBeVisible();
		await confirmDlg.getByRole('button', { name: 'Remove' }).click();
		await expect
			.poll(async () =>
				(await request.get(`/api/users/${alice.id}/list`, { headers: auth('dave:Dave Durian') })).status(),
			)
			.toBe(403);
	});
});
