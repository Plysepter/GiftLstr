import { expect, test } from '@playwright/test';
import { ALICE, BOB, apiToken, login } from './helpers.js';

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });
let aliceId = 0;

test.beforeAll(async ({ request }) => {
	const admin = await (
		await request.get('/api/admin/users', { headers: auth('admin:Ada Root') })
	).json();
	aliceId = admin.users.find((u) => u.username === 'alice').id;
	// Give alice a Jan 1 birthday reset date and connect bob to her.
	await request.patch(`/api/admin/users/${aliceId}`, {
		headers: auth('admin:Ada Root'),
		data: { birthday: '01-01' },
	});
	const me = await (await request.get('/api/me', { headers: auth(BOB) })).json();
	await request.put('/api/connections', {
		headers: auth('admin:Ada Root'),
		data: { a: me.user.id, b: aliceId, add: true },
	});
});

test.describe('archive / reset flow', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('reset archives the list, copies unclaimed items, and records claimers', async ({
		page,
		request,
	}) => {
		const stamp = Date.now();
		const keep = await request.post('/api/my/items', {
			headers: auth(ALICE),
			data: { title: `E2E Keep ${stamp}` },
		});
		const gifted = await request.post('/api/my/items', {
			headers: auth(ALICE),
			data: { title: `E2E Gifted ${stamp}` },
		});
		const giftedId = (await gifted.json()).id;
		expect((await keep).ok()).toBeTruthy();

		// Bob claims one item before the reset.
		expect(
			(await request.put(`/api/items/${giftedId}/claim`, { headers: auth(BOB) })).ok(),
		).toBeTruthy();

		// Force the archive as of just before New Year: the birthday reset
		// (Jan 1) is in the past, but this year's Christmas is only days away,
		// so the birthday reset is the one that triggers.
		const year = new Date().getUTCFullYear();
		const res = await request.post('/api/e2e/archive', {
			headers: auth(ALICE),
			data: { date: `${year}-12-30` },
		});
		expect(res.ok()).toBeTruthy();

		// Archive index shows the new entry.
		await login(page, ALICE);
		await page.goto('/#/archive');
		const row = page.locator('.card', { hasText: `${year} Birthday` });
		await expect(row).toBeVisible();

		// Detail view is read-only and shows who claimed what.
		await row.getByRole('button', { name: 'View' }).click();
		await expect(page.locator('.card', { hasText: `E2E Gifted ${stamp}` })).toContainText(
			'from Bob Banana',
		);
		await expect(
			page.locator('.card', { hasText: `E2E Gifted ${stamp}` }).getByRole('button'),
		).toHaveCount(0);

		// Unclaimed item was copied to the new active list; claimed one was not.
		const active = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(ALICE) })).json();
		const titles = active.items.map((i) => i.title);
		expect(titles).toContain(`E2E Keep ${stamp}`);
		expect(titles).not.toContain(`E2E Gifted ${stamp}`);
	});

	test('another user cannot read my archive (404)', async ({ request }) => {
		const index = await (await request.get('/api/archive', { headers: auth(ALICE) })).json();
		if (index.lists.length === 0) return;
		const res = await request.get(`/api/archive/${index.lists[0].id}`, { headers: auth(BOB) });
		expect([403, 404]).toContain(res.status());
	});

	test('archive reveals claimed items from private and shared lists; unclaimed ones stay hidden', async ({
		request,
	}) => {
		const stamp = Date.now();
		const claimedPrivate = `E2E Arc Claimed Priv ${stamp}`;
		const unclaimedPrivate = `E2E Arc Private Unclaimed ${stamp}`;
		const claimedShared = `E2E Arc Shared Claimed ${stamp}`;
		const unclaimedShared = `E2E Arc Shared Unclaimed ${stamp}`;

		const privA = await savePrivate(request, BOB, aliceId, claimedPrivate);
		await savePrivate(request, BOB, aliceId, unclaimedPrivate);
		const privC = await savePrivate(request, BOB, aliceId, claimedShared);
		const privD = await savePrivate(request, BOB, aliceId, unclaimedShared);
		await request.put(`/api/users/${aliceId}/private-items/${privA}/share`, { headers: auth(BOB) });
		await request.put(`/api/users/${aliceId}/private-items/${privD}/share`, { headers: auth(BOB) });

		// Bob claims one private idea and one shared idea before the reset.
		expect((await request.put(`/api/items/${privA}/claim`, { headers: auth(BOB) })).ok()).toBeTruthy();
		expect((await request.put(`/api/items/${privC}/claim`, { headers: auth(BOB) })).ok()).toBeTruthy();

		const year = new Date().getUTCFullYear();
		expect((await request.post('/api/e2e/archive', { headers: auth(ALICE), data: { date: `${year}-12-30` } })).ok()).toBeTruthy();

		// The owner's archive reveals their public + claimed (private or
		// shared) items; unclaimed private and unclaimed shared stay hidden.
		const archiveIndex = await (await request.get('/api/archive', { headers: auth(ALICE) })).json();
		const first = archiveIndex.lists[0];
		const archive = await (await request.get(`/api/archive/${first.id}`, { headers: auth(ALICE) })).json();
		const archiveTitles = archive.items.map((i: { title: string }) => i.title);
		expect(archiveTitles).toContain(claimedPrivate);
		expect(archiveTitles).toContain(claimedShared);
		expect(archiveTitles).not.toContain(unclaimedPrivate);
		expect(archiveTitles).not.toContain(unclaimedShared);

		// On the new active list: claimed items are revealed as public, the
		// unclaimed private idea rolls over privately to its author, and the
		// unclaimed shared idea is dropped.
		const bobList = await (await request.get(`/api/users/${aliceId}/list`, { headers: auth(BOB) })).json();
		const itemTitles = bobList.items.map((i: { title: string }) => i.title);
		const privateTitles = bobList.privateItems.map((i: { title: string }) => i.title);
		expect(itemTitles).toContain(claimedPrivate);
		expect(itemTitles).toContain(claimedShared);
		expect(itemTitles).not.toContain(unclaimedShared);
		expect(privateTitles).toContain(unclaimedPrivate);
		expect(privateTitles).not.toContain(unclaimedShared);
	});

	test('private lists roll over into the private archive tab', async ({ page, request }) => {
		const stamp = Date.now();
		const person = `Robin Reed${stamp}`;

		// A private list for a person whose birthday has already passed.
		const created = await request.post('/api/private-lists', {
			headers: auth(ALICE),
			data: { firstName: 'Robin', lastName: `Reed${stamp}`, birthday: '01-05' },
		});
		expect(created.ok()).toBeTruthy();
		const listId = (await created.json()).id;

		const keep = await request.post('/api/my/items', {
			headers: auth(ALICE),
			data: { title: `E2E Priv Keep ${stamp}`, listId },
		});
		const bought = await request.post('/api/my/items', {
			headers: auth(ALICE),
			data: { title: `E2E Priv Bought ${stamp}`, listId },
		});
		expect((await keep).ok()).toBeTruthy();
		const boughtId = (await bought.json()).id;
		expect((await request.put(`/api/items/${boughtId}/claim`, { headers: auth(ALICE) })).ok()).toBeTruthy();

		// Force the reset as of the end of the year: the list's Jan 5 birthday
		// is long past, so it rolls over.
		const year = new Date().getUTCFullYear();
		const res = await request.post('/api/e2e/archive', {
			headers: auth(ALICE),
			data: { date: `${year}-12-30` },
		});
		expect(res.ok()).toBeTruthy();

		// The private tab lists it under the person's name, and the detail view
		// reveals every idea on the rolled-over list.
		await login(page, ALICE);
		await page.goto('/#/archive');
		await page.getByRole('tab', { name: 'Private lists' }).click();
		const row = page.locator('.card', { hasText: person });
		await expect(row).toBeVisible();
		await row.getByRole('button', { name: 'View' }).click();
		await expect(page.locator('.card', { hasText: `E2E Priv Keep ${stamp}` })).toBeVisible();
		await expect(page.locator('.card', { hasText: `E2E Priv Bought ${stamp}` })).toBeVisible();

		// Drop the fresh private list so later specs still see Alice's seeded
		// connection-only totals.
		const lists = await (await request.get('/api/lists?page=1', { headers: auth(ALICE) })).json();
		const fresh = lists.users.find((u: { isPrivate?: boolean; displayName: string }) => u.isPrivate && u.displayName === person);
		if (fresh) await request.delete(`/api/private-lists/${fresh.id}`, { headers: auth(ALICE) });
	});

	test('a private list can be archived on demand from its page', async ({ page, request }) => {
		const stamp = Date.now();
		const person = `Robin Reed${stamp}`;

		const created = await request.post('/api/private-lists', {
			headers: auth(ALICE),
			data: { firstName: 'Robin', lastName: `Reed${stamp}`, birthday: '01-05' },
		});
		expect(created.ok()).toBeTruthy();
		const listId = (await created.json()).id;

		await request.post('/api/my/items', {
			headers: auth(ALICE),
			data: { title: `E2E Manual Keep ${stamp}`, listId },
		});
		const bought = await request.post('/api/my/items', {
			headers: auth(ALICE),
			data: { title: `E2E Manual Bought ${stamp}`, listId },
		});
		const boughtId = (await bought.json()).id;
		expect((await request.put(`/api/items/${boughtId}/claim`, { headers: auth(ALICE) })).ok()).toBeTruthy();

		await login(page, ALICE);
		await page.goto(`/#/private-list/${listId}`);
		await page.getByRole('button', { name: 'Archive Robin' }).click();
		await page.getByRole('button', { name: 'Archive list' }).click();

		// The page opens the fresh replacement, carrying only the unclaimed idea.
		await expect(page.locator('.card', { hasText: `E2E Manual Keep ${stamp}` })).toBeVisible();
		await expect(page.locator('.card', { hasText: `E2E Manual Bought ${stamp}` })).toHaveCount(0);

		// The archived copy lands in the private archive tab under the manual label.
		const index = await (await request.get('/api/archive?type=private', { headers: auth(ALICE) })).json();
		const archived = index.lists.find((l: { id: string }) => l.id === listId);
		expect(archived?.label).toBe(`${person} Manual Archive`);

		// Drop the fresh list so later specs still see Alice's seeded totals.
		const lists = await (await request.get('/api/lists?page=1', { headers: auth(ALICE) })).json();
		const fresh = lists.users.find((u: { isPrivate?: boolean; displayName: string }) => u.isPrivate && u.displayName === person);
		if (fresh) await request.delete(`/api/private-lists/${fresh.id}`, { headers: auth(ALICE) });
	});
});

async function savePrivate(request: import('@playwright/test').APIRequestContext, token: string, ownerId: string, title: string): Promise<string> {
	const res = await request.post(`/api/users/${ownerId}/private-items`, {
		headers: auth(token),
		data: { title, url: '' },
	});
	expect(res.ok()).toBeTruthy();
	return (await res.json()).id;
}

test.describe('lists page pagination', () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test('lists page paginates and shows users', async ({ page }) => {
		await login(page, ALICE);
		await page.goto('/#/lists');
		await expect(page.getByRole('heading', { name: 'Lists' })).toBeVisible();
		await expect(page.locator('.card').first()).toBeVisible();
	});
});
