<script lang="ts">
	import { api } from '../lib/api.js';
	import type { ArchiveDetailResponse } from '../lib/types.js';
	import PageHeader from '../lib/PageHeader.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import ItemTile from '../lib/ItemTile.svelte';
	import { isOfflineError } from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';

	let { listId }: { listId: string } = $props();

	let data = $state<ArchiveDetailResponse | null>(null);
	let error = $state('');
	let loading = $state(true);

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			data = await api<ArchiveDetailResponse>(`/api/archive/${listId}`);
			if (navigator.onLine) network.offline = false;
		} catch (err) {
			if (isOfflineError(err) || !navigator.onLine) network.offline = true;
			else error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}
	load();

	let received = $derived(data ? data.items.filter((i) => i.claimed && !i.removedAt) : []);
</script>

<PageHeader owner={data?.label ?? null} accent={"Archive"} />
<a href="#/archive" class="muted">← Archive</a>
{#if network.offline}
	<OfflineState onRetry={() => void load()} />
{:else if error}
	<ErrorState message={error} onRetry={() => void load()} />
{/if}

<p class="muted">Read-only view of what was received and by who.</p>

{#each received as item (item.id)}
	<div class="card">
		<div class="row1">
			<ItemTile title={item.title} url={item.url} />
			<div class="txt">
				<div class="spread">
					<h3>{item.title}</h3>
					<span class="badge">from {item.claimedByName ?? '?'}</span>
				</div>
				{#if item.url}<a class="domain" href={item.url} target="_blank" rel="noreferrer">{new URL(item.url).host} ↗</a>{/if}
			</div>
		</div>
	</div>
{/each}
<EmptyState
	icon="gift"
	title="No claimed items on this list"
	hint="Nothing was claimed before this list wrapped up."
	ready={!loading && !error && !network.offline && data !== null && received.length === 0}
/>
