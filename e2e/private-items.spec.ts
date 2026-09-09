import { expect, test, type Page } from '@playwright/test';
import { ALICE, BOB, ADMIN, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

let aliceId = 0;

test.beforeAll(async ({ request }) => {
	// Provision users and connect bob to alice so he can view her list.
	await request.get('/api/me', { headers: auth(ALICE) });
	await request.get('/api/me', { headers: auth(BOB) });
	const admin = await (await request.get('/api/admin/users', { headers: auth(ADMIN) })).json();
	aliceId = admin.users.find((u) => u.username === 'alice').id;
	const bobMe = await (await request.get('/api/me', { headers: auth(BOB) })).json();
	await request.put('/api/connections', {
		headers: auth(ADMIN),
		data: { a: bobMe.user.id, b: aliceId, add: true },
	});
});

/**
 * Adds a private idea via Alice's UI composer (the "Private ideas" section).
 * Returns the unique title used.
 */
async function addPrivateViaUI(page: Page, title: string): Promise<void> {
	await login(page, ALICE);
	await page.goto('/');
	// The private composer sits in the "Private ideas" section.
	const privateForm = page.locator('form.composer.is-private');
	await expect(privateForm).toBeVisible();
	const input = privateForm.getByLabel('Private gift idea');
	await input.click();
	await input.fill(title);
	await privateForm.getByRole('button', { name: 'Add item' }).click();
	await expect(page.locator('.card', { hasText: title })).toBeVisible({ timeout: 10000 });
}

test.describe('private ideas', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('owner can add a private idea and it does not appear to a connection', async ({ page, request }) => {
		const title = `E2E Secret ${Date.now()}`;
		await addPrivateViaUI(page, title);

		// Persists across reload (owner-side).
		await page.reload();
		await expect(page.locator('.card', { hasText: title })).toBeVisible();

		// A connected user does not see the private idea (only public / suggested).
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		await page.reload();
		await expect(page.locator('.card', { hasText: title })).toHaveCount(0);
	});

	test('owner can hard-delete a private idea after confirming', async ({ page, request }) => {
		const title = `E2E Secret Del ${Date.now()}`;
		await addPrivateViaUI(page, title);

		page.on('dialog', (d) => void d.accept());
		const card = page.locator('.card', { hasText: title });
		await card.click();
		await card.getByRole('button', { name: `Delete private idea ${title}` }).click();
		const dlg = page.getByRole('dialog', { name: 'Delete private idea?' });
		await expect(dlg).toBeVisible();
		await dlg.getByRole('button', { name: 'Delete' }).click();
		await expect(page.locator('.card', { hasText: title })).toHaveCount(0);
	});

	test('private items are not exposed via the public list API', async ({ request }) => {
		const stamp = Date.now();
		const title = `E2E API Secret ${stamp}`;
		const res = await request.post('/api/my/private-items', {
			headers: auth(ALICE),
			data: { title, url: '' },
		});
		expect(res.ok()).toBeTruthy();
		const privateId = (await res.json()).id;

		const ownerList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(ALICE) })).json();
		expect(ownerList.privateItems?.some((i) => i.id === privateId)).toBeTruthy();
		const publicTitles = ownerList.items.map((i) => i.title);
		expect(publicTitles).not.toContain(title);

		// A connection sees neither the private item nor the privateItems field.
		const bobList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(BOB) })).json();
		expect(bobList.privateItems ?? []).toHaveLength(0);
		expect(bobList.items.map((i) => i.title)).not.toContain(title);
	});
});

test.describe('share-to-suggested flow', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('owner shares a private idea, converting it to a suggestion visible to a connection', async ({ page, request }) => {
		const title = `E2E Shared ${Date.now()}`;
		await addPrivateViaUI(page, title);

		// Owner clicks Share on the private idea and confirms the dialog.
		const card = page.locator('.card', { hasText: title });
		await card.click();
		await card.getByRole('button', { name: `Share ${title}` }).click();
		const dlg = page.getByRole('dialog', { name: 'Share this idea?' });
		await expect(dlg).toBeVisible();
		await dlg.getByRole('button', { name: 'Share' }).click();

		// The idea moves into the "Suggested" section for the owner.
		const suggestedSection = page.locator('ul.suggested-list').first();
		await expect(suggestedSection.locator('.card', { hasText: title })).toBeVisible({ timeout: 10000 });

		// The connection can now see it as "Suggested by Alice".
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		await page.reload();
		const bobCard = page.locator('.card', { hasText: title });
		await expect(bobCard).toBeVisible();
		await expect(bobCard.getByText(/Suggested by/)).toBeVisible();
	});

	test('owner can unshare (remove) a suggested item after confirming', async ({ page, request }) => {
		const title = `E2E Unshare ${Date.now()}`;
		// Seed a shared suggestion via the API.
		const privateRes = await request.post('/api/my/private-items', {
			headers: auth(ALICE),
			data: { title, url: '' },
		});
		const privateId = (await privateRes.json()).id;
		await request.put(`/api/my/private-items/${privateId}/share`, { headers: auth(ALICE) });

		await login(page, ALICE);
		await page.goto('/');
		const suggestedCard = page.locator('.card', { hasText: title });
		await expect(suggestedCard).toBeVisible();

		page.on('dialog', (d) => void d.accept());
		await suggestedCard.click();
		await suggestedCard.getByRole('button', { name: `Unshare ${title}` }).click();
		const dlg = page.getByRole('dialog', { name: 'Unshare this idea?' });
		await expect(dlg).toBeVisible();
		await dlg.getByRole('button', { name: 'Unshare' }).click();
		await expect(page.locator('.card', { hasText: title })).toHaveCount(0);
	});

	test('a connection can claim a suggested item', async ({ page, request }) => {
		const title = `E2E Claim Suggestion ${Date.now()}`;
		const privateRes = await request.post('/api/my/private-items', {
			headers: auth(ALICE),
			data: { title, url: '' },
		});
		const privateId = (await privateRes.json()).id;
		await request.put(`/api/my/private-items/${privateId}/share`, { headers: auth(ALICE) });

		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const card = page.locator('.card', { hasText: title });
		await expect(card).toBeVisible();
		await card.getByRole('button', { name: 'Claim', exact: true }).click();
		await expect(card.getByRole('button', { name: 'Unclaim' })).toBeVisible();
	});
});

test.describe('suggesting on another user list', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('a connected user can suggest an idea to the owner via the UI', async ({ page, request }) => {
		const title = `E2E Suggested By Bob ${Date.now()}`;
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const composer = page.locator('form.composer');
		const input = composer.getByLabel('Suggest gift idea');
		await input.click();
		await input.fill(title);
		await composer.getByRole('button', { name: 'Add item' }).click();

		// Bob sees it with a "Suggested by" badge on the owner's list.
		const card = page.locator('.card', { hasText: title });
		await expect(card).toBeVisible({ timeout: 10000 });
		await expect(card.getByText(/Suggested by Bob/)).toBeVisible();

		// The owner sees it in their Suggested section.
		await login(page, ALICE);
		await page.goto('/');
		await page.reload();
		const suggestedSection = page.locator('ul.suggested-list').first();
		await expect(suggestedSection.locator('.card', { hasText: title })).toBeVisible({ timeout: 10000 });
	});

	test("an unconnected user's suggestion is rejected with 403", async ({ request }) => {
		await request.get('/api/me', { headers: auth('eve:Eve Elderberry') });
		const res = await request.post(`/api/users/${aliceId}/suggestions`, {
			headers: auth('eve:Eve Elderberry'),
			data: { title: 'Sneaky', url: '' },
		});
		expect(res.status()).toBe(403);
	});

	test('the owner cannot suggest to themselves (400)', async ({ request }) => {
		const me = await (await request.get('/api/me', { headers: auth(ALICE) })).json();
		const res = await request.post(`/api/users/${me.user.id}/suggestions`, {
			headers: auth(ALICE),
			data: { title: 'For me', url: '' },
		});
		expect(res.status()).toBe(400);
	});
});
