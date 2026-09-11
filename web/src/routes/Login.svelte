<script lang="ts">
	import { onMount } from 'svelte';
	import { openSignIn } from '../lib/auth.js';
	import { authE2e } from '../lib/providers.js';

	interface DemoUser {
		username: string;
		displayName: string;
		isAdmin?: boolean;
	}

	let error = '';
	/** Demo accounts offered in e2e mode; null until loaded. */
	let demoUsers: DemoUser[] | null = null;

	function onError(event: Event): void {
		error = (event as CustomEvent<string | null>).detail ?? '';
	}

	async function loadDemoUsers(): Promise<void> {
		try {
			const res = await fetch('/api/e2e/demo-users');
			if (!res.ok) {
				demoUsers = [];
				return;
			}
			const data = await res.json();
			demoUsers = data.users ?? [];
		} catch {
			demoUsers = [];
		}
	}

	function useDemoAccount(u: DemoUser): void {
		localStorage.setItem('e2e_token', `e2e:${u.username}:${u.displayName}`);
		window.location.reload();
	}

	onMount(() => {
		window.addEventListener('giftlstr-signin-error', onError);
		if (authE2e) void loadDemoUsers();
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

{#if authE2e}
	<section class="card demo" aria-label="Demo accounts" style="max-width: 380px; margin: 0 auto 3rem; text-align: center;">
		<h2>Demo accounts</h2>
		<p class="muted">No identity provider here. Jump straight into a seeded account.</p>
		{#if demoUsers === null}
			<p class="muted">Loading accounts…</p>
		{:else if demoUsers.length > 0}
			<ul class="demo-list">
				{#each demoUsers as u (u.username)}
					<li>
						<button class="demo-btn" type="button" onclick={() => useDemoAccount(u)}>
							{u.displayName}{u.isAdmin ? ' · admin' : ''}
						</button>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="muted">No demo accounts yet.</p>
		{/if}
	</section>
{/if}

<style>
	.login-btn {
		display: inline-block;
		margin: 0 auto;
	}
	.err {
		color: var(--c-del);
		margin: 0.75rem 0;
	}
	.demo-list {
		list-style: none;
		padding: 0;
		margin: 1rem 0 0;
		display: grid;
		gap: 0.5rem;
	}
	.demo-btn {
		width: 100%;
	}
</style>