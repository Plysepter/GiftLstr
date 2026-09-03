import { expect, test } from '@playwright/test';
import { ALICE, ADMIN, BOB, apiToken, login } from './helpers.js';

let aliceId = 0;

const auth = (who: string) => ({ Authorization: `Bearer ${apiToken(who)}` });

test.beforeAll(async ({ request }) => {
	// Provision the users and connect bob to alice so he can view her list.
	await request.get('/api/me', { headers: auth(ALICE) });
	const bobMe = await (await request.get('/api/me', { headers: auth(BOB) })).json();
	const admin = await (await request.get('/api/admin/users', { headers: auth(ADMIN) })).json();
	aliceId = admin.users.find((u: { username: string }) => u.username === 'alice').id;
	await request.put('/api/connections', {
		headers: auth(ADMIN),
		data: { a: bobMe.user.id, b: aliceId, add: true }
	});
});

async function addItem(request: import('@playwright/test').APIRequestContext, data: object): Promise<number> {
	const res = await request.post('/api/my/items', { headers: auth(ALICE), data });
	return (await res.json()).id;
}

async function addAndArchive(request: import('@playwright/test').APIRequestContext, title: string): Promise<void> {
	const id = await addItem(request, { title, url: '' });
	await request.delete(`/api/my/items/${id}`, { headers: auth(ALICE) });
}

test.describe('copy list to clipboard', () => {
	test.use({
		permissions: ['clipboard-read', 'clipboard-write'],
		storageState: { cookies: [], origins: [] }
	});

	test('owner copies the full list: URL as title, notes, and spacing', async ({ page, request }) => {
		const stamp = Date.now();
		const url = `https://example.com/kettle-${stamp}`;
		const plain = `Plain ${stamp}`;
		const taken = `Taken ${stamp}`;
		await addItem(request, { title: `Kettle ${stamp}`, url, notes: 'Shiny one' });
		await addAndArchive(request, `Gone ${stamp}`);
		await addItem(request, { title: plain, url: '' });
		const takenId = await addItem(request, { title: taken, url: '' });
		await request.put(`/api/items/${takenId}/claim`, { headers: auth(BOB) });

		await login(page, ALICE);
		await page.goto('/');
		await page.bringToFront();
		const copyBtn = page.getByRole('button', { name: 'Copy list to clipboard' });
		await copyBtn.click();

		const text = await page.evaluate(() => navigator.clipboard.readText());
		expect(text).toMatch(/^Alice Apple's (?:birthday|Christmas )?list:\n\n/);
		// Item title is the URL when there is one, else the name.
		expect(text).toContain(`- ${url}`);
		expect(text).toContain(`- ${plain}`);
		// One blank line between item and notes, two between items.
		expect(text).toContain(`${url}\n\nNotes: Shiny one\n\n\n- ${plain}`);
		expect(text).toContain(`- ${plain}\n\n\n- ${taken}`);
		// The owner keeps claimed items and always drops archived ones.
		expect(text).toContain(`- ${taken}`);
		expect(text).not.toContain(`Gone ${stamp}`);

		await request.delete(`/api/items/${takenId}/claim`, { headers: auth(BOB) });
	});

	test('visitor copies the list without claimed or archived items', async ({ page, request }) => {
		const stamp = Date.now();
		const free = `Free ${stamp}`;
		const taken = `Taken ${stamp}`;
		await addItem(request, { title: free, url: '' });
		await addAndArchive(request, `Archived ${stamp}`);
		const takenId = await addItem(request, { title: taken, url: '' });
		await request.put(`/api/items/${takenId}/claim`, { headers: auth(BOB) });

		try {
			await login(page, BOB);
			await page.goto(`/#/list/${aliceId}`);
			await page.bringToFront();
			const copyBtn = page.getByRole('button', { name: 'Copy list to clipboard' });
			await copyBtn.click();

			const text = await page.evaluate(() => navigator.clipboard.readText());
			expect(text).toMatch(/^Alice Apple's (?:birthday|Christmas )?list:\n\n/);
			expect(text).toContain(`- ${free}`);
			expect(text).not.toContain(`- ${taken}`);
			expect(text).not.toContain(`Archived ${stamp}`);
		} finally {
			await request.delete(`/api/items/${takenId}/claim`, { headers: auth(BOB) });
		}
	});
});