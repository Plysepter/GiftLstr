<script lang="ts">
	import { api } from '../lib/api.js';
	import { user } from '../lib/auth.js';
	import type { ListsPageResponse } from '../lib/types.js';
	import { formatMoney } from '../lib/money.js';
	import PageHeader from '../lib/PageHeader.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import { isOfflineError } from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';
	import Icon from '../lib/Icon.svelte';

	let page = $state(1);
	let users = $state<ListsPageResponse['users']>([]);
	let hasMore = $state(false);
	let error = $state('');
	let loading = $state(true);

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			const d = await api<ListsPageResponse>(`/api/lists?page=${page}`);
			users = d.users;
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
		void network.synced;
		void load();
	});

	function go(p: number): void {
		page = p;
		void load();
	}

	const spendingOn = $derived($user?.spendingEnabled === true);

	let shareCode = $state('');
	let connectBtnEl: HTMLButtonElement | undefined = $state();
	let connectState = $state<'idle' | 'done' | 'error'>('idle');
	let connectMsg = $state('');

	async function connect(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		const code = shareCode.trim();
		if (!code || connectState === 'done') return;
		connectState = 'idle';
		connectMsg = '';
		try {
			const res = await api<{ displayName?: string }>('/api/connect/accept', {
				method: 'POST',
				body: { code }
			});
			connectState = 'done';
			connectMsg = res.displayName ? `Connected to ${res.displayName}!` : 'Connected!';
			setTimeout(() => (connectState = 'idle'), 1800);
			void load();
		} catch (err) {
			connectState = 'error';
			connectMsg = err instanceof Error ? err.message : 'Could not connect';
			setTimeout(() => (connectState = 'idle'), 2200);
		}
	}
</script>

<div class="lists-page">
	<PageHeader title={"Lists"} accent={null} />
	{#if network.offline}
		<OfflineState onRetry={() => void load()} />
	{:else if error}
		<ErrorState message={error} onRetry={() => void load()} />
	{/if}
	{#each users as u (u.id)}
		<div class="card spread">
			<div class="listinfo">
				<strong>{u.displayName}</strong>
				<span class="meta muted">
					{#if u.birthday}🎂 {u.birthday}{/if}
					{#if u.birthday} · {/if}
					{u.myClaims} claimed
				</span>
				{#if spendingOn && u.spendCents !== undefined}
					<span class="meta muted">
						<b class="spend-amt">{formatMoney(u.spendCents)}</b>
					</span>
				{/if}
			</div>
			<a href={`#/list/${u.id}`}><button class="secondary">View list</button></a>
		</div>
	{/each}
	<EmptyState
		icon="list"
		title="No shared lists yet"
		hint="Connect with someone using a share code below, or ask them to connect once you share your code from your profile."
		ready={!loading && !error && !network.offline && users.length === 0}
	/>

	{#if hasMore || page > 1}
		<div class="pager">
			<button class="secondary" disabled={page <= 1} onclick={() => go(page - 1)}>Prev</button>
			<span class="muted">Page {page}</span>
			<button class="secondary" disabled={!hasMore} onclick={() => go(page + 1)}>Next</button>
		</div>
	{/if}

	<form class="composer" onsubmit={connect}>
		<div class="composer-inner">
			<div class="namerow">
				<input
					class="share-input"
					placeholder="Share code…"
					aria-label="Share code"
					autocomplete="off"
					spellcheck="false"
					bind:value={shareCode}
					disabled={connectState === 'done'}
				/>
				<button
					bind:this={connectBtnEl}
					class="connect-btn"
					class:done={connectState === 'done'}
					class:err={connectState === 'error'}
					type="submit"
					disabled={!shareCode.trim() || connectState === 'done'}
					aria-label="Connect with share code"
				>
					<span class="ci idle" aria-hidden="true"><Icon name="link" size={20} strokeWidth={2.5} /></span>
					<span class="ci ok" aria-hidden="true"><Icon name="check" size={20} strokeWidth={3} /></span>
					<span class="ci no" aria-hidden="true"><Icon name="x" size={20} strokeWidth={3} /></span>
				</button>
			</div>
			{#if connectMsg}<p class="connect-msg" aria-live="polite">{connectMsg}</p>{/if}
		</div>
	</form>
</div>

<style>
	.lists-page {
		padding-bottom: calc(var(--nav-h) + 7.5rem);
	}
	.listinfo {
		flex: 1 1 auto;
		min-width: min(13rem, 65%);
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.listinfo strong {
		font-size: 1.05rem;
	}
	.meta {
		font-size: 0.85rem;
	}
	.spend-amt {
		color: var(--acc-ink);
		font-variant-numeric: tabular-nums;
	}
	.pager {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 0.65rem;
		margin-top: 1.25rem;
	}
	.connect-btn {
		width: 52px;
		height: 52px;
		padding: 0;
		border-radius: 16px;
		justify-self: end;
		display: grid;
		place-items: center;
		position: relative;
		overflow: hidden;
		transition:
			transform 170ms cubic-bezier(.2,.8,.25,1),
			border-radius 220ms ease,
			background-color 300ms ease,
			box-shadow 0.25s ease;
	}
	.connect-btn:not(:disabled):hover { transform: scale(1.07); }
	.connect-btn:not(:disabled):active { transform: scale(.92); }
	.connect-btn .ci { position: absolute; inset: 0; display: grid; place-items: center;
		transition: opacity 220ms ease, transform 360ms cubic-bezier(.34,1.56,.64,1); }
	.connect-btn .ci.idle { opacity: 1; transform: scale(1) rotate(0deg); }
	.connect-btn .ci.ok, .connect-btn .ci.no { opacity: 0; transform: scale(0.45) rotate(-70deg); }
	.connect-btn.done { background-image: none; background-color: #15803d; border-radius: 50%;
		box-shadow: 0 6px 24px -6px rgba(52,211,153,.5); }
	.connect-btn.done .ci.idle { opacity: 0; transform: scale(0.45) rotate(70deg); }
	.connect-btn.done .ci.ok { opacity: 1; transform: scale(1) rotate(0deg); }
	.connect-btn.err { background-image: none; background-color: #dc2626; border-radius: 50%;
		box-shadow: 0 6px 24px -6px rgba(248,113,113,.5);
		animation: shake-x 420ms cubic-bezier(.36,.07,.19,.97) 3; }
	.connect-btn.err .ci.idle { opacity: 0; transform: scale(0.45) rotate(70deg); }
	.connect-btn.err .ci.no { opacity: 1; transform: scale(1) rotate(0deg); }
	@keyframes shake-x {
		0%, 100% { transform: translateX(0); }
		25% { transform: translateX(1px); }
		75% { transform: translateX(-1px); }
	}
	.connect-msg { margin: 0.5rem 0.1rem 0; font-size: 0.85rem; color: var(--muted); }
</style>
