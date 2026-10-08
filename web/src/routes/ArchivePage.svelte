<script lang="ts">
	import { api } from '../lib/api.js';
	import type { ArchiveIndexResponse, ArchiveListSummary } from '../lib/types.js';
	import PageHeader from '../lib/PageHeader.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import { isOfflineError } from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';

	type Tab = 'archived' | 'private';

	let tab = $state<Tab>('archived');
	let page = $state(1);
	let lists = $state<ArchiveIndexResponse['lists']>([]);
	let hasMore = $state(false);
	let error = $state('');
	let loading = $state(true);

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			const q = tab === 'private' ? '&type=private' : '';
			const d = await api<ArchiveIndexResponse>(`/api/archive?page=${page}${q}`);
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
		void tab;
		void network.synced;
		void load();
	});

	function selectTab(t: Tab): void {
		if (t === tab) return;
		tab = t;
		page = 1;
	}

	function go(p: number): void {
		page = p;
		void load();
	}

	function personName(l: ArchiveListSummary): string {
		return [l.firstName, l.lastName].filter(Boolean).join(' ') || l.label;
	}
</script>

<PageHeader title={"Archive"} accent={null} />

<div class="tabs" role="tablist" aria-label="Archive sections">
	<button
		class="tab"
		class:active={tab === 'archived'}
		role="tab"
		aria-selected={tab === 'archived'}
		onclick={() => selectTab('archived')}
	>
		Archived
	</button>
	<button
		class="tab"
		class:active={tab === 'private'}
		role="tab"
		aria-selected={tab === 'private'}
		onclick={() => selectTab('private')}
	>
		Private lists
	</button>
</div>

{#if network.offline}
	<OfflineState onRetry={() => void load()} />
{:else if error}
	<ErrorState message={error} onRetry={() => void load()} />
{/if}

{#each lists as l (l.id)}
	<div class="card spread">
		<div>
			<strong>{tab === 'private' ? personName(l) : l.label}</strong>
			<p class="muted">
				{#if tab === 'private'}{l.label} · {/if}{l.itemCount} items · {l.claimedCount} claimed
			</p>
		</div>
		<a href={`#/archive/${l.id}`}><button class="secondary">View</button></a>
	</div>
{/each}

{#if tab === 'archived'}
	<EmptyState
		icon="archive"
		title="Nothing archived yet"
		hint="A week after each reset date, your list is archived here automatically."
		ready={!loading && !error && !network.offline && lists.length === 0}
	/>
{:else}
	<EmptyState
		icon="archive"
		title="No private lists rolled over yet"
		hint="A week after a private list's birthday, it moves here and a fresh list starts."
		ready={!loading && !error && !network.offline && lists.length === 0}
	/>
{/if}

{#if hasMore || page > 1}
	<div class="pager">
		<button class="secondary" disabled={page <= 1} onclick={() => go(page - 1)}>Prev</button>
		<span class="muted">Page {page}</span>
		<button class="secondary" disabled={!hasMore} onclick={() => go(page + 1)}>Next</button>
	</div>
{/if}

<style>
	.tabs {
		display: flex;
		gap: 0.3rem;
		margin: 0.25rem 0 1rem;
		padding: 0.25rem;
		width: fit-content;
		border-radius: 12px;
		background: color-mix(in srgb, var(--solid) 30%, transparent);
	}
	.tab {
		border: 0;
		background: transparent;
		color: var(--muted);
		font: inherit;
		font-weight: 600;
		padding: 0.45rem 0.9rem;
		border-radius: 9px;
		cursor: pointer;
	}
	.tab.active {
		background: var(--surface);
		color: var(--acc-ink);
	}
	.pager {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 0.65rem;
		margin-top: 1.25rem;
	}
</style>
