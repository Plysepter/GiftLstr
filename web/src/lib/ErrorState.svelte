<script lang="ts">
	import Icon from './Icon.svelte';

	let {
		message = '',
		onRetry
	}: { message?: string; onRetry?: () => void } = $props();
</script>

<div class="empty error-state" role="alert" data-testid="error-state">
	<div class="art" aria-hidden="true">
		<span class="ring"></span>
		<span class="halo" style="background: radial-gradient(circle, rgba(239,68,68,.18), transparent 70%)"></span>
		<span class="main" style="background: #EF4444"><Icon name="alert" size={30} /></span>
	</div>
	<p class="t">Couldn’t load</p>
	<p class="h">{message || 'Something went wrong. Please try again.'}</p>
	{#if onRetry}
		<div style="display:flex; justify-content:center; margin-top:.9rem">
			<button class="secondary" type="button" onclick={onRetry}>Try again</button>
		</div>
	{/if}
</div>

<style>
	.error-state .ring {
		border-color: rgba(239, 68, 68, 0.35);
	}
</style>
