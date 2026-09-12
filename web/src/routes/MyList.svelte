<script lang="ts">
	import { api } from '../lib/api.js';
	import { user } from '../lib/auth.js';
	import { season } from '../lib/theme.svelte.js';
	import type { Item, UserListResponse } from '../lib/types.js';
	import { normalizeItemURL } from '../lib/types.js';
	import LinkPreview from '../lib/LinkPreview.svelte';
	import ItemTile from '../lib/ItemTile.svelte';
	import EmptyState from '../lib/EmptyState.svelte';
	import ErrorState from '../lib/ErrorState.svelte';
	import OfflineState from '../lib/OfflineState.svelte';
	import PageHeader from '../lib/PageHeader.svelte';
	import Icon from '../lib/Icon.svelte';
	import Composer from '../lib/design/Composer.svelte';
	import ConfirmDialog from '../lib/ConfirmDialog.svelte';
	import { confetti } from '../lib/confetti.js';
	import {
		enqueueItem,
		isOfflineError,
		pendingItems,
		syncPendingItems,
		startAutoSync,
		type PendingItem
	} from '../lib/offlineQueue.js';
	import { network } from '../lib/network.svelte.js';
	import { buildListText, copyToClipboard } from '../lib/listText.js';

	interface DisplayItem extends Item {
		unsynced?: boolean;
		localId?: string;
	}

	let items = $state<Item[]>([]);
	let pending = $state<PendingItem[]>([]);
	let error = $state('');
	let loading = $state(true);
	let showRemoved = $state(false);
	let openId = $state<string | null>(null);
	let editing = $state<Item | null>(null);
	let copied = $state(false);
	let dialogEl: HTMLDialogElement | undefined = $state();
	let listEl: HTMLElement | undefined = $state();

	// private / suggested confirmations
	let confirmState = $state<{ title: string; message: string; confirmLabel: string; danger?: boolean; action: () => Promise<void> } | null>(null);

	$effect(() => {
		if (editing && dialogEl && !dialogEl.open) dialogEl.showModal();
		if (!editing && dialogEl?.open) dialogEl.close();
	});

	const canHover = matchMedia('(hover: hover) and (pointer: fine)').matches;

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			const me = $user;
			if (!me) return;
			const data = await api<UserListResponse>(`/api/users/${me.id}/list`);
			items = data.items;
			if (navigator.onLine) network.offline = false;
		} catch (err) {
			if (isOfflineError(err) || !navigator.onLine) {
				network.offline = true;
			} else {
				error = err instanceof Error ? err.message : String(err);
			}
		} finally {
			loading = false;
		}
	}

	function refreshPending(): void {
		pending = pendingItems();
	}

	load();
	refreshPending();
	startAutoSync();

	let activeItems = $derived(items.filter((i) => !i.removedAt));
	let removedItems = $derived(items.filter((i) => i.removedAt));
	let isEmpty = $derived(activeItems.length === 0 && pending.length === 0);

	function scrollToBottom(): void {
		requestAnimationFrame(() => {
			listEl?.lastElementChild?.scrollIntoView({ behavior: 'smooth', block: 'center' });
		});
	}

	async function copyList(): Promise<void> {
		error = '';
		try {
			await copyToClipboard(
				buildListText({
					ownerName: $user?.displayName ?? '',
					event: season.label,
					items,
					isOwner: true
				})
			);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function addItem(item: { title: string; url: string; notes: string }): Promise<boolean> {
		error = '';
		try {
			await api('/api/my/items', { method: 'POST', body: item });
			await load();
			scrollToBottom();
			return true;
		} catch (err) {
			if (isOfflineError(err)) {
				enqueueItem(item.title, normalizeItemURL(item.url) ?? '', item.notes);
				refreshPending();
				scrollToBottom();
				return true;
			}
			error = err instanceof Error ? err.message : String(err);
			return false;
		}
	}

	async function syncNow(): Promise<void> {
		await syncPendingItems();
		refreshPending();
		await load();
		scrollToBottom();
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
			items = items.map((i) =>
				i.id === item.id ? { ...i, removedAt: new Date().toISOString() } : i
			);
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
			items = items.map((i) => (i.id === item.id ? { ...i, removedAt: null } : i));
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	function onCardClick(e: MouseEvent, item: DisplayItem): void {
		const t = e.target as Element;
		if (canHover || t.closest('a, button') || item.removedAt) return;
		openId = openId === item.id ? null : item.id;
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
		if (drag || !listEl || e.button > 0) return;
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
		items = [...next, ...removedItems];
		try {
			await api('/api/my/items/order', { method: 'PUT', body: { ids: next.map((i) => i.id) } });
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

<div class="mylist">
	<PageHeader owner={($user?.displayName ?? '').split(/\s+/)[0] ?? ''}>
		<div class="stats">
			<span class="pill"><Icon name="sparkle" size={14} /> <b>{activeItems.length}</b> ideas</span>
			<span class="pill days">
				<Icon name={season.kind === 'christmas' ? 'tree' : 'cake'} size={14} />
				<b>{season.days}</b>d till {season.label || 'your next reset'}
			</span>
			<button class="pill copy-btn" class:copied type="button" onclick={() => void copyList()} aria-label="Copy list to clipboard">
				<Icon name={copied ? 'check' : 'copy'} size={14} />
			</button>
		</div>
	</PageHeader>

	{#if network.offline}
		<OfflineState onRetry={() => void load()} />
	{:else if error}
		<ErrorState message={error} onRetry={() => void load()} />
	{/if}
	{#if pending.length > 0}
		<div class="spread" style="margin-bottom:.75rem">
			<span class="badge warn unsynced-pulse">{pending.length} unsynced</span>
			<button class="ghost" type="button" onclick={() => void syncNow()}>Try sync now</button>
		</div>
	{/if}

	<EmptyState
		icon="gift"
		title="No gift ideas yet"
		hint="Type an idea or paste a link below. Your wishlist starts here."
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
						
					</div>
					<div class="actions">
						<button class="act edit" aria-label="Edit {item.title}" onclick={() => openEdit(item)}>
							<Icon name="edit" />
						</button>
						<button class="act del" aria-label="Remove {item.title}" onclick={() => removeItem(item)}>
							<Icon name="trash" />
						</button>
						<button
							class="act drag"
							aria-label="Reorder {item.title}"
							title="Drag to reorder (or use arrow keys)"
							onpointerdown={(e) => onDragStart(e, i)}
							onkeydown={(e) => onDragKey(e, i)}
						>
							<Icon name="grip" />
						</button>
					</div>
				</article>
			</li>
		{/each}

		{#each pending as p (p.localId)}
			<li class="itemcard">
				<article class="card">
					<div class="row1">
						<span class="tile">{p.title.slice(0, 1).toUpperCase()}</span>
						<div class="txt"><h3>{p.title}</h3></div>
						<span class="badge warn unsynced-pulse">unsynced</span>
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
								<button class="restore-btn" onclick={(e) => restoreItem(item, e)}>
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

	<Composer onadd={(i) => addItem(i)} />

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

<style>
	.mylist {
		padding-bottom: calc(var(--nav-h) + 185px);
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
