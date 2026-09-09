<script lang="ts">
	import { normalizeItemURL } from '../types.js';
	import { getMetadata } from '../metadata.js';
	import { confetti } from '../confetti.js';
	import Icon from '../Icon.svelte';

	let {
		onadd,
		isPrivate = false,
		placeholder,
		label
	}: {
		onadd: (item: { title: string; url: string; notes: string }) => Promise<boolean> | boolean | void;
		isPrivate?: boolean;
		placeholder?: string;
		label?: string;
	} = $props();

	let title = $state('');
	let url = $state('');
	let notes = $state('');
	let expanded = $state(false);
	let notesOpen = $state(false);
	let resolving = $state(false);
	let done = $state(false);
	let addBtnEl: HTMLButtonElement | undefined = $state();

	const metaChip = $state<{ hidden: boolean; text: string }>({ hidden: true, text: '' });

	const hasText = $derived(Boolean(title.trim() || url.trim() || notes.trim()));
	const canAdd = $derived(Boolean(title.trim() || (url.trim() && !resolving)));

	function setExpanded(v: boolean): void {
		expanded = v;
		if (!v) {
			notesOpen = false;
			metaChip.hidden = true;
		}
	}

	async function resolveMeta(): Promise<void> {
		const normalized = normalizeItemURL(url);
		if (!normalized) return;
		resolving = true;
		try {
			const m = await getMetadata(normalized);
			if (normalizeItemURL(url) === normalized) {
				if (!title.trim() && m?.title) title = m.title;
				metaChip.text = m?.title || normalized;
				metaChip.hidden = false;
			}
		} catch {
			// No preview available — leave the name as typed.
		} finally {
			resolving = false;
		}
	}

	function onUrlChange(): void {
		void resolveMeta();
	}

	function onNamePaste(e: ClipboardEvent): void {
		const text = e.clipboardData?.getData('text').trim() ?? '';
		if (/^https?:\/\//.test(text)) {
			e.preventDefault();
			url = text;
			title = '';
			setExpanded(true);
			void resolveMeta();
		}
	}

	function submit(e: SubmitEvent): void {
		e.preventDefault();
		if (resolving) return;
		const normalized = normalizeItemURL(url);
		if (normalized === null) return;
		const t = title.trim();
		if (!t && !normalized) return;
		void commit({
			title: t || normalized,
			url: normalized === '' ? '' : normalized,
			notes: notes.trim()
		});
	}

	async function commit(item: { title: string; url: string; notes: string }): Promise<void> {
		// Wait for the add to complete before clearing the form and
		// celebrating, so a rejection (e.g. list full) leaves the typed input
		// intact for the user to correct or retry.
		let ok: boolean;
		try {
			ok = (await onadd(item)) !== false;
		} catch {
			return;
		}
		if (!ok) return;
		if (addBtnEl) {
			const r = addBtnEl.getBoundingClientRect();
			confetti(r.left + r.width / 2, r.top + r.height / 2);
			done = true;
			setTimeout(() => (done = false), 1000);
		}
		title = url = notes = '';
		metaChip.hidden = true;
		setExpanded(false);
	}
</script>

<form class={`composer${isPrivate ? ' is-private' : ''}`} onsubmit={submit}>
	<div class="composer-inner">
		{#if expanded}
			<input
				class="url"
				type="url"
				inputmode="url"
				autocomplete="off"
				placeholder="Paste a link — I’ll fetch the name &amp; preview"
				bind:value={url}
				onchange={onUrlChange}
				aria-label="Link URL"
			/>
			{#if !metaChip.hidden}
				<div class="meta-chip">
					<span class="dot"></span>
					<b>{metaChip.text}</b>
					<span>auto-filled ✓</span>
				</div>
			{/if}
		{/if}
		<div class="namerow">
			<input
				class="name"
				placeholder={placeholder ?? (isPrivate ? 'Add a private idea…' : 'Gift idea…')}
				bind:value={title}
				onfocus={() => setExpanded(true)}
				onpaste={onNamePaste}
				aria-label={label ?? (isPrivate ? 'Private gift idea' : 'Gift idea')}
			/>
			<button
				bind:this={addBtnEl}
				id="addBtn"
				class:done
				type="submit"
				disabled={!canAdd && !done}
				aria-label="Add item"
			>
				<span class="add-icon plus" aria-hidden="true"><Icon name="plus" size={20} strokeWidth={2.5} /></span>
				<span class="add-icon check" aria-hidden="true"><Icon name="check" size={20} strokeWidth={3} /></span>
			</button>
		</div>
		{#if expanded}
			<div class="noterow">
				<button type="button" class="note-toggle" aria-expanded={notesOpen} onclick={() => (notesOpen = !notesOpen)}>
					note <span class="chev">▾</span>
				</button>
				<div id="extraWrap" class:open={notesOpen}>
					<div>
						<textarea class="notes-input" rows="2" placeholder="Notes (optional)" bind:value={notes} aria-label="Notes"></textarea>
					</div>
				</div>
			</div>
		{/if}
	</div>
</form>

<style>
	:global(form.composer.is-private) {
		position: static;
		border-top: none;
		padding: 0.5rem 0 0;
		background: transparent;
	}
	.composer :global(.dot) {
		width: 22px;
		height: 22px;
		border-radius: 7px;
		background: var(--acc);
		flex-shrink: 0;
	}
	input.url {
		font-size: 0.9rem;
		margin-bottom: 0.5rem;
	}
	button.note-toggle,
	:global(button.note-toggle) {
		box-shadow: none;
		background: none;
		border-radius: 8px;
		color: var(--muted);
		padding: 0.15rem 0;
	}
	textarea.notes-input {
		resize: none;
	}
</style>
