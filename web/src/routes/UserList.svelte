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
	import Composer from '../lib/design/Composer.svelte';
	import ConfirmDialog from '../lib/ConfirmDialog.svelte';

	let { userId }: { userId: string } = $props();

	let data = $state<UserListResponse | null>(null);
	let error = $state('');
	let loading = $state(true);
	let openId = $state<string | null>(null);
	let revealClaimers = $state(false);
	let copied = $state(false);
	let privateError = $state('');
	let confirmState = $state<{ title: string; message: string; confirmLabel: string; danger?: boolean; action: () => Promise<void> } | null>(null);

	const canHover = matchMedia('(hover: hover) and (pointer: fine)').matches;

	const canAddPrivate = $derived(data !== null && !data.isOwner && $user?.allowSuggestions !== false);

	let privateIdeas = $derived(data?.privateItems ?? []);

	let ownerSeason = $derived(
		data?.user.birthday ? resolveSeason(data.user.birthday, prefs.events, data.user.holidayReset ?? '') : null
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
				untrack(() => apply());
			}
			return;
		}

		// Season: respect viewer's events toggle, dataset + global season for Weather/PageHeader
		const birthday = untrack(() => owner.birthday);
		const eventsEnabled = untrack(() => prefs.events);
		if (birthday) {
			const os = untrack(() => resolveSeason(birthday, eventsEnabled, owner.holidayReset ?? ''));
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
			untrack(() => apply());
		} else if (!wantsOwnerStyle && activeOverride !== null) {
			untrack(() => {
				ownerStyleOverride.value = null;
			});
			untrack(() => apply());
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
				untrack(() => apply());
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

	async function addPrivateIdea(item: { title: string; url: string; notes: string }): Promise<boolean> {
		privateError = '';
		try {
			await api(`/api/users/${userId}/private-items`, { method: 'POST', body: item });
			await load();
			return true;
		} catch (err) {
			privateError = err instanceof Error ? err.message : String(err);
			return false;
		}
	}

	function confirmDeletePrivate(item: Item): void {
		confirmState = {
			title: 'Delete private idea?',
			message: `Delete “${item.title}” permanently? This cannot be undone.`,
			confirmLabel: 'Delete',
			danger: true,
			action: async () => {
				await deletePrivate(item);
				confirmState = null;
			}
		};
	}

	async function deletePrivate(item: Item): Promise<void> {
		privateError = '';
		try {
			await api(`/api/users/${userId}/private-items/${item.id}`, { method: 'DELETE' });
			await load();
		} catch (err) {
			privateError = err instanceof Error ? err.message : String(err);
		}
	}

	function confirmSharePrivate(item: Item): void {
		confirmState = {
			title: 'Share this idea?',
			message: `“${item.title}” will appear on ${data?.user.displayName.split(/\s+/)[0] ?? 'this'} list as “Suggested by you” and can be claimed.`,
			confirmLabel: 'Share',
			action: async () => {
				await sharePrivate(item);
				confirmState = null;
			}
		};
	}

	async function sharePrivate(item: Item): Promise<void> {
		privateError = '';
		try {
			await api(`/api/users/${userId}/private-items/${item.id}/share`, { method: 'PUT' });
			await load();
		} catch (err) {
			privateError = err instanceof Error ? err.message : String(err);
		}
	}

	function confirmUnshareSuggested(item: Item): void {
		confirmState = {
			title: 'Unshare this idea?',
			message: `Remove “${item.title}” from the shared suggestions? This cannot be undone.`,
			confirmLabel: 'Unshare',
			danger: true,
			action: async () => {
				await deleteSuggested(item);
				confirmState = null;
			}
		};
	}

	async function deleteSuggested(item: Item): Promise<void> {
		privateError = '';
		try {
			await api(`/api/suggested-items/${item.id}`, { method: 'DELETE' });
			await load();
		} catch (err) {
			privateError = err instanceof Error ? err.message : String(err);
		}
	}

	let visible = $derived(
		data ? data.items.filter((i) => !i.removedAt || i.claimed || i.claimedByMe) : []
	);

	let selfAuthored = $derived(visible.filter((i) => !i.suggestedBy));
	let suggestedItems = $derived(visible.filter((i) => i.suggestedBy));

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
<div class="userlist">
	{#if loading && !data}
		<div class="page-loading">
			<a href="#/lists" class="back-link">
				<Icon name="chevron-left" size={16} strokeWidth={2.2} />
				All lists
			</a>
			<span class="loader" aria-hidden="true"></span>
		</div>
	{:else}
	<div class="page-body">
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
		{#each selfAuthored as item (item.id)}
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
						{#if revealClaimers && item.claimed && !item.claimedByMe && item.claimedByName}
							<span class="badge claimed claim-owner-badge">claimed by {item.claimedByName}</span>
						{/if}
						{#if item.removedAt}<span class="badge warn removed-badge">removed by owner</span>{/if}
					</div>
				</article>
			</li>
		{/each}
	</ul>

	{#if suggestedItems.length > 0}
		<div class="section-divider suggested-divider">
			<span class="section-label">
				<Icon name="share" size={13} /> Suggested
			</span>
			<span class="section-hint">{data?.user.displayName ? data.user.displayName.split(/\s+/)[0] : 'they'} cannot see suggestions</span>
		</div>
		<ul class="items suggested-list">
			{#each suggestedItems as item (item.id)}
				<li class="itemcard" class:open={openId === item.id}>
					<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
					<article class="card" onclick={(e) => onCardClick(e, item.id)}>
						<div class="row1">
							<ItemTile title={item.title} url={item.url} />
							<div class="txt">
								<h3>{item.title}</h3>
								{#if item.url}<a class="domain" href={item.url} target="_blank" rel="noreferrer">{new URL(item.url).host} ↗</a>{/if}
							</div>
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
						<div class="body">
							{#if item.url}<LinkPreview url={item.url} />{/if}
							{#if item.notes}<p class="notes"><span>{item.notes}</span></p>{/if}
						</div>
						<div class="actions secondary-actions">
							{#if item.suggestedBy}
								<span class="badge suggested-badge">Suggested by {item.suggestedBy}</span>
							{/if}
							{#if revealClaimers && item.claimed && !item.claimedByMe && item.claimedByName}
								<span class="badge claimed claim-owner-badge">claimed by {item.claimedByName}</span>
							{/if}
							{#if item.removedAt}<span class="badge warn removed-badge">removed by owner</span>{/if}
							{#if item.suggestedByMe}
								<button
									class="act del"
									aria-label="Unshare {item.title}"
									title="Remove from shared suggestions"
									onclick={() => confirmUnshareSuggested(item)}
								>
									<Icon name="trash" />
								</button>
							{/if}
						</div>
					</article>
				</li>
			{/each}
		</ul>
	{/if}

	<EmptyState
		icon="gift"
		title="No gifts on this list yet"
		hint="Check back later. Maybe they are still thinking."
		ready={!loading && !error && !network.offline && data !== null && visible.length === 0}
	/>

	{#if privateIdeas.length > 0}
		<div class="section-divider private-divider">
			<span class="section-label">
				<Icon name="key" size={13} /> Private ideas
			</span>
			<span class="section-hint">Only you can see these</span>
		</div>
		<ul class="items private-list">
			{#each privateIdeas as item (item.id)}
				<li class="itemcard private-item" class:open={openId === item.id}>
					<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
					<article class="card" onclick={(e) => onCardClick(e, item.id)}>
						<div class="row1 private-row1">
							<ItemTile title={item.title} url={item.url} />
							<div class="txt">
								<h3>{item.title}</h3>
								{#if item.url}<a class="domain" href={item.url} target="_blank" rel="noreferrer">{new URL(item.url).host} ↗</a>{/if}
							</div>
							<button
								class="claim-btn"
								class:on={item.claimedByMe}
								onclick={(e) => toggleClaim(item, e)}
							>
								<Icon name={item.claimedByMe ? 'check' : 'heart'} size={15} />
								{item.claimedByMe ? 'Unclaim' : 'Claim'}
							</button>
						</div>
						<div class="body">
							{#if item.url}<LinkPreview url={item.url} />{/if}
							{#if item.notes}<p class="notes"><span>{item.notes}</span></p>{/if}
							{#if item.claimedByMe}
								<span class="badge claimed">your pick</span>
							{/if}
						</div>
						<div class="actions secondary-actions">
							<button
								class="act share"
								aria-label="Share {item.title}"
								title="Share publicly on this list"
								onclick={() => confirmSharePrivate(item)}
							>
								<Icon name="share" />
							</button>
							<button
								class="act del"
								aria-label="Delete private idea {item.title}"
								title="Delete permanently"
								onclick={() => confirmDeletePrivate(item)}
							>
								<Icon name="trash" />
							</button>
						</div>
					</article>
				</li>
			{/each}
		</ul>
	{/if}

	{#if canAddPrivate}
		<div class="suggest-bar">
			{#if privateError}
				<p class="error" role="alert">{privateError}</p>
			{/if}
			<Composer
				onadd={addPrivateIdea}
				label="Add private idea"
				placeholder={'A private idea for ' + (data?.user.displayName.split(/\s+/)[0] ?? 'them') + '…'}
			/>
		</div>
	{/if}

	{#if confirmState}
		<ConfirmDialog
			open={true}
			title={confirmState.title}
			message={confirmState.message}
			confirmLabel={confirmState.confirmLabel}
			danger={confirmState.danger}
			onconfirm={confirmState.action}
			oncancel={() => (confirmState = null)}
		/>
	{/if}
	</div>
	{/if}
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

	.act.share {
		background: color-mix(in srgb, var(--acc) 14%, transparent);
		color: var(--acc);
	}
	.private-row1 {
		flex-wrap: wrap;
	}
	.private-row1 .claim-btn {
		flex-shrink: 0;
	}
	/* Mirror the My List mobile treatment: auxiliary actions sit in a
	   right-aligned row below the card body. */
	.secondary-actions {
		margin-top: 0.65rem;
		justify-content: flex-end;
		align-items: center;
	}
	.secondary-actions .badge {
		margin-top: 0;
	}
	/* Suggested item: badges stay left, delete button pushed to the far right. */
	.suggested-list .secondary-actions .act.del {
		margin-left: auto;
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

	.userlist {
		padding-bottom: calc(var(--nav-h) + 185px);
	}

	.page-loading {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		padding: 0.75rem 0;
		min-height: calc(100dvh - var(--nav-h) - 9rem);
	}

	/* Centered bouncy-dot loader; colours follow the theme. */
	.loader {
		--color-1: var(--acc, #ff3d00);
		--color-2: var(--ink, #fff);
		--size: 1px;
		width: calc(50 * var(--size));
		height: calc(165 * var(--size));
		position: absolute;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
	}
	.loader::before {
		content: '';
		position: absolute;
		left: 50%;
		top: 0;
		transform: translate(-50%, 0);
		width: calc(16 * var(--size));
		height: calc(16 * var(--size));
		background: var(--color-1);
		border-radius: 50%;
		animation: bounce 2.86s linear infinite;
	}
	.loader::after {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		margin: auto;
		height: calc(48 * var(--size));
		width: calc(48 * var(--size));
		background: var(--color-2);
		border-radius: calc(4 * var(--size));
		animation: rotate 2.86s linear infinite;
	}
	@keyframes bounce {
		0%,
		50%,
		100% {
			transform: translate(-50%, 0);
			height: calc(20 * var(--size));
		}
		20% {
			transform: translate(-25%, calc(85 * var(--size)));
			height: calc(28 * var(--size));
		}
		25% {
			transform: translate(-25%, calc(110 * var(--size)));
			height: calc(12 * var(--size));
		}
		70% {
			transform: translate(-75%, calc(85 * var(--size)));
			height: calc(28 * var(--size));
		}
		75% {
			transform: translate(-75%, calc(108 * var(--size)));
			height: calc(12 * var(--size));
		}
	}
	@keyframes rotate {
		0%,
		50%,
		100% {
			transform: rotate(0deg);
		}
		25% {
			transform: rotate(90deg);
		}
		75% {
			transform: rotate(-90deg);
		}
	}

	.page-body {
		animation: page-in 240ms ease-out both;
	}
	@keyframes page-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	.section-divider {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.6rem;
		margin: 1.6rem 0 0.4rem;
	}
	.section-label {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.78rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.section-hint {
		font-size: 0.74rem;
		color: var(--muted);
		opacity: 0.8;
	}

	.suggest-bar {
		position: fixed;
		left: 0;
		right: 0;
		bottom: calc(var(--kb, 0px) + var(--nav-h));
		z-index: 50;
		padding: 0.6rem 0.9rem 0.7rem;
		background: color-mix(in srgb, var(--bg) 95%, transparent);
		border-top: 1px solid var(--line);
	}
	.suggest-bar .error {
		margin: 0 0 0.5rem;
		text-align: center;
		color: #ef4444;
		font-size: 0.85rem;
		font-weight: 600;
	}
	.suggest-bar :global(form.composer) {
		position: static;
		max-width: 610px;
		margin: 0 auto;
		padding: 0;
		background: transparent;
		border-top: none;
	}

	/* wide screens: align the suggest bar with the content column and sit it
	   flush at the viewport bottom (the nav moved to the side) */
	@media (min-width: 900px) {
		.suggest-bar {
			margin: 0 calc(50% - var(--nav-side) / 2 - 320px) 0 calc(50% + var(--nav-side) / 2 - 320px);
			bottom: var(--kb, 0px);
			border-radius: var(--radius) var(--radius) 0 0;
		}
	}
</style>
