<script lang="ts">
	import { getMetadata, metadataVersion } from './metadata.js';
	import type { PageMetadata } from './types.js';

	let { url }: { url: string } = $props();

	let meta = $state<PageMetadata | null>(null);

	$effect(() => {
		const target = url;
		// Re-run when a preview is invalidated elsewhere (URL string unchanged).
		void $metadataVersion;
		meta = null;
		if (!target) return;
		getMetadata(target).then((m) => {
			if (url === target) meta = m;
		});
	});

	// Image lives in the item tile, title in the header, host in the domain
	// link — the body only adds the page description.
	const desc = $derived(meta?.description.trim() ?? '');
</script>

{#if desc}
	<p class="desc">{desc}</p>
{/if}

<style>
	.desc {
		margin: 0.3rem 0 0.1rem;
		font-size: 0.86rem;
		color: var(--muted);
		line-height: 1.45;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
		overflow-wrap: anywhere;
	}
</style>
