<script lang="ts">
	import EmptyState from './EmptyState.svelte';

	let { onRetry }: { onRetry?: () => void } = $props();
</script>

<div role="status" data-testid="offline-state">
	<EmptyState icon="wifi-off" title="You’re offline" hint="Check your connection and try again. New items will sync automatically when you’re back online.">
		{#if onRetry}
			<div style="display:flex; justify-content:center; margin-top:.9rem">
				<button class="secondary" type="button" onclick={onRetry}>Try again</button>
			</div>
		{/if}
	</EmptyState>
</div>
