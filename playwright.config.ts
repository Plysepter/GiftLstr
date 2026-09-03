import { defineConfig } from '@playwright/test';

const port = 8080;
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
	testDir: './e2e',
	testMatch: '**/*.spec.ts',
	globalSetup: './e2e/global-setup.ts',
	fullyParallel: false,
	workers: 1,
	reporter: process.env.CI ? 'github' : 'list',
	use: {
		baseURL,
		trace: 'retain-on-failure',
		// Disable the GPU process in headless Chromium. Its shared-image /
		// GPU command-buffer path intermittently segfaults in headless-shell,
		// crashing the browser mid-test; software rendering avoids it.
		launchOptions: {
			args: ['--disable-gpu', '--disable-software-rasterizer'],
		},
	},
	webServer: {
		command: './e2e/e2e-server.sh',
		url: `${baseURL}/api/openapi.yaml`,
		reuseExistingServer: !process.env.CI,
		timeout: 120_000,
		env: {
			DATA_DIR: './e2e/.data',
			E2E_TEST_MODE: '1',
			// No CLERK_SECRET_KEY here: the server refuses to combine it with
			// E2E_TEST_MODE, and e2e mode bypasses Clerk entirely.
		},
	},
});
