<script lang="ts">
	import PageHeader from '../lib/PageHeader.svelte';
	import Icon from '../lib/Icon.svelte';
	import ConfirmDialog from '../lib/ConfirmDialog.svelte';
	import { api } from '../lib/api.js';
	import { user } from '../lib/auth.js';
	import { season, seasonOverride, setSeasonOverride } from '../lib/theme.svelte.js';
	import type { Season } from '../lib/theme.svelte.js';
	import type { AdminUser, AdminUsersResponse } from '../lib/types.js';

	let users = $state<AdminUser[]>([]);
	let query = $state('');
	let error = $state('');
	let saved = $state<string | null>(null);
	let actionMsg = $state('');
	let page = $state(1);
	let total = $state(0);
	let hasMore = $state(false);
	let loading = $state(false);
	let queryDebounce: ReturnType<typeof setTimeout> | null = null;
	let nameMap = $state<Map<string, string>>(new Map());

	// per-user connect search
	let connQueries = $state<Record<string, string>>({});
	let connResults = $state<Record<string, AdminUser[]>>({});
	let connLoading = $state<Record<string, boolean>>({});
	let connDebounce = $state<Record<string, ReturnType<typeof setTimeout>>>({});

	// modal for connections
	let modalUser = $state<AdminUser | null>(null);
	let modalPage = $state(1);
	const modalPer = 10;
	let connDialogEl = $state<HTMLDialogElement | undefined>(undefined);
	$effect(() => {
		if (modalUser && connDialogEl && !connDialogEl.open) connDialogEl.showModal();
		if (!modalUser && connDialogEl?.open) connDialogEl.close();
	});

	// 3-dot menu per card
	let openMenu = $state<string | null>(null);
	$effect(() => {
		if (openMenu === null) return;
		const close = (): void => {
			openMenu = null;
		};
		const onKey = (e: KeyboardEvent): void => {
			if (e.key === 'Escape') close();
		};
		document.addEventListener('click', close);
		document.addEventListener('keydown', onKey);
		window.addEventListener('blur', close);
		return () => {
			document.removeEventListener('click', close);
			document.removeEventListener('keydown', onKey);
			window.removeEventListener('blur', close);
		};
	});

	// confirmation modal
	interface ConfirmState {
		title: string;
		message: string;
		confirmLabel: string;
		danger?: boolean;
		confirmText?: string;
		action: () => Promise<void>;
	}
	let confirmState = $state<ConfirmState | null>(null);

	async function load(): Promise<void> {
		loading = true;
		try {
			const params = new URLSearchParams();
			params.set('page', String(page));
			const q = query.trim();
			if (q) params.set('q', q);
			const d = await api<AdminUsersResponse>(`/api/admin/users?${params.toString()}`);
			users = d.users;
			total = d.total;
			hasMore = d.hasMore;
			// build nameMap from response names + current users
			const m = new Map<string, string>();
			if (d.names) {
				for (const [k, v] of Object.entries(d.names)) m.set(k, v);
			}
			for (const u of d.users) m.set(u.id, u.displayName);
			nameMap = m;
			// if modal is open, refresh its reference
			if (modalUser) {
				const fresh = users.find((x) => x.id === modalUser!.id);
				if (fresh) modalUser = fresh;
				else modalUser = null;
			}
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}
	load();

	function onQueryInput(): void {
		if (queryDebounce) clearTimeout(queryDebounce);
		queryDebounce = setTimeout(() => {
			page = 1;
			void load();
		}, 300);
	}

	function go(p: number): void {
		page = p;
		void load();
	}

	function allConnections(u: AdminUser): string[] {
		return [...new Set([...u.connections, ...u.connectedBy])];
	}

	function nameOf(id: string): string {
		return users.find((u) => u.id === id)?.displayName ?? nameMap.get(id) ?? `#${id}`;
	}

	async function saveBirthday(u: AdminUser): Promise<void> {
		error = '';
		saved = null;
		try {
			await api(`/api/admin/users/${u.id}`, { method: 'PATCH', body: { birthday: u.birthday } });
			saved = u.id;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function runToggleAdmin(u: AdminUser): Promise<void> {
		error = '';
		actionMsg = '';
		try {
			await api(`/api/admin/users/${u.id}`, {
				method: 'PATCH',
				body: { isAdmin: !u.isAdmin }
			});
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function runForceArchive(u: AdminUser): Promise<void> {
		error = '';
		actionMsg = '';
		try {
			await api(`/api/admin/users/${u.id}/archive`, { method: 'POST' });
			actionMsg = `Archived ${u.displayName}'s list.`;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function runHardDelete(u: AdminUser): Promise<void> {
		error = '';
		actionMsg = '';
		try {
			const res = await api<{ deleted: number }>(`/api/admin/users/${u.id}/list`, { method: 'DELETE' });
			actionMsg = `Deleted ${res.deleted} items from ${u.displayName}'s list.`;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	async function runDeleteUser(u: AdminUser): Promise<void> {
		error = '';
		actionMsg = '';
		try {
			await api(`/api/admin/users/${u.id}`, { method: 'DELETE' });
			actionMsg = `Deleted user ${u.displayName}.`;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	function toggleAdmin(u: AdminUser): void {
		const grant = !u.isAdmin;
		confirmState = {
			title: grant ? 'Make admin?' : 'Revoke admin?',
			message: grant
				? `Grant administrator access to ${u.displayName} (@${u.username})?`
				: `Revoke administrator access from ${u.displayName} (@${u.username})?`,
			confirmLabel: grant ? 'Make admin' : 'Revoke admin',
			action: async () => {
				await runToggleAdmin(u);
				confirmState = null;
			}
		};
	}

	function forceArchive(u: AdminUser): void {
		confirmState = {
			title: 'Archive current list?',
			message: `Archive ${u.displayName}'s current list? Unclaimed items will be copied to a new active list, and this list moves to the archive.`,
			confirmLabel: 'Archive list',
			action: async () => {
				await runForceArchive(u);
				confirmState = null;
			}
		};
	}

	function hardDelete(u: AdminUser): void {
		confirmState = {
			title: 'Delete list?',
			message: `Permanently delete all items and claims on ${u.displayName}'s current list? This cannot be undone.`,
			confirmLabel: 'Delete list',
			danger: true,
			action: async () => {
				await runHardDelete(u);
				confirmState = null;
			}
		};
	}

	function deleteUser(u: AdminUser): void {
		confirmState = {
			title: 'Delete user?',
			message: `Permanently delete ${u.displayName} (@${u.username}) and ALL their data — lists, items, claims and connections. This cannot be undone. Type the username to confirm.`,
			confirmLabel: 'Delete user',
			danger: true,
			confirmText: u.username,
			action: async () => {
				await runDeleteUser(u);
				confirmState = null;
			}
		};
	}

	async function disconnect(x: string, y: string): Promise<void> {
		confirmState = {
			title: 'Remove connection?',
			message: `Remove the connection between ${nameOf(x)} and ${nameOf(y)}?`,
			confirmLabel: 'Remove',
			danger: true,
			action: async () => {
				error = '';
				try {
					await api('/api/connections', { method: 'PUT', body: { a: x, b: y, add: false } });
					await load();
				} catch (err) {
					error = err instanceof Error ? err.message : String(err);
				}
				confirmState = null;
			}
		};
	}

	function onConnInput(u: AdminUser): void {
		const q = (connQueries[u.id] ?? '').trim();
		if (connDebounce[u.id]) clearTimeout(connDebounce[u.id]);
		if (!q) {
			connResults = { ...connResults, [u.id]: [] };
			return;
		}
		connDebounce[u.id] = setTimeout(() => void fetchConnCandidates(u, q), 300);
	}

	async function fetchConnCandidates(u: AdminUser, q: string): Promise<void> {
		connLoading = { ...connLoading, [u.id]: true };
		try {
			const params = new URLSearchParams({ q, page: '1' });
			const d = await api<AdminUsersResponse>(`/api/admin/users?${params.toString()}`);
			// also populate nameMap
			for (const cand of d.users) nameMap.set(cand.id, cand.displayName);
			const already = new Set(allConnections(u));
			already.add(u.id);
			const filtered = d.users.filter((c) => !already.has(c.id)).slice(0, 5);
			connResults = { ...connResults, [u.id]: filtered };
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			connLoading = { ...connLoading, [u.id]: false };
		}
	}

	async function addConnection(targetId: string, candidateId: string): Promise<void> {
		error = '';
		try {
			await api('/api/connections', { method: 'PUT', body: { a: targetId, b: candidateId, add: true } });
			connQueries = { ...connQueries, [targetId]: '' };
			connResults = { ...connResults, [targetId]: [] };
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	function openModal(u: AdminUser): void {
		modalUser = u;
		modalPage = 1;
		connQueries = { ...connQueries, [u.id]: '' };
		connResults = { ...connResults, [u.id]: [] };
	}
	function closeModal(): void {
		modalUser = null;
	}

	let modalConns = $derived(modalUser ? allConnections(modalUser) : []);
	let modalTotalPages = $derived(Math.max(1, Math.ceil(modalConns.length / modalPer)));
	let modalPageConns = $derived(modalConns.slice((modalPage - 1) * modalPer, modalPage * modalPer));

	let me = $derived($user);
	const birthdayPattern = '\\d{1,2}(-|/)\\d{1,2}';
</script>

{#if !me?.isAdmin}
	<h1>Admin</h1>
	<p class="muted">Admin access required.</p>
{:else}
	<PageHeader title={"Admin"} accent={null} />
	{#if error}<p class="error">{error}</p>{/if}
	{#if actionMsg}<p class="muted" style="color:var(--c-claim); font-weight:600">{actionMsg}</p>{/if}

	<section>
		<h2>Event theme</h2>
		<p class="muted">
			Force a seasonal palette locally for preview. <strong>Auto</strong> follows birthdays/Christmas when
			they are &lt; 30 days away (and Event theming is enabled). Override is stored in this browser only.
		</p>
		<div class="row" role="radiogroup" aria-label="Force event theme">
			{#each [{ v: 'auto', label: 'Auto' }, { v: 'none', label: 'Off' }, { v: 'christmas', label: '🎄 Christmas' }, { v: 'birthday', label: '🎂 Birthday' }] as opt (opt.v)}
				<button
					class={seasonOverride.value === opt.v ? '' : 'secondary'}
					aria-pressed={seasonOverride.value === opt.v}
					onclick={() => setSeasonOverride(opt.v as Season | 'auto')}
				>
					{opt.label}
				</button>
			{/each}
		</div>
		<p class="muted" style="margin:.5rem 0 0">
			Active: <strong>{season.kind}</strong>{#if season.label} · {season.days}d till {season.label}{/if}
			{#if seasonOverride.value !== 'auto'} <span class="badge" style="margin-left:.4em">forced {seasonOverride.value}</span>{/if}
		</p>
	</section>

	<section>
		<h2>Users</h2>
		<p class="muted">
			Users are created automatically the first time they sign in. The last admin
			cannot be demoted or deleted. Showing 25 per page.
		</p>

		<div class="searchbox">
			<Icon name="search" size={15} />
			<input
				class="search"
				type="search"
				placeholder="Search people…"
				bind:value={query}
				oninput={onQueryInput}
				aria-label="Search users"
			/>
		</div>
		{#if loading}<p class="muted">Loading…</p>{/if}

		{#each users as u (u.id)}
			<div class="card admin-card">
				<div class="spread">
					<div class="who">
						<strong>{u.displayName}</strong>
						<span class="muted">@{u.username}</span>
						{#if u.isAdmin}<span class="badge">admin</span>{/if}
					</div>
					<div class="menu-wrap">
						<button
							type="button"
							class="menu-btn"
							aria-label="Options for {u.displayName}"
							aria-haspopup="menu"
							aria-expanded={openMenu === u.id}
							onclick={(e) => {
								e.stopPropagation();
								openMenu = openMenu === u.id ? null : u.id;
							}}
						>
							<Icon name="dots" size={19} />
						</button>
						{#if openMenu === u.id}
							<div class="menu-pop" role="menu" aria-label="Options for {u.displayName}">
								<button
									type="button"
									role="menuitem"
									disabled={u.isAdmin && users.filter((x) => x.isAdmin).length <= 1 && total <= users.length}
									title={u.isAdmin && users.filter((x) => x.isAdmin).length <= 1 && total <= users.length ? 'Cannot demote the last admin' : undefined}
									onclick={() => void toggleAdmin(u)}
								>
									{u.isAdmin ? 'Revoke admin' : 'Make admin'}
								</button>
								<button type="button" role="menuitem" onclick={() => void hardDelete(u)}>
									Delete list
								</button>
								<button
									type="button"
									role="menuitem"
									class="menu-danger"
									disabled={me?.id === u.id}
									title={me?.id === u.id ? 'Cannot delete yourself' : 'Delete user and all data'}
									onclick={() => void deleteUser(u)}
								>
									Delete user
								</button>
							</div>
						{/if}
					</div>
				</div>
				<p class="statline muted">
					<b>{u.itemCount}</b> items · <b>{u.claimedCount}</b> claimed ·
					<b>{allConnections(u).length}</b> connections
				</p>
				<div class="card-foot">
					<form
						class="row"
						onsubmit={(e) => {
							e.preventDefault();
							void saveBirthday(u);
						}}
					>
						<input
							style="max-width: 140px;"
							bind:value={u.birthday}
							pattern={birthdayPattern}
							title="MM-DD"
							placeholder="MM-DD"
							aria-label="Birthday for {u.displayName}"
						/>
						<button class="secondary">Save birthday</button>
						{#if saved === u.id}<span class="muted">Saved.</span>{/if}
					</form>
					<div class="footbtns">
						<button class="secondary" onclick={() => openModal(u)}>Manage connections</button>
						<button class="danger" onclick={() => void forceArchive(u)}>Archive current list</button>
					</div>
				</div>
			</div>
		{:else}
			{#if !loading}
				<p class="muted">No users match “{query}”.</p>
			{/if}
		{/each}
		{#if hasMore || page > 1}
			<div class="pager">
				<button class="secondary" disabled={page <= 1} onclick={() => go(page - 1)}>Prev</button>
				<span class="muted">Page {page} of {Math.max(1, Math.ceil(total / 25))} · {total} users</span>
				<button class="secondary" disabled={!hasMore} onclick={() => go(page + 1)}>Next</button>
			</div>
		{/if}
	</section>

	<dialog
		class="edit-dlg"
		bind:this={connDialogEl}
		onclose={closeModal}
		aria-label="Connections for {modalUser?.displayName ?? ''}"
	>
		{#if modalUser}
			<form class="dlg-card" method="dialog" onsubmit={(e) => e.preventDefault()}>
				<div class="dlg-head">
					<h2>{modalUser.displayName} — {allConnections(modalUser).length} connections</h2>
					<button class="dlg-close" type="button" onclick={closeModal} aria-label="Close">
						<Icon name="x" size={14} />
					</button>
				</div>
				{#if modalConns.length === 0}
					<p class="muted">No connections yet.</p>
				{:else}
					<ul class="conns">
						{#each modalPageConns as cid (cid)}
							<li class="spread">
								<span>{nameOf(cid)}</span>
								<button class="danger" type="button" onclick={() => void disconnect(modalUser!.id, cid)}>Remove</button>
							</li>
						{/each}
					</ul>
					{#if modalTotalPages > 1}
						<div class="pager" style="margin-top:.8rem">
							<button class="secondary" type="button" disabled={modalPage <= 1} onclick={() => modalPage--}>Prev</button>
							<span class="muted">Page {modalPage} of {modalTotalPages}</span>
							<button class="secondary" type="button" disabled={modalPage >= modalTotalPages} onclick={() => modalPage++}>Next</button>
						</div>
					{/if}
				{/if}

				<div class="conn-add">
					<label class="muted" style="font-size:.82rem" for="conn-{modalUser.id}">Add connection</label>
					<div class="searchbox" style="margin:.3rem 0 0">
						<Icon name="search" size={14} />
						<input
							id="conn-{modalUser.id}"
							class="search"
							type="search"
							placeholder="Search to connect…"
							bind:value={connQueries[modalUser.id]}
							oninput={() => onConnInput(modalUser!)}
							aria-label="Search to connect {modalUser.displayName}"
							autocomplete="off"
						/>
						{#if connLoading[modalUser.id]}<span class="muted" style="font-size:.8rem">…</span>{/if}
					</div>
					{#if connResults[modalUser.id]?.length}
						<ul class="suggest">
							{#each connResults[modalUser.id] as cand (cand.id)}
								<li>
									<button class="suggest-btn" type="button" onclick={() => void addConnection(modalUser!.id, cand.id)}>
										<strong>{cand.displayName}</strong> <span class="muted">@{cand.username}</span>
									</button>
								</li>
							{/each}
						</ul>
					{:else if connQueries[modalUser.id]?.trim() && !connLoading[modalUser.id]}
						<p class="muted" style="font-size:.82rem; margin:.3rem 0 0">No matches</p>
					{/if}
				</div>
			</form>
		{/if}
	</dialog>

	<ConfirmDialog
		open={confirmState !== null}
		title={confirmState?.title ?? ''}
		message={confirmState?.message ?? ''}
		confirmLabel={confirmState?.confirmLabel}
		danger={confirmState?.danger}
		confirmText={confirmState?.confirmText}
		onconfirm={confirmState?.action}
		oncancel={() => (confirmState = null)}
	/>
{/if}

<style>
	.admin-card { overflow: visible; }
	.searchbox {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.75rem 0 0.9rem;
		color: var(--muted);
	}
	.search {
		max-width: 260px;
	}
	.who {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		min-width: 0;
	}
	.statline {
		margin: 0.45rem 0 0;
		font-size: 0.85rem;
	}
	.statline b {
		color: var(--ink);
		font-variant-numeric: tabular-nums;
	}
	.card-foot {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
		margin-top: 0.75rem;
		padding-top: 0.65rem;
		border-top: 1px solid var(--line);
	}
	.footbtns {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		align-items: center;
	}
	.conns {
		list-style: none;
		margin: 0.6rem 0 0;
		padding: 0;
		display: grid;
		gap: 0.35rem;
	}
	.conns li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.6rem;
		padding: 0.3rem 0;
		border-bottom: 1px dashed var(--line);
	}
	.conns li:last-child {
		border-bottom: none;
	}
	.pager {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 0.65rem;
		margin-top: 1.25rem;
	}
	.conn-add {
		margin-top: .9rem;
		padding-top: .7rem;
		border-top: 1px solid var(--line);
	}
	.suggest {
		list-style: none;
		margin: .4rem 0 0;
		padding: 0;
		border: 1px solid var(--line);
		border-radius: .5rem;
		overflow: hidden;
		max-width: 320px;
		background: color-mix(in srgb, var(--solid) 92%, transparent);
		backdrop-filter: blur(18px);
	}
	.suggest li { border-bottom: 1px solid var(--line); }
	.suggest li:last-child { border-bottom: none; }
	.suggest-btn {
		width: 100%;
		text-align: left;
		background: transparent;
		border: none;
		color: var(--ink);
		padding: .5rem .6rem;
		cursor: pointer;
		display: flex;
		gap: .4rem;
		align-items: center;
		box-shadow: none;
	}
	.suggest-btn:hover { background: var(--line); }

	.menu-wrap {
		position: relative;
		flex-shrink: 0;
	}
	.menu-btn {
		width: 44px;
		height: 44px;
		padding: 0;
		border-radius: 14px;
		border: none;
		background: color-mix(in srgb, var(--c-edit) 15%, transparent);
		color: var(--c-edit);
		box-shadow: none;
		display: grid;
		place-items: center;
		cursor: pointer;
	}
	.menu-btn:not(:disabled):hover { filter: brightness(1.08); }
	.menu-btn[aria-expanded="true"] {
		background: color-mix(in srgb, var(--c-edit) 24%, transparent);
		border: 1px solid color-mix(in srgb, var(--acc) 45%, var(--line));
		color: var(--acc-ink);
	}
	.menu-pop {
		position: absolute;
		top: 48px;
		right: 0;
		z-index: 30;
		min-width: 190px;
		background: color-mix(in srgb, var(--solid) 96%, transparent);
		border: 1px solid var(--line);
		border-radius: 14px;
		box-shadow: 0 18px 44px -18px rgba(0, 0, 0, .45);
		backdrop-filter: blur(24px);
		padding: .35rem;
		display: grid;
		gap: .15rem;
	}
	.menu-pop button {
		width: 100%;
		text-align: left;
		border: none;
		background: none;
		color: var(--ink);
		font-size: .88rem;
		font-weight: 600;
		padding: .5rem .7rem;
		border-radius: 10px;
		box-shadow: none;
	}
	.menu-pop button:hover:not(:disabled) { background: color-mix(in srgb, var(--acc) 12%, var(--surface)); }
	.menu-pop button:disabled { opacity: .45; cursor: not-allowed; }
	.menu-pop button.menu-danger { color: #F87171; }
	.menu-pop button.menu-danger:hover:not(:disabled) { background: rgba(248, 113, 113, .1); }
</style>