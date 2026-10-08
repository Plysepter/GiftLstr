<script lang="ts">
	import { api } from '../lib/api.js';
	import type { ArchiveDetailResponse } from '../lib/types.js';
	import PageHeader from '../lib/PageHeader.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import ItemTile from '../lib/ItemTile.svelte';
	import LinkPreview from '../lib/LinkPreview.svelte';
	import Icon from '../lib/Icon.svelte';
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
	// Initial load, and a fresh fetch whenever connectivity returns.
	$effect(() => {
		void network.synced;
		void load();
	});

	let items = $derived(data ? data.items.filter((i) => !i.removedAt || i.claimed) : []);
	let title = $derived(
		data
			? data.isPrivate
				? [data.firstName, data.lastName].filter(Boolean).join(' ') || data.label
				: data.label
			: '…'
	);
</script>

<PageHeader title={title}>
	<a href="#/archive" class="back-link">
		<Icon name="chevron-left" size={16} strokeWidth={2.2} />
		Archive
	</a>
</PageHeader>

{#if network.offline}
	<OfflineState onRetry={() => void load()} />
{:else if error}
	<ErrorState message={error} onRetry={() => void load()} />
{/if}

{#if data?.isPrivate}
	<p class="muted">{data.label} · read-only view of what rolled over from this private list.</p>
{:else}
	<p class="muted">Read-only view of what was received and by who.</p>
{/if}

<ul class="items">
	{#each items as item (item.id)}
		<li class="itemcard">
			<article class="card">
				<div class="row1">
					<ItemTile title={item.title} url={item.url} />
					<div class="txt">
						<h3>{item.title}</h3>
						{#if item.url}<a class="domain" href={item.url} target="_blank" rel="noreferrer">{new URL(item.url).host} ↗</a>{/if}
					</div>
				</div>
				<div class="body">
					{#if item.url}<LinkPreview url={item.url} />{/if}
					{#if item.notes}<p class="notes"><span>{item.notes}</span></p>{/if}
					<div class="badges">
						{#if item.claimed && item.claimedByName}
							<span class="badge claimed">from {item.claimedByName}</span>
						{/if}
						{#if item.removedAt}
							<span class="badge warn">removed by owner</span>
						{/if}
					</div>
				</div>
			</article>
		</li>
	{/each}
</ul>

{#if data?.isPrivate}
	<EmptyState
		icon="gift"
		title="Nothing was on this list"
		hint="This private list wrapped up with no ideas left on it."
		ready={!loading && !error && !network.offline && items.length === 0}
	/>
{:else}
	<EmptyState
		icon="gift"
		title="Nothing received from this list"
		hint="No items were claimed before this list wrapped up."
		ready={!loading && !error && !network.offline && data !== null && items.length === 0}
	/>
{/if}

<style>
	.back-link {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		min-height: 44px;
		min-width: 44px;
		padding: 0.5rem 0.75rem;
		margin: 0.15rem 0 0 -0.75rem;
		color: var(--muted);
		text-decoration: none;
		font-size: 0.9rem;
		font-weight: 600;
		border-radius: 10px;
		border: 1px solid transparent;
		transition:
			color 0.15s ease,
			background 0.15s ease,
			border-color 0.15s ease;
	}
	.back-link:hover {
		color: var(--acc-ink);
		background: color-mix(in srgb, var(--acc) 12%, var(--surface));
		border-color: color-mix(in srgb, var(--acc) 30%, var(--surface));
	}
	.back-link:active {
		transform: scale(0.98);
	}

	.items {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.notes span {
		color: var(--muted);
		font-size: 0.88rem;
		line-height: 1.5;
	}
	.badges {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin-top: 0.65rem;
	}
</style>