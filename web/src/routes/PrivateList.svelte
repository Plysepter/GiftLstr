<script lang="ts">
	import { onDestroy } from 'svelte';
	import { api } from '../lib/api.js';
	import { user } from '../lib/auth.js';
	import type { Item, PrivateListResponse } from '../lib/types.js';
	import { normalizeItemURL } from '../lib/types.js';
	import { formatMoney, centsToDollars, dollarsToCents } from '../lib/money.js';
	import { buildListText, copyToClipboard } from '../lib/listText.js';
	import LinkPreview from '../lib/LinkPreview.svelte';
	import ItemTile from '../lib/ItemTile.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import PageHeader from '../lib/PageHeader.svelte';
	import Icon from '../lib/Icon.svelte';
	import Loader from '../lib/Loader.svelte';
	import Composer from '../lib/design/Composer.svelte';
	import ConfirmDialog from '../lib/ConfirmDialog.svelte';
	import PrivateListDialog from '../lib/PrivateListDialog.svelte';
	import { confetti } from '../lib/confetti.js';
	import { isOfflineError } from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';
	import { prefs, resolveSeason, ALL_STYLES, ownerStyleOverride, apply } from '../lib/theme.svelte.js';
	import type { ThemeStyle } from '../lib/theme.svelte.js';

	let { listId }: { listId: string } = $props();

	let data = $state<PrivateListResponse | null>(null);
	let error = $state('');
	let loading = $state(true);
	let showRemoved = $state(false);
	let openId = $state<string | null>(null);
	let editing = $state<Item | null>(null);
	let priceError = $state('');
	let dialogEl: HTMLDialogElement | undefined = $state();
	let listEl: HTMLElement | undefined = $state();

	let confirmState = $state<{ title: string; message: string; confirmLabel: string; danger?: boolean; action: () => Promise<void> } | null>(null);

	$effect(() => {
		if (editing && dialogEl && !dialogEl.open) dialogEl.showModal();
		if (!editing && dialogEl?.open) dialogEl.close();
	});

	const canHover = matchMedia('(hover: hover) and (pointer: fine)').matches;

	const spendingOn = $derived($user?.spendingEnabled === true);

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			data = await api<PrivateListResponse>(`/api/private-lists/${listId}`);
			if (navigator.onLine) network.offline = false;
		} catch (err) {
			if (isOfflineError(err) || !navigator.onLine) network.offline = true;
			else error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}
	load();

	let ownerSeason = $derived(
		data?.user.birthday ? resolveSeason(data.user.birthday, prefs.events, '') : null
	);

	let activeItems = $derived((data?.items ?? []).filter((i) => !i.removedAt));
	let removedItems = $derived((data?.items ?? []).filter((i) => i.removedAt));
	let isEmpty = $derived(activeItems.length === 0);
	let displayName = $derived(data?.user.displayName ?? '');
	let firstName = $derived(displayName ? displayName.split(/\s+/)[0] ?? '' : '…');

	// The list's chosen theme drives this page's palette, and it resets when we
	// leave so the viewer's account theme returns.
	let appliedTheme = $state<ThemeStyle | null>(null);
	$effect(() => {
		const style = data?.user.themeStyle;
		if (style && ALL_STYLES.includes(style as never) && ownerStyleOverride.value !== style) {
			ownerStyleOverride.value = style as ThemeStyle;
			appliedTheme = style as ThemeStyle;
			apply();
		}
	});
	onDestroy(() => {
		if (appliedTheme !== null && ownerStyleOverride.value === appliedTheme) {
			ownerStyleOverride.value = null;
			apply();
		}
	});

	let copied = $state(false);
	async function copyList(): Promise<void> {
		if (!data) return;
		error = '';
		try {
			await copyToClipboard(
				buildListText({
					ownerName: data.user.displayName,
					event: ownerSeason?.label ?? '',
					items: data.items,
					isOwner: true
				})
			);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	// ---------- edit list ----------
	let editOpen = $state(false);
	const editInitial = $derived<{
		firstName: string;
		lastName: string;
		birthday: string;
		theme: ThemeStyle;
	} | null>(
		data
			? {
					firstName: displayName.split(/\s+/)[0] ?? '',
					lastName: displayName.split(/\s+/).slice(1).join(' '),
					birthday: data.user.birthday,
					theme: (data.user.themeStyle as ThemeStyle) ?? 'aurora'
				}
			: null
	);

	function scrollToBottom(): void {
		requestAnimationFrame(() => {
			listEl?.lastElementChild?.scrollIntoView({ behavior: 'smooth', block: 'center' });
		});
	}

	// Private-list ideas never go through the offline queue: they carry a
	// listId the queue doesn't know about, so an offline add is surfaced
	// instead of silently deferred.
	async function addItem(item: { title: string; url: string; notes: string }): Promise<boolean> {
		error = '';
		try {
			await api('/api/my/items', { method: 'POST', body: { ...item, listId } });
			await load();
			scrollToBottom();
			return true;
		} catch (err) {
			if (isOfflineError(err)) {
				network.offline = true;
				return false;
			}
			error = err instanceof Error ? err.message : String(err);
			return false;
		}
	}

	function openEdit(item: Item): void {
		editing = { ...item };
	}

	async function saveEdit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (!editing) return;
		const normalized = normalizeItemURL(editing.url);
		if (normalized === null) {
			error = 'URL must be a valid http(s) link, e.g. https://example.com';
			return;
		}
		try {
			await api(`/api/my/items/${editing.id}`, {
				method: 'PATCH',
				body: { title: editing.title.trim(), url: normalized, notes: editing.notes }
			});
			editing = null;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function removeItem(item: Item): Promise<void> {
		error = '';
		openId = null;
		try {
			await api(`/api/my/items/${item.id}`, { method: 'DELETE' });
			if (data) {
				data = {
					...data,
					items: data.items.map((i) =>
						i.id === item.id ? { ...i, removedAt: new Date().toISOString() } : i
					)
				};
			}
			showRemoved = true;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function restoreItem(item: Item, e?: MouseEvent): Promise<void> {
		error = '';
		if (e?.currentTarget instanceof HTMLElement) {
			const r = e.currentTarget.getBoundingClientRect();
			confetti(r.left + r.width / 2, r.top + r.height / 2, ['#34D399', '#10B981', '#A7F3D0']);
		}
		try {
			await api(`/api/my/items/${item.id}/restore`, { method: 'POST' });
			if (data) {
				data = { ...data, items: data.items.map((i) => (i.id === item.id ? { ...i, removedAt: null } : i)) };
			}
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
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

	async function setPrice(item: Item, raw: string): Promise<void> {
		priceError = '';
		const cents = dollarsToCents(raw);
		if (cents === null) {
			priceError = 'Enter a valid price, e.g. 12.34';
			return;
		}
		try {
			await api<{ mySpendCents: number; totalSpendCents: number }>(`/api/items/${item.id}/price`, {
				method: 'PATCH',
				body: { price: cents / 100 }
			});
			await load();
		} catch (err) {
			priceError = err instanceof Error ? err.message : String(err);
		}
	}

	function onCardClick(e: MouseEvent, item: Item): void {
		const t = e.target as Element;
		if (canHover || t.closest('a, button') || item.removedAt) return;
		openId = openId === item.id ? null : item.id;
	}

	function confirmDeleteList(): void {
		confirmState = {
			title: 'Delete this list?',
			message: `Delete ${displayName} permanently? All ideas and claims on it go with it. This cannot be undone.`,
			confirmLabel: 'Delete list',
			danger: true,
			action: async () => {
				await deleteList();
				confirmState = null;
			}
		};
	}

	async function deleteList(): Promise<void> {
		error = '';
		try {
			await api(`/api/private-lists/${listId}`, { method: 'DELETE' });
			window.location.hash = '#/lists';
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	function confirmArchiveList(): void {
		confirmState = {
			title: 'Archive this list?',
			message: `Archive ${displayName}'s list? Claimed ideas stay in the archive and the rest carry over to a fresh list.`,
			confirmLabel: 'Archive list',
			action: async () => {
				await archiveList();
				confirmState = null;
			}
		};
	}

	async function archiveList(): Promise<void> {
		error = '';
		try {
			const res = await api<{ id: string }>(`/api/private-lists/${listId}/archive`, { method: 'POST' });
			window.location.hash = `#/private-list/${res.id}`;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	// ---------- reordering ----------
	interface DragCtx {
		index: number;
		target: number;
		startY: number;
		centers: number[];
		lis: HTMLElement[];
		height: number;
		gap: number;
	}

	let drag: DragCtx | null = null;

	function onDragStart(e: PointerEvent, index: number): void {
		if (network.offline || drag || !listEl || e.button > 0) return;
		const lis = [
			...(listEl.querySelectorAll<HTMLLIElement>('li[data-item]') ?? [])
		];
		const targetLi = lis[index];
		if (lis.length < 2 || !targetLi) return;
		e.preventDefault();
		const rects = lis.map((li) => li.getBoundingClientRect());
		const rect = rects[index];
		const first = rects[0];
		const neighbour = rects[1];
		if (!rect || !first || !neighbour) return;
		drag = {
			index,
			target: index,
			startY: e.clientY,
			centers: rects.map((r) => r.top + r.height / 2),
			lis,
			height: rect.height,
			gap: Math.max(4, neighbour.top - first.bottom)
		};
		targetLi.classList.add('dragging');
		document.body.classList.add('reordering');
		window.addEventListener('pointermove', onDragMove);
		window.addEventListener('pointerup', onDragEnd);
		window.addEventListener('pointercancel', onDragEnd);
	}

	function onDragMove(e: PointerEvent): void {
		const d = drag;
		if (!d) return;
		const origin = d.lis[d.index];
		const originCenter = d.centers[d.index];
		if (!origin || originCenter === undefined) return;
		const n = d.lis.length;
		const dy = e.clientY - d.startY;
		const center = originCenter + dy;
		let target = 0;
		for (let j = 0; j < n; j++) {
			const c = d.centers[j];
			if (j !== d.index && c !== undefined && c < center) target++;
		}
		d.target = Math.max(0, Math.min(n - 1, target));
		origin.style.transform = `translateY(${dy}px)`;
		const shift = d.height + d.gap;
		for (let j = 0; j < n; j++) {
			const li = d.lis[j];
			if (j === d.index || !li) continue;
			const t =
				d.target > d.index && j > d.index && j <= d.target
					? -shift
					: d.target < d.index && j >= d.target && j < d.index
						? shift
						: 0;
			li.style.transform = t ? `translateY(${t}px)` : '';
		}
	}

	function onDragEnd(): void {
		window.removeEventListener('pointermove', onDragMove);
		window.removeEventListener('pointerup', onDragEnd);
		window.removeEventListener('pointercancel', onDragEnd);
		document.body.classList.remove('reordering');
		const d = drag;
		drag = null;
		if (!d) return;
		d.lis.forEach((li) => {
			li.classList.remove('dragging');
			li.style.transform = '';
			li.style.zIndex = '';
		});
		if (d.target !== d.index) void commitReorder(d.index, d.target);
	}

	async function commitReorder(from: number, to: number): Promise<void> {
		const next = [...activeItems];
		const [moved] = next.splice(from, 1);
		if (!moved) return;
		next.splice(to, 0, moved);
		if (data) data = { ...data, items: [...next, ...removedItems] };
		try {
			await api('/api/my/items/order', {
				method: 'PUT',
				body: { ids: next.map((i) => i.id), listId }
			});
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			await load();
		}
	}

	function onDragKey(e: KeyboardEvent, index: number): void {
		const delta = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0;
		if (!delta) return;
		e.preventDefault();
		const to = index + delta;
		if (to < 0 || to >= activeItems.length) return;
		void commitReorder(index, to);
	}
</script>

<div class="privatelist">
	{#if loading && !data}
		<div class="page-loading">
			<a href="#/lists" class="back-link">
				<Icon name="chevron-left" size={16} strokeWidth={2.2} />
				All lists
			</a>
			<Loader />
		</div>
	{:else}
		<div class="page-body">
			<PageHeader owner={firstName} accent="Wishlist">
				<a href="#/lists" class="back-link">
					<Icon name="chevron-left" size={16} strokeWidth={2.2} />
					All lists
				</a>
				<div class="stats">
					<span class="pill priv-pill"><Icon name="lock" size={14} /> Private</span>
					<span class="pill"><Icon name="gift" size={14} /> <b>{activeItems.length}</b> ideas</span>
					{#if ownerSeason}
						<span class="pill days">
							<Icon name={ownerSeason.kind === 'christmas' ? 'tree' : 'cake'} size={14} />
							<b>{ownerSeason.days}</b>d till {ownerSeason.label || 'their next reset'}
						</span>
					{/if}
					{#if spendingOn && data?.totalSpendCents !== undefined}
						<span class="pill spend-pill" title="Your private spend on the active items in this list">
							<b>{formatMoney(data.totalSpendCents)}</b>
						</span>
					{/if}
					{#if data}
						<button class="pill copy-btn" class:copied type="button" onclick={() => void copyList()} aria-label="Copy list to clipboard">
							<Icon name={copied ? 'check' : 'copy'} size={14} />
						</button>
					{/if}
				</div>
			</PageHeader>

			{#if network.offline}
				<OfflineState onRetry={() => void load()} />
			{:else if error}
				<ErrorState message={error} onRetry={() => void load()} />
			{/if}
			{#if priceError}
				<p class="error price-error" role="alert">{priceError}</p>
			{/if}

			<EmptyState
				icon="gift"
				title="No gift ideas yet"
				hint="Type an idea or paste a link below — it stays between you and this list."
				ready={!loading && !error && !network.offline && isEmpty}
			/>

			<ul class="items" bind:this={listEl}>
				{#each activeItems as item, i (item.id)}
					<li class="itemcard item-in" class:open={openId === item.id} data-item={item.id}>
						<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
						<article class="card" onclick={(e) => onCardClick(e, item)}>
							<div class="row1">
								<ItemTile title={item.title} url={item.url} />
								<div class="txt">
									<h3>{item.title}</h3>
									{#if item.url}<a class="domain" href={item.url} target="_blank" rel="noreferrer">{new URL(item.url).host} ↗</a>{/if}
								</div>
							</div>
							<div class="body">
								{#if item.url}<LinkPreview url={item.url} />{/if}
								{#if item.notes}
									<p class="notes"><span>{item.notes}</span></p>
								{/if}
								<div class="claim-row">
									<button
										class="claim-btn"
										class:on={item.claimedByMe}
										disabled={(item.claimed && !item.claimedByMe) || network.offline}
										onclick={(e) => toggleClaim(item, e)}
									>
										<Icon name={item.claimedByMe ? 'check' : 'heart'} size={15} />
										{item.claimedByMe ? 'Unclaim' : item.claimed ? 'Claimed' : 'Claim'}
									</button>
									{#if spendingOn && item.claimedByMe}
										<input
											class="price-input"
											inputmode="decimal"
											placeholder="$0.00"
											value={centsToDollars(item.mySpendCents ?? 0)}
											aria-label={`Price for ${item.title}`}
											onchange={(e) => void setPrice(item, (e.currentTarget as HTMLInputElement).value)}
											onkeydown={(e) => {
												if (e.key === 'Enter') (e.currentTarget as HTMLInputElement).blur();
											}}
										/>
									{/if}
								</div>
							</div>
							<div class="actions">
								<button class="act edit" aria-label="Edit {item.title}" disabled={network.offline} onclick={() => openEdit(item)}>
									<Icon name="edit" />
								</button>
								<button class="act del" aria-label="Remove {item.title}" disabled={network.offline} onclick={() => removeItem(item)}>
									<Icon name="trash" />
								</button>
								<button
									class="act drag"
									aria-label="Reorder {item.title}"
									title="Drag to reorder (or use arrow keys)"
									disabled={network.offline}
									onpointerdown={(e) => onDragStart(e, i)}
									onkeydown={(e) => onDragKey(e, i)}
								>
									<Icon name="grip" />
								</button>
							</div>
						</article>
					</li>
				{/each}
			</ul>

			<div class="removed-row">
				<button
					class="removed-toggle"
					type="button"
					hidden={removedItems.length === 0}
					aria-pressed={showRemoved}
					onclick={() => (showRemoved = !showRemoved)}
				>
					<Icon name="eye" size={14} /> {removedItems.length} removed · {showRemoved ? 'hide' : 'show'}
				</button>
			</div>

			{#if showRemoved && removedItems.length > 0}
				<ul class="items removed-list">
					{#each removedItems as item (item.id)}
						<li class="itemcard is-removed">
							<article class="card">
								<div class="row1">
									<ItemTile title={item.title} url={item.url} />
									<div class="txt"><h3>{item.title}</h3></div>
									<div class="actions">
										<button class="restore-btn" disabled={network.offline} onclick={(e) => restoreItem(item, e)}>
											<Icon name="undo" size={14} /> Restore
										</button>
									</div>
								</div>
								<span class="badge mutedb">removed</span>
							</article>
						</li>
					{/each}
				</ul>
			{/if}

			<div class="danger-zone">
				<div class="zone-row">
					<button
						class="secondary edit-list-btn"
						type="button"
						disabled={network.offline}
						onclick={() => (editOpen = true)}
					>
						<Icon name="edit" size={15} /> Edit list
					</button>
					<button class="secondary archive-list-btn" type="button" disabled={network.offline} onclick={confirmArchiveList}>
						<Icon name="archive" size={15} /> Archive {firstName}
					</button>
					<button class="danger delete-list-btn" type="button" disabled={network.offline} onclick={confirmDeleteList}>
						<Icon name="trash" size={15} /> Delete {firstName}
					</button>
				</div>
				<span class="hint muted">Editing updates the name, birthday, or theme. Archive moves it to the archive and starts a fresh list; delete hard-removes every idea and claim on it.</span>
			</div>

			<Composer onadd={(i) => addItem(i)} label="Add gift idea" />

			<dialog
				class="edit-dlg"
				bind:this={dialogEl}
				onclose={() => (editing = null)}
				aria-label="Edit item"
			>
				<form class="dlg-card" method="dialog" onsubmit={saveEdit}>
					<div class="dlg-head">
						<h2>Edit item</h2>
						<button class="dlg-close" type="button" onclick={() => (editing = null)} aria-label="Close">
							<Icon name="x" size={14} />
						</button>
					</div>
					{#if editing}
						<label for="eName">Name</label>
						<input id="eName" bind:value={editing.title} required autocomplete="off" />
						<label for="eUrl">Link</label>
						<input id="eUrl" type="url" inputmode="url" autocomplete="off" placeholder="https://…" bind:value={editing.url} />
						<label for="eNotes">Notes</label>
						<textarea id="eNotes" rows="3" bind:value={editing.notes}></textarea>
						<div class="dlg-menu">
							<button class="secondary" type="button" onclick={() => (editing = null)}>Cancel</button>
							<button type="submit">Save changes</button>
						</div>
					{/if}
				</form>
			</dialog>

			<PrivateListDialog
				open={editOpen}
				listId={listId}
				initial={editInitial}
				onclose={() => (editOpen = false)}
				onsaved={() => void load()}
			/>

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
	.privatelist {
		padding-bottom: calc(var(--nav-h) + 185px);
	}
	.page-loading {
		display: flex;
		flex-direction: column;
		padding: 0.75rem 0;
	}
	.page-loading :global(.loader-box) {
		flex: 1;
	}
	.page-body {
		animation: page-in 240ms ease-out backwards;
	}
	@keyframes page-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
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
	.priv-pill {
		color: var(--acc-ink);
	}
	.spend-pill {
		color: var(--acc-ink);
	}
	.items {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.removed-list {
		margin-top: 0.95rem;
	}
	.item-in {
		animation: rise-in 460ms cubic-bezier(0.2, 0.8, 0.25, 1) backwards;
	}
	@keyframes rise-in {
		from {
			opacity: 0;
			transform: translateY(16px) scale(0.985);
		}
	}
	.notes span {
		color: var(--muted);
		font-size: 0.88rem;
		line-height: 1.5;
	}
	.claim-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-top: 0.65rem;
	}
	.price-input {
		width: 6.5rem;
		padding: 0.35rem 0.6rem;
		border-radius: 10px;
		background: color-mix(in srgb, var(--solid) 35%, transparent);
		font-variant-numeric: tabular-nums;
	}
	@media (prefers-reduced-motion: no-preference) {
		.price-input {
			transition:
				border-color 0.16s ease,
				box-shadow 0.25s ease,
				background-color 0.45s ease;
		}
	}
	.price-error {
		margin: 0.65rem 0;
	}
	.danger-zone {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.4rem;
		margin-top: 1.75rem;
		padding-top: 1.15rem;
		border-top: 1px solid var(--line);
	}
	.danger-zone .zone-row {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}
	.danger-zone .hint {
		font-size: 0.78rem;
		text-align: center;
		max-width: 34rem;
	}
	.edit-list-btn,
	.archive-list-btn,
	.delete-list-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.45em;
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
