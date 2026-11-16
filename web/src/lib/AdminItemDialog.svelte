<script lang="ts">
	import { api } from './api.js';
	import type { Item, PageMetadata } from './types.js';
	import { normalizeItemURL } from './types.js';
	import { invalidateMetadata } from './metadata.js';
	import Icon from './Icon.svelte';
	import { untrack } from 'svelte';

	let {
		item,
		onclose,
		onsaved,
		onerror
	}: {
		item: Item;
		onclose: () => void;
		onsaved?: (() => void) | undefined;
		onerror?: ((message: string) => void) | undefined;
	} = $props();

	// The dialog is mounted per item, so these capture the item's values once.
	let name = $state(untrack(() => item.title));
	let link = $state(untrack(() => item.url));
	let notes = $state(untrack(() => item.notes));
	let regenerate = $state(false);
	let updateName = $state(false);
	let busy = $state(false);
	let dialogError = $state('');
	let el: HTMLDialogElement | undefined = $state();

	// "Update name from page title" only makes sense alongside a refresh.
	$effect(() => {
		if (!regenerate) updateName = false;
	});

	$effect(() => {
		if (el && !el.open) el.showModal();
	});

	async function save(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (busy) return;
		const trimmed = name.trim();
		if (!trimmed) {
			dialogError = 'Name cannot be empty.';
			return;
		}
		const normalized = normalizeItemURL(link);
		if (normalized === null) {
			dialogError = 'URL must be a valid http(s) link, e.g. https://example.com';
			return;
		}
		busy = true;
		dialogError = '';
		try {
			const res = await api<{ ok: boolean; metadata?: PageMetadata; metadataError?: string }>(
				`/api/admin/items/${item.id}`,
				{
					method: 'PATCH',
					body: {
						title: trimmed,
						url: normalized,
						notes,
						regenerate,
						updateName: regenerate && updateName
					}
				}
			);
			// Drop the client preview cache so the refreshed card re-fetches.
			if (regenerate || item.url !== normalized) invalidateMetadata(normalized);
			if (item.url && item.url !== normalized) invalidateMetadata(item.url);
			if (res.metadataError) {
				onerror?.(`Saved, but the preview could not be refreshed: ${res.metadataError}`);
			}
			onsaved?.();
			onclose();
		} catch (err) {
			dialogError = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}
</script>

<dialog class="edit-dlg" bind:this={el} onclose={onclose} aria-label="Edit item as admin">
	<form class="dlg-card" method="dialog" onsubmit={save}>
		<div class="dlg-head">
			<h2>Edit item</h2>
			<button class="dlg-close" type="button" onclick={onclose} aria-label="Close">
				<Icon name="x" size={14} />
			</button>
		</div>
		<p class="admin-note"><Icon name="shield" size={13} /> Admin edit — this changes the owner's item.</p>

		<label for="aiName">Name</label>
		<input id="aiName" bind:value={name} required autocomplete="off" disabled={busy} />
		<label for="aiUrl">Link</label>
		<input
			id="aiUrl"
			type="url"
			inputmode="url"
			autocomplete="off"
			placeholder="https://…"
			bind:value={link}
			disabled={busy}
		/>
		<label for="aiNotes">Notes</label>
		<textarea id="aiNotes" rows="3" bind:value={notes} disabled={busy}></textarea>

		<label class="chk" for="aiRegen">
			<input id="aiRegen" type="checkbox" bind:checked={regenerate} disabled={busy} />
			Regenerate link preview
		</label>
		<label class="chk" class:disabled={!regenerate} for="aiUpdateName">
			<input id="aiUpdateName" type="checkbox" bind:checked={updateName} disabled={busy || !regenerate} />
			Update name from page title
		</label>

		{#if dialogError}
			<p class="dlg-error" role="alert">{dialogError}</p>
		{/if}

		<div class="dlg-menu">
			<button class="secondary" type="button" onclick={onclose} disabled={busy}>Cancel</button>
			<button type="submit" disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button>
		</div>
	</form>
</dialog>

<style>
	.admin-note {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin: 0 0 0.2rem;
		font-size: 0.76rem;
		color: var(--muted);
	}
	.chk {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		text-transform: none;
		letter-spacing: normal;
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--ink);
		margin-top: 0.85rem;
		cursor: pointer;
	}
	.chk.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.chk input {
		width: auto;
		margin: 0;
	}
	.dlg-error {
		margin: 0.85rem 0 0;
		color: #f87171;
		font-size: 0.85rem;
		font-weight: 600;
	}
</style>
