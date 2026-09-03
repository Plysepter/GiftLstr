<script lang="ts">
	import Icon from './Icon.svelte';

	let {
		open = false,
		title = 'Are you sure?',
		message = '',
		confirmLabel = 'Confirm',
		danger = false,
		confirmText = '',
		onconfirm,
		oncancel
	}: {
		open?: boolean | undefined;
		title?: string | undefined;
		message?: string | undefined;
		confirmLabel?: string | undefined;
		danger?: boolean | undefined;
		/** When set, the user must type this exact text in an input to enable Confirm. */
		confirmText?: string | undefined;
		onconfirm?: (() => void | Promise<void>) | undefined;
		oncancel?: (() => void) | undefined;
	} = $props();

	let el: HTMLDialogElement | undefined = $state();
	let typed = $state('');

	$effect(() => {
		if (open && el && !el.open) {
			typed = '';
			el.showModal();
		}
		if (!open && el?.open) el.close();
	});

	let busy = $state(false);

	async function confirm(): Promise<void> {
		if (busy || (confirmText && typed !== confirmText)) return;
		busy = true;
		try {
			await onconfirm?.();
		} finally {
			busy = false;
		}
	}

	function cancel(): void {
		if (busy) return;
		oncancel?.();
	}
</script>

<dialog class="edit-dlg" bind:this={el} onclose={oncancel} aria-label={title}>
	<form class="dlg-card" method="dialog" onsubmit={(e) => e.preventDefault()}>
		<div class="dlg-head">
			<h2>{title}</h2>
			<button class="dlg-close" type="button" onclick={cancel} aria-label="Close">
				<Icon name="x" size={14} />
			</button>
		</div>
		<p class="muted" style="margin:0">{message}</p>
		{#if confirmText}
			<label for="confirm-dialog-input">
				Type <strong>{confirmText}</strong> to confirm
			</label>
			<input id="confirm-dialog-input" bind:value={typed} autocomplete="off" disabled={busy} />
		{/if}
		<div class="dlg-menu">
			<button class="secondary" type="button" onclick={cancel} disabled={busy}>Cancel</button>
			<button
				class={danger ? 'danger' : ''}
				type="button"
				disabled={busy || (confirmText ? typed !== confirmText : false)}
				onclick={() => void confirm()}
			>
				{busy ? 'Working…' : confirmLabel}
			</button>
		</div>
	</form>
</dialog>