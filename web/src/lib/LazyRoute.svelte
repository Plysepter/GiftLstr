<script lang="ts">
	import { mount, onMount, unmount, type Component } from 'svelte';

	interface Props {
		/** Returns the page component module once loaded. */
		load: () => Promise<{ default: unknown }>;
		props?: Record<string, unknown>;
	}

	let { load, props = {} }: Props = $props();

	let host = $state<HTMLDivElement>() as HTMLDivElement;
	let child: Record<string, any> | null = null;

	// Runs once per mount: App remounts this via {#key $route}, so a single
	// load/unload cycle is all it ever needs.
	onMount(() => {
		let alive = true;
		load()
			.then((mod) => {
				if (!alive) return;
				child = mount(mod.default as Component, { target: host, props });
			})
			.catch((err) => console.error('failed to lazy-load route', err));
		return () => {
			alive = false;
			if (child) {
				unmount(child);
				child = null;
			}
		};
	});
</script>

<div bind:this={host}></div>