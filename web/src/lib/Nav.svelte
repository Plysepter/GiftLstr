<script lang="ts">
	import { route } from './router.js';
	import { user } from './auth.js';
	import type { User } from './types.js';
	import Icon from './Icon.svelte';

	const tabs: ReadonlyArray<{ path: string; label: string; icon: string }> = [
		{ path: '/', label: 'My List', icon: 'gift' },
		{ path: '/lists', label: 'Lists', icon: 'list' },
		{ path: '/archive', label: 'Archive', icon: 'archive' },
		{ path: '/profile', label: 'Profile', icon: 'user' }
	];

	let current = $derived($route);
	let isActive = $derived((p: string): boolean => {
		if (p === '/') return current === '/';
		if (p === '/lists') return current === '/lists' || current.startsWith('/list/');
		return current.startsWith(p);
	});
	let u = $derived<User | null>($user);
</script>

<nav class="tabbar" aria-label="Main navigation">
	{#each tabs as t (t.path)}
		<a href={'#' + t.path} class:active={isActive(t.path)} aria-current={isActive(t.path) ? 'page' : undefined}>
			<Icon name={t.icon} size={21} />
			<span class="lbl">{t.label}</span>
		</a>
	{/each}
	{#if u?.isAdmin}
		<a href="#/admin" class:active={isActive('/admin')} aria-current={isActive('/admin') ? 'page' : undefined}>
			<Icon name="shield" size={21} />
			<span class="lbl">Admin</span>
		</a>
	{/if}
</nav>
