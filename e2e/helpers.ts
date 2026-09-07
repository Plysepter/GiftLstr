import type { Page } from '@playwright/test';

export const ALICE = 'alice:Alice Apple';
export const BOB = 'bob:Bob Banana';
export const CAROL = 'carol:Carol Cherry';
export const ADMIN = 'admin:Ada Root';

// Mock users to exercise pagination (lists view is 20 per page). 30 mock users
// plus the 4 core users (34 total) ensures at least 2 pages for any viewer.
export const MOCK_USERS: readonly string[] = Array.from({ length: 30 }, (_, i) => {
	const n = String(i + 1).padStart(2, '0');
	return `mock${n}:Mock User ${n}`;
});

/**
 * Signs the browser in as the given e2e user. The token format is
 * `e2e:<username>:<display name>` and is understood by the server when
 * E2E_TEST_MODE=1; the frontend picks it up from localStorage and skips the
 * identity provider.
 */
export async function login(page: Page, who: string): Promise<void> {
	await page.addInitScript(
		([token]) => {
			localStorage.setItem('e2e_token', token!);
		},
		[`e2e:${who}`],
	);
}

/** Authenticated API helper for server-layer assertions. */
export function apiToken(who: string): string {
	return `e2e:${who}`;
}
