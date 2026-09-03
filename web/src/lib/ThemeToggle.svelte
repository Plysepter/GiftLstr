<script lang="ts">
	import { theme, setTheme } from './theme.svelte.js';
	import Icon from './Icon.svelte';

	function toggle(): void {
		const btn = document.getElementById('themeToggle');
		btn?.classList.remove('spin');
		void btn?.offsetWidth;
		btn?.classList.add('spin');
		setTheme(theme.choice === 'dark' ? 'light' : 'dark');
	}

	const resolved = $derived(
		theme.choice === 'system'
			? matchMedia('(prefers-color-scheme: dark)').matches
				? 'dark'
				: 'light'
			: theme.choice
	);
</script>

<button
	id="themeToggle"
	type="button"
	onclick={toggle}
	aria-label="Switch to {(resolved === 'dark' ? 'light' : 'dark') + ' mode'}"
>
	<span class="sun"><Icon name="sun" /></span>
	<span class="moon"><Icon name="moon" /></span>
</button>

<style>
	.sun,
	.moon {
		display: inline-flex;
	}
	:global([data-theme='dark']) .sun {
		display: none;
	}
	:global([data-theme='dark']) .moon {
		display: inline-flex;
	}
</style>
