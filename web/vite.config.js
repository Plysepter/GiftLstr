import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { readdir, readFile, writeFile } from 'node:fs/promises';

// After the production build, stamp the emitted asset list into the service
// worker so every hashed chunk (incl. lazy-loaded route chunks) is precached
// and available offline. Also version the cache so old hashed assets are pruned.
function injectServiceWorkerManifest() {
	return {
		name: 'giftlstr-sw-manifest',
		apply: 'build',
		async closeBundle() {
			const assetsDir = new URL('./dist/assets/', import.meta.url);
			const swPath = new URL('./dist/sw.js', import.meta.url);
			try {
				const files = await readdir(assetsDir);
				const assets = files.filter((f) => !f.endsWith('.map')).map((f) => '/assets/' + f);
				const sw = await readFile(swPath, 'utf8');
				const buildId = new Date().toISOString().slice(0, 19).replace(/\D/g, '');
				const out = sw
					.replace('const PRECACHE = [];', `const PRECACHE = ${JSON.stringify(assets)};`)
					.replace(/giftlstr-v1/, `giftlstr-${buildId}`);
				await writeFile(swPath, out);
				console.log(`[sw] precaching ${assets.length} assets (cache giftlstr-${buildId})`);
			} catch (err) {
				console.error('[sw] failed to inject asset manifest:', err);
			}
		}
	};
}

export default defineConfig({
	plugins: [svelte(), injectServiceWorkerManifest()],
	// Only Safari < 18 requires the -webkit- backdrop-filter prefix. Target
	// modern browsers so lightningcss stops emitting it (it breaks in Firefox).
	css: {
		lightningcss: {
			targets: {
				chrome: 150,
				firefox: 154,
				safari: 26,
				ios: 26
			}
		}
	},
	build: {
		cssTarget: ['chrome150', 'firefox154', 'safari26', 'ios26'],
		chunkSizeWarningLimit: 900
	},
	// Vitest runs the transform in SSR mode; direct svelte resolution to the
	// browser build so @testing-library/svelte can mount components. Leave the
	// conditions unset for normal builds so Vite's defaults apply.
	...(process.env.VITEST ? { resolve: { conditions: ['browser'] } } : {}),
	server: {
		proxy: {
			'/api': 'http://localhost:8080'
		}
	},
	test: {
		environment: 'jsdom',
		setupFiles: ['./src/lib/test-setup.ts']
	}
});
