import { expect, test, type APIRequestContext } from '@playwright/test';
import { ALICE, BOB, CAROL, ADMIN, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

let aliceId = 0;
let bobId = 0;
let carolId = 0;

test.beforeAll(async ({ request }) => {
	// Provision users and connect bob + carol to alice so they can view her list.
	await request.get('/api/me', { headers: auth(ALICE) });
	await request.get('/api/me', { headers: auth(BOB) });
	await request.get('/api/me', { headers: auth(CAROL) });
	const admin = await (await request.get('/api/admin/users', { headers: auth(ADMIN) })).json();
	aliceId = admin.users.find((u) => u.username === 'alice').id;
	bobId = admin.users.find((u) => u.username === 'bob').id;
	carolId = admin.users.find((u) => u.username === 'carol').id;
	for (const [a, b] of [[bobId, aliceId], [carolId, aliceId]] as const) {
		await request.put('/api/connections', {
			headers: auth(ADMIN),
			data: { a, b, add: true },
		});
	}
});

async function savePrivateIdea(request: APIRequestContext, token: string, ownerId: string, title: string): Promise<string> {
	const res = await request.post(`/api/users/${ownerId}/private-items`, {
		headers: auth(token),
		data: { title, url: '' },
	});
	expect(res.ok()).toBeTruthy();
	return (await res.json()).id;
}

async function sharePrivateIdea(request: APIRequestContext, token: string, ownerId: string, itemId: string): Promise<void> {
	const res = await request.put(`/api/users/${ownerId}/private-items/${itemId}/share`, {
		headers: auth(token),
	});
	expect(res.ok()).toBeTruthy();
}

test.describe('private ideas for someone else\'s list', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('a connected user saves a private idea that only they can see', async ({ page, request }) => {
		const title = `E2E Private ${Date.now()}`;
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const composer = page.locator('.suggest-bar form.composer');
		const input = composer.getByLabel('Add private idea');
		await input.click();
		await input.fill(title);
		await composer.getByRole('button', { name: 'Add item' }).click();

		const privateCard = page.locator('.private-list .card', { hasText: title });
		await expect(privateCard).toBeVisible({ timeout: 10000 });
		await expect(page.locator('.private-divider')).toContainText('Only you can see these');

		// The author sees it under privateItems on the list API.
		const bobList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(BOB) })).json();
		expect(bobList.privateItems.map((i: { title: string }) => i.title)).toContain(title);
		expect(bobList.items.map((i: { title: string }) => i.title)).not.toContain(title);

		// The owner never sees it on their list or My List.
		const aliceList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(ALICE) })).json();
		expect(aliceList.items.map((i: { title: string }) => i.title)).not.toContain(title);
		expect(aliceList.privateItems ?? []).toHaveLength(0);

		await login(page, ALICE);
		await page.goto('/');
		await page.reload();
		await expect(page.getByText(title)).toHaveCount(0);
	});

	test('a private idea can be marked claimed and then unclaimed', async ({ page, request }) => {
		const title = `E2E Private Claim ${Date.now()}`;
		await savePrivateIdea(request, BOB, aliceId, title);
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const privateCard = page.locator('.private-list .card', { hasText: title });
		await expect(privateCard).toBeVisible({ timeout: 10000 });

		await privateCard.getByRole('button', { name: 'Claim', exact: true }).click();
		await expect(privateCard.getByRole('button', { name: 'Unclaim', exact: true })).toBeVisible();
		await expect(privateCard.getByText('your pick')).toBeVisible();

		await privateCard.getByRole('button', { name: 'Unclaim', exact: true }).click();
		await expect(privateCard.getByRole('button', { name: 'Claim', exact: true })).toBeVisible();
	});

	test('sharing publishes an idea to the list and leaves the private section', async ({ page, request }) => {
		const title = `E2E Share ${Date.now()}`;
		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const composer = page.locator('.suggest-bar form.composer');
		await composer.getByLabel('Add private idea').fill(title);
		await composer.getByRole('button', { name: 'Add item' }).click();

		const privateCard = page.locator('.private-list .card', { hasText: title });
		await expect(privateCard).toBeVisible({ timeout: 10000 });
		await privateCard.click();
		await privateCard.getByRole('button', { name: `Share ${title}` }).click();

		const dlg = page.getByRole('dialog', { name: 'Share this idea?' });
		await expect(dlg).toBeVisible();
		await expect(dlg).toContainText('Suggested by you');
		await dlg.getByRole('button', { name: 'Share' }).click();

		await expect(page.locator('.private-list .card', { hasText: title })).toHaveCount(0);
		const suggestedCard = page.locator('.suggested-list .card', { hasText: title });
		await expect(suggestedCard).toBeVisible({ timeout: 10000 });
		await expect(suggestedCard.getByText(`Suggested by ${BOB.split(':')[1]}`)).toBeVisible();

		// The owner still never sees the shared idea on their My List.
		await login(page, ALICE);
		await page.goto('/');
		await page.reload();
		await expect(page.getByText(title)).toHaveCount(0);
	});

	test('a connection can claim a shared suggestion', async ({ page, request }) => {
		const title = `E2E Shared Claim ${Date.now()}`;
		const id = await savePrivateIdea(request, BOB, aliceId, title);
		await sharePrivateIdea(request, BOB, aliceId, id);

		await login(page, CAROL);
		await page.goto(`/#/list/${aliceId}`);
		const card = page.locator('.suggested-list .card', { hasText: title });
		await expect(card).toBeVisible({ timeout: 10000 });
		await card.getByRole('button', { name: 'Claim', exact: true }).click();
		await expect(card.getByRole('button', { name: 'Unclaim' })).toBeVisible();
	});

	test('the author can retract a shared suggestion they no longer want', async ({ page, request }) => {
		const title = `E2E Retract ${Date.now()}`;
		const id = await savePrivateIdea(request, BOB, aliceId, title);
		await sharePrivateIdea(request, BOB, aliceId, id);

		await login(page, BOB);
		await page.goto(`/#/list/${aliceId}`);
		const card = page.locator('.suggested-list .card', { hasText: title });
		await expect(card).toBeVisible({ timeout: 10000 });
		await card.click();
		await card.getByRole('button', { name: `Unshare ${title}` }).click();
		const dlg = page.getByRole('dialog', { name: 'Unshare this idea?' });
		await expect(dlg).toBeVisible();
		await dlg.getByRole('button', { name: 'Unshare' }).click();
		await expect(page.locator('.card', { hasText: title })).toHaveCount(0);
	});

	test('an unconnected user cannot save a private idea', async ({ request }) => {
		await request.get('/api/me', { headers: auth('eve:Eve Elderberry') });
		const res = await request.post(`/api/users/${aliceId}/private-items`, {
			headers: auth('eve:Eve Elderberry'),
			data: { title: 'Sneaky', url: '' },
		});
		expect(res.status()).toBe(403);
	});

	test('the owner cannot save a private idea for themselves', async ({ request }) => {
		const res = await request.post(`/api/users/${aliceId}/private-items`, {
			headers: auth(ALICE),
			data: { title: 'For me', url: '' },
		});
		expect(res.status()).toBe(400);
	});
});

test.describe('private ideas API', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('private ideas are not exposed via the public list API', async ({ request }) => {
		const title = `E2E API Secret ${Date.now()}`;
		const id = await savePrivateIdea(request, BOB, aliceId, title);

		// The author sees it only in privateItems.
		const bobList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(BOB) })).json();
		expect(bobList.privateItems.some((i: { id: string }) => i.id === id)).toBeTruthy();
		expect(bobList.items.map((i: { title: string }) => i.title)).not.toContain(title);

		// The owner sees neither the private item nor the privateItems field.
		const ownerList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(ALICE) })).json();
		expect(ownerList.privateItems ?? []).toHaveLength(0);
		expect(ownerList.items.map((i: { title: string }) => i.title)).not.toContain(title);

		// Another connection sees neither.
		const carolList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(CAROL) })).json();
		expect(carolList.privateItems ?? []).toHaveLength(0);
		expect(carolList.items.map((i: { title: string }) => i.title)).not.toContain(title);
	});
});