<script lang="ts">
	import { onMount } from 'svelte';
	import { openSignIn } from '../lib/auth.js';

	let error = '';

	function onError(event: Event): void {
		error = (event as CustomEvent<string | null>).detail ?? '';
	}

	onMount(() => {
		window.addEventListener('giftlstr-signin-error', onError);
		return () => window.removeEventListener('giftlstr-signin-error', onError);
	});
</script>

<div class="card" style="max-width: 380px; margin: 3rem auto; text-align: center;">
	<h1><span class="accent">GiftLstr</span></h1>
	<p class="muted">Sign in to view and manage gift lists.</p>
	{#if error}
		<p class="err" role="alert">{error}</p>
	{/if}
	<button
		class="login-btn"
		onclick={() => {
			error = '';
			openSignIn();
		}}
	>
		Sign in
	</button>
</div>

<style>
	.login-btn {
		display: inline-block;
		margin: 0 auto;
	}
	.err {
		color: var(--c-del);
		margin: 0.75rem 0;
	}
</style>