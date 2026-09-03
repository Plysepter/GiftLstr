<script lang="ts">
	import { getMetadata } from './metadata.js';

	let { title, url = '' }: { title: string; url?: string } = $props();

	let ogImage = $state('');
	let resolved = $state(false);
	let ogFailed = $state(false);
	let favFailed = $state(false);

	$effect(() => {
		const target = url;
		ogImage = '';
		resolved = false;
		ogFailed = false;
		favFailed = false;
		if (!target) return;
		void getMetadata(target).then((m) => {
			if (target !== url) return;
			ogImage = m?.image ?? '';
			resolved = true;
		});
	});

	const host = $derived.by(() => {
		if (!url) return '';
		try {
			return new URL(url).host;
		} catch {
			return '';
		}
	});

	const favicon = $derived(host ? `https://www.google.com/s2/favicons?domain=${host}&sz=64` : '');
	const showOg = $derived(Boolean(ogImage && !ogFailed));
	const showFav = $derived(Boolean(!showOg && host && !favFailed && resolved));
</script>

<span class="tile" class:img={showOg} class:fav={showFav}>
	{#if showOg}
		<img class="fill" src={ogImage} alt="" loading="lazy" onerror={() => (ogFailed = true)} />
	{:else if showFav}
		<img class="fill fav-img" src={favicon} alt="" loading="lazy" onerror={() => (favFailed = true)} />
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
	.fav-img {
		object-fit: contain;
		padding: 7px;
		background: var(--solid);
	}
	.tile.fav {
		background: var(--solid) !important;
		animation: none !important;
		border: 1px solid var(--line);
	}
</style>
