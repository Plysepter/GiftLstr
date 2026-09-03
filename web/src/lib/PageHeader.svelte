<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import ThemeToggle from './ThemeToggle.svelte';
	import { season } from './theme.svelte.js';

	let {
		owner = null,
		title,
		accent = 'Wishlist',
		children
	}: {
		owner?: string | null;
		title?: string;
		accent?: string | null;
		children?: Snippet;
	} = $props();

	const HATS: Record<string, string> = {
		christmas:
			'<svg class="hat" viewBox="0 0 40 32"><path d="M5 26C7 12 19 4 29 6c3 .6 5 2.6 4.5 5L34 24z" fill="#E11D48"/><rect x="3" y="24" width="34" height="6" rx="3" fill="#fff"/><circle cx="33" cy="8" r="4.5" fill="#fff"/></svg>',
		birthday:
			'<svg class="hat" viewBox="0 0 40 32"><path d="M20 2L34 28H6z" fill="#EC4899"/><path d="M14.5 17.5l11-1M12 23h16" stroke="#fff" stroke-width="2.4"/><circle cx="20" cy="3.4" r="3.2" fill="#FBBF24"/></svg>'
	};

	let first = $derived(owner ? owner.slice(0, 1) : '');
	let rest = $derived(owner ? owner.slice(1) : '');
</script>

<header class="pagehead">
	<div class="headbtns">
		<ThemeToggle />
	</div>
	<div class="titlerow">
		<span class="gift"><Icon name="gift" size={30} /></span>
		<h1>
			{#if owner}
				<span data-owner>
					{#if season.kind !== 'none'}
						<span class="fl">{first}{@html HATS[season.kind]}</span>{rest}
					{:else}{owner}{/if}
				</span>’s
				{#if accent}<span class="accent">{accent}</span>{/if}
			{:else}
				<span class="accent">{title}</span>
			{/if}
		</h1>
	</div>
	{@render children?.()}
</header>
