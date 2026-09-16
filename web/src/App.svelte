<script lang="ts">
	import { route, parseRoute } from './lib/router.js';
	import { user, loaded } from './lib/auth.js';
			import { computeSeason, prefs, ALL_STYLES } from './lib/theme.svelte.js';
	import type { ThemeStyle } from './lib/theme.svelte.js';
	import LazyRoute from './lib/LazyRoute.svelte';
	import Loader from './lib/Loader.svelte';
	import Nav from './lib/Nav.svelte';
	import BirthdayModal from './lib/BirthdayModal.svelte';
	import Weather from './lib/Weather.svelte';
	import BgFx from './lib/BgFx.svelte';
	import BgArt from './lib/BgArt.svelte';

	let parsed = $derived(parseRoute($route));

	let hasSyncedTheme = false;
	// Recompute event theming when the user (birthday) or day changes.
	// When viewing another user's list, UserList owns the season/theme — don't clobber it.
	// $effect.pre: dataset must be set before the render that follows user/route changes,
	// otherwise a theme flip can paint a frame later ("background pops").
	$effect.pre(() => {
		const u = $user;
		const r = $route;
		if (parseRoute(r).name === 'userList') return;
		computeSeason(u?.birthday ?? '01-01', u?.holidayReset ?? '');
	});
	// Once on initial load, bring local theme in line with server's persisted choice.
	$effect.pre(() => {
		const u = $user;
		if (hasSyncedTheme || !u) return;
		const raw = (u as unknown as { themeStyle?: string })?.themeStyle;
		const serverStyle = raw;
		if (serverStyle && ALL_STYLES.includes(serverStyle as ThemeStyle) && serverStyle !== prefs.style) {
			prefs.style = serverStyle as ThemeStyle;
			try {
				localStorage.setItem('theme-style', serverStyle);
			} catch {}
			document.documentElement.dataset.style = serverStyle;
		}
		hasSyncedTheme = true;
	});

	// Docked composer / tab-bar duck under the keyboard.
	$effect(() => {
		const vv = window.visualViewport;
		if (!vv) return;
		const upd = (): void => {
			// During iOS rubber-band overscroll the visual viewport is panned
			// beyond the layout viewport; ignore those transient offsets or the
			// docked composer would jump off the tab bar. Only the keyboard
			// remains inside the layout viewport, so this still tracks it.
			if (vv.offsetTop < 0 || vv.offsetTop + vv.height > window.innerHeight) return;
			const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
			document.documentElement.style.setProperty('--kb', kb + 'px');
			document.documentElement.classList.toggle('kb', kb > 80);
		};
		vv.addEventListener('resize', upd);
		vv.addEventListener('scroll', upd);
		upd();
		return () => {
			vv.removeEventListener('resize', upd);
			vv.removeEventListener('scroll', upd);
		};
	});
</script>

<BgFx />
<BgArt />
<Weather />
{#if !$loaded}
			<Loader minHeight="100dvh" />
		{:else if !$user}
			<LazyRoute load={() => import('./routes/Login.svelte')} />
		{:else}
			{#if parsed.name === 'admin' || parsed.name === 'profile'}
				<div class="page-scrim" aria-hidden="true"></div>
			{/if}
			{#key $route}
				<div class="page">
					{#if parsed.name === 'myList'}
						<LazyRoute load={() => import('./routes/MyList.svelte')} />
					{:else if parsed.name === 'userList'}
						<LazyRoute load={() => import('./routes/UserList.svelte')} props={{ userId: parsed.id ?? '' }} />
					{:else if parsed.name === 'lists'}
						<LazyRoute load={() => import('./routes/ListsPage.svelte')} />
					{:else if parsed.name === 'archive'}
						<LazyRoute load={() => import('./routes/ArchivePage.svelte')} />
					{:else if parsed.name === 'archiveDetail'}
						<LazyRoute load={() => import('./routes/ArchiveDetail.svelte')} props={{ listId: parsed.id ?? '' }} />
					{:else if parsed.name === 'profile'}
						<LazyRoute load={() => import('./routes/Profile.svelte')} />
					{:else if parsed.name === 'admin'}
						<LazyRoute load={() => import('./routes/Admin.svelte')} />
					{:else}
						<h1>Not found</h1>
					{/if}
				</div>
			{/key}
			<Nav />
			<BirthdayModal />
		{/if}
