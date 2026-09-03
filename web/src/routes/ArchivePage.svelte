<script lang="ts">
	import { api } from '../lib/api.js';
	import type { ArchiveIndexResponse } from '../lib/types.js';
	import PageHeader from '../lib/PageHeader.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import { isOfflineError } from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';

	let page = $state(1);
	let lists = $state<ArchiveIndexResponse['lists']>([]);
	let hasMore = $state(false);
	let error = $state('');
	let loading = $state(true);

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			const d = await api<ArchiveIndexResponse>(`/api/archive?page=${page}`);
			lists = d.lists;
			hasMore = d.hasMore;
			if (navigator.onLine) network.offline = false;
		} catch (err) {
			if (isOfflineError(err) || !navigator.onLine) network.offline = true;
			else error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}
	$effect(() => {
		void page;
		void load();
	});

	function go(p: number): void {
		page = p;
		void load();
	}
</script>

<PageHeader title={"Archive"} accent={null} />
{#if network.offline}
	<OfflineState onRetry={() => void load()} />
{:else if error}
	<ErrorState message={error} onRetry={() => void load()} />
{/if}
{#each lists as l (l.id)}
	<div class="card spread">
		<div>
			<strong>{l.label}</strong>
			<p class="muted">{l.itemCount} items · {l.claimedCount} claimed</p>
		</div>
		<a href={`#/archive/${l.id}`}><button class="secondary">View</button></a>
	</div>
{/each}
<EmptyState
	icon="archive"
	title="Nothing archived yet"
	hint="A week after each reset date, your list is archived here automatically."
	ready={!loading && !error && !network.offline && lists.length === 0}
/>

{#if hasMore || page > 1}
	<div class="pager">
		<button class="secondary" disabled={page <= 1} onclick={() => go(page - 1)}>Prev</button>
		<span class="muted">Page {page}</span>
		<button class="secondary" disabled={!hasMore} onclick={() => go(page + 1)}>Next</button>
	</div>
{/if}

<style>
	.pager {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 0.65rem;
		margin-top: 1.25rem;
	}
</style>
