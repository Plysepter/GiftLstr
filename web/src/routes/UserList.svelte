<script lang="ts">
	import { api } from '../lib/api.js';
	import { user } from '../lib/auth.js';
	import type { Item, UserListResponse } from '../lib/types.js';
	import LinkPreview from '../lib/LinkPreview.svelte';
	import ItemTile from '../lib/ItemTile.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import PageHeader from '../lib/PageHeader.svelte';
	import Icon from '../lib/Icon.svelte';
	import { confetti } from '../lib/confetti.js';
	import { isOfflineError } from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';
	import { season, prefs, resolveSeason, ALL_STYLES, ownerStyleOverride, apply } from '../lib/theme.svelte.js';
	import type { ThemeStyle } from '../lib/theme.svelte.js';
	import { untrack } from 'svelte';
	import { buildListText, copyToClipboard } from '../lib/listText.js';

	let { userId }: { userId: string } = $props();

	let data = $state<UserListResponse | null>(null);
	let error = $state('');
	let loading = $state(true);
	let openId = $state<string | null>(null);
	let revealClaimers = $state(false);
	let copied = $state(false);

	const canHover = matchMedia('(hover: hover) and (pointer: fine)').matches;

	let ownerSeason = $derived(
		data?.user.birthday ? resolveSeason(data.user.birthday, prefs.events) : null
	);

	let prevSeason: { kind: typeof season.kind; days: number; label: string; dataset: string } | null = null;

	// $effect.pre: the owner's season/style must land before the owner's items render,
	// otherwise the page paints one frame in the viewer's theme and then "pops" to
	// the owner's (background blobs brighten as data-style changes after paint).
	$effect.pre(() => {
		void prefs.useOwnerTheme;
		void prefs.events;
		const owner = data?.user;
		const isOwner = data?.isOwner;

		if (!owner || isOwner) {
			const ps = prevSeason;
			if (ps !== null) {
				untrack(() => {
					season.kind = ps.kind;
					season.days = ps.days;
					season.label = ps.label;
				});
				document.documentElement.dataset.season = ps.dataset;
				prevSeason = null;
			}
			if (untrack(() => ownerStyleOverride.value) !== null) {
				untrack(() => {
					ownerStyleOverride.value = null;
				});
				apply();
			}
			return;
		}

		// Season: respect viewer's events toggle, dataset + global season for Weather/PageHeader
		const birthday = untrack(() => owner.birthday);
		const eventsEnabled = untrack(() => prefs.events);
		if (birthday) {
			const os = untrack(() => resolveSeason(birthday, eventsEnabled));
			if (prevSeason === null) {
				prevSeason = untrack(() => ({
					kind: season.kind,
					days: season.days,
					label: season.label,
					dataset: document.documentElement.dataset.season ?? 'none'
				}));
			}
			// Keep global season in sync so Weather/PageHeader show owner's event
			untrack(() => {
				season.kind = os.kind;
				season.days = os.days;
				season.label = os.label;
			});
			if (document.documentElement.dataset.season !== os.kind) {
				document.documentElement.dataset.season = os.kind;
			}
		}

		// Style: respect viewer's light/dark, and the owner's palette via the
		// ownerStyleOverride, so a theme toggle re-skins brightness only.
		const ownerStyle = untrack(() => owner.themeStyle);
		const useOwner = untrack(() => prefs.useOwnerTheme);
		const wantsOwnerStyle = useOwner && ownerStyle && untrack(() => ALL_STYLES.includes(ownerStyle as never));
		const activeOverride = untrack(() => ownerStyleOverride.value);
		if (wantsOwnerStyle && activeOverride !== ownerStyle) {
			untrack(() => {
				ownerStyleOverride.value = ownerStyle as ThemeStyle;
			});
			apply();
		} else if (!wantsOwnerStyle && activeOverride !== null) {
			untrack(() => {
				ownerStyleOverride.value = null;
			});
			apply();
		}

		return () => {
			const ps = prevSeason;
			if (ps !== null) {
				untrack(() => {
					season.kind = ps.kind;
					season.days = ps.days;
					season.label = ps.label;
				});
				document.documentElement.dataset.season = ps.dataset;
				prevSeason = null;
			}
			if (untrack(() => ownerStyleOverride.value) !== null) {
				untrack(() => {
					ownerStyleOverride.value = null;
				});
				apply();
			}
		};
	});

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams();
			if (revealClaimers) params.set('revealClaimers', '1');
			const qs = params.toString();
			data = await api<UserListResponse>(`/api/users/${userId}/list${qs ? `?${qs}` : ''}`);
			if (navigator.onLine) network.offline = false;
		} catch (err) {
			if (isOfflineError(err) || !navigator.onLine) network.offline = true;
			else error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}
	load();

	async function setReveal(v: boolean): Promise<void> {
		revealClaimers = v;
		await load();
	}

	async function toggleClaim(item: Item, e?: MouseEvent): Promise<void> {
		error = '';
		openId = null;
		const claiming = !item.claimedByMe;
		if (claiming && e?.currentTarget instanceof HTMLElement) {
			const r = e.currentTarget.getBoundingClientRect();
			confetti(r.left + r.width / 2, r.top + r.height / 2, ['#34D399', '#10B981', '#A7F3D0']);
		}
		try {
			if (item.claimedByMe) {
				await api(`/api/items/${item.id}/claim`, { method: 'DELETE' });
			} else {
				await api(`/api/items/${item.id}/claim`, { method: 'PUT' });
			}
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	let visible = $derived(
		data ? data.items.filter((i) => !i.removedAt || i.claimed || i.claimedByMe) : []
	);

	async function copyList(): Promise<void> {
		if (!data) return;
		error = '';
		try {
			await copyToClipboard(
				buildListText({
					ownerName: data.user.displayName,
					event: ownerSeason?.label ?? '',
					items: data.items,
					isOwner: data.isOwner
				})
			);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	function onCardClick(e: MouseEvent, id: string): void {
		const t = e.target as Element;
		if (canHover || t.closest('a, button')) return;
		openId = openId === id ? null : id;
	}
</script>
<div>
	<PageHeader owner={data?.user.displayName ? data.user.displayName.split(/\s+/)[0] ?? '…' : '…'} accent="Wishlist">
		<a href="#/lists" class="back-link">
			<Icon name="chevron-left" size={16} strokeWidth={2.2} />
			All lists
		</a>
		<div class="stats">
			<span class="pill"><Icon name="gift" size={14} /> <b>{visible.length}</b> ideas</span>
			{#if ownerSeason}
				<span class="pill days">
					<Icon name={ownerSeason?.kind === 'christmas' ? 'tree' : 'cake'} size={14} />
					<b>{ownerSeason.days}</b>d till {ownerSeason.label || 'their next reset'}
				</span>
			{/if}
			{#if data}
				<button class="pill copy-btn" class:copied type="button" onclick={() => void copyList()} aria-label="Copy list to clipboard">
					<Icon name={copied ? 'check' : 'copy'} size={14} />
				</button>
			{/if}

			{#if data && $user?.isAdmin && !data.isOwner}
					<button
						class="reveal-toggle"
						type="button"
						aria-pressed={revealClaimers}
						aria-label={revealClaimers ? 'Hide who claimed items' : 'View who claimed items'}
						onclick={() => void setReveal(!revealClaimers)}
					>
						<Icon name="eye" size={14} />
						{revealClaimers ? 'Hide who claimed items' : 'View who claimed items'}
					</button>
			{/if}
		</div>
	</PageHeader>

	{#if network.offline}
		<OfflineState onRetry={() => void load()} />
	{:else if error}
		<ErrorState message={error} onRetry={() => void load()} />
	{/if}

	<ul class="items">
		{#each visible as item (item.id)}
			<li class="itemcard" class:open={openId === item.id}>
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
				<article class="card" onclick={(e) => onCardClick(e, item.id)}>
					<div class="row1">
						<ItemTile title={item.title} url={item.url} />
						<div class="txt">
							<h3>{item.title}</h3>
							{#if item.url}<a class="domain" href={item.url} target="_blank" rel="noreferrer">{new URL(item.url).host} ↗</a>{/if}
						</div>
						<div class="actions">
							<button
								class="claim-btn"
								class:on={item.claimedByMe}
								disabled={item.claimed && !item.claimedByMe}
								onclick={(e) => toggleClaim(item, e)}
							>
								<Icon name={item.claimedByMe ? 'check' : 'heart'} size={15} />
								{item.claimedByMe ? 'Unclaim' : item.claimed ? 'Claimed' : 'Claim'}
							</button>
						</div>
					</div>
					<div class="body">
						{#if item.url}<LinkPreview url={item.url} />{/if}
						{#if item.notes}<p class="notes"><span>{item.notes}</span></p>{/if}
						{#if item.suggestedBy}
							<span class="badge suggested-badge">Suggested by {item.suggestedBy}</span>
						{/if}
						{#if revealClaimers && item.claimed && !item.claimedByMe && item.claimedByName}
							<span class="badge claimed claim-owner-badge">claimed by {item.claimedByName}</span>
						{/if}
						{#if item.removedAt}<span class="badge warn removed-badge">removed by owner</span>{/if}
					</div>
				</article>
			</li>
		{/each}
	</ul>

	<EmptyState
		icon="gift"
		title="No gifts on this list yet"
		hint="Check back later — maybe they are still thinking."
		ready={!loading && !error && !network.offline && data !== null && visible.length === 0}
	/>
</div>

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

	.removed-badge {
		margin-top: 0.65rem;
	}

	.reveal-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.45em;
		border: 1.5px solid color-mix(in srgb, var(--acc) 40%, var(--line));
		background: transparent;
		color: var(--muted);
		font: inherit;
		font-size: 0.8rem;
		font-weight: 600;
		padding: 0.4rem 1rem;
		border-radius: 999px;
		box-shadow: none;
	}
	.reveal-toggle:hover { color: var(--ink); border-color: var(--acc); }
	.reveal-toggle[aria-pressed="true"] {
		color: var(--acc-ink);
		background: color-mix(in srgb, var(--acc) 12%, var(--surface));
		border-color: var(--acc);
	}
	.claim-owner-badge {
		margin-top: 0.65rem;
		margin-right: 0.4rem;
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
	.copy-btn {
		cursor: pointer;
		font: inherit;
		transition:
			color 0.15s ease,
			background-color 0.3s ease,
			border-color 0.15s ease,
			box-shadow 0.25s ease,
			transform 0.12s ease;
	}
	.copy-btn:hover {
		color: var(--acc-ink);
		border-color: var(--acc);
	}
	.copy-btn.copied {
		background-color: #15803d;
		border-color: #15803d;
		color: #fff;
		box-shadow: 0 6px 24px -6px rgba(52, 211, 153, 0.5);
	}
	.copy-btn:active {
		transform: scale(0.97);
	}
</style>
