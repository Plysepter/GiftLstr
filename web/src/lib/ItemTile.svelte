<script lang="ts">
	import { getMetadata } from './metadata.js';

	let { title, url = '' }: { title: string; url?: string } = $props();

	let ogImage = $state('');
	let ogFailed = $state(false);

	$effect(() => {
		const target = url;
		ogImage = '';
		ogFailed = false;
		if (!target) return;
		void getMetadata(target).then((m) => {
			if (target !== url) return;
			ogImage = m?.image ?? '';
		});
	});

	const showOg = $derived(Boolean(ogImage && !ogFailed));
</script>

<span class="tile" class:img={showOg}>
	{#if showOg}
		<img class="fill" src={ogImage} alt="" loading="lazy" onerror={() => (ogFailed = true)} />
	{:else}
		{title.slice(0, 1).toUpperCase()}
	{/if}
</span>

<style>
	.tile {
		position: relative;
	}
	.fill {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		border-radius: inherit;
	}
</style>
