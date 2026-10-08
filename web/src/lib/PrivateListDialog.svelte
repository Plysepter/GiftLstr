<script lang="ts">
	import { api } from './api.js';
	import { BDAY_MONTHS, BDAY_DAYS, formatBday, parseBday } from './birthday.js';
	import type { ThemeStyle } from './theme.svelte.js';
	import Icon from './Icon.svelte';

	const STYLE_LABELS: Record<ThemeStyle, string> = {
		aurora: 'Aurora',
		twilight: 'Twilight',
		kinetic: 'Kinetic',
		oceanic: 'Oceanic',
		forest: 'Forest',
		faerie: 'Faerie',
		daisies: 'Daisies',
		ruby: 'Ruby',
		lagoon: 'Lagoon',
		dino: 'Dino',
		pawprint: 'Pawprint'
	};

	let {
		open = false,
		listId = null,
		initial = null,
		onclose,
		onsaved
	}: {
		open?: boolean;
		/** The list being edited, or null when creating a new one. */
		listId?: string | null;
		/** Seed values (empty when creating); theme applies when editing. */
		initial?: { firstName: string; lastName: string; birthday: string; theme?: ThemeStyle } | null;
		onclose: () => void;
		onsaved?: (id: string) => void;
	} = $props();

	const STYLES = Object.keys(STYLE_LABELS) as ThemeStyle[];

	/** A new list starts on a random theme; editing keeps the list's own. */
	function randomStyle(): ThemeStyle {
		return STYLES[Math.floor(Math.random() * STYLES.length)] ?? 'aurora';
	}

	let firstName = $state('');
	let lastName = $state('');
	let birthMonth = $state('');
	let birthDay = $state('');
	let theme = $state<ThemeStyle>('aurora');
	let saving = $state(false);
	let err = $state('');
	let dlg: HTMLDialogElement | undefined = $state();

	$effect(() => {
		if (open && dlg && !dlg.open) {
			firstName = initial?.firstName ?? '';
			lastName = initial?.lastName ?? '';
			const bday = parseBday(initial?.birthday ?? '');
			birthMonth = bday.m;
			birthDay = bday.d;
			theme = listId !== null ? (initial?.theme ?? 'aurora') : randomStyle();
			err = '';
			dlg.showModal();
		}
		if (!open && dlg?.open) dlg.close();
	});

	const canSave = $derived(
		firstName.trim() !== '' && lastName.trim() !== '' && birthMonth !== '' && birthDay !== ''
	);

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (!canSave || saving) return;
		saving = true;
		err = '';
		const body = {
			firstName: firstName.trim(),
			lastName: lastName.trim(),
			birthday: formatBday(birthMonth, birthDay),
			theme
		};
		try {
			const id =
				listId ??
				(await api<{ id: string }>('/api/private-lists', { method: 'POST', body })).id;
			if (listId) await api(`/api/private-lists/${listId}`, { method: 'PATCH', body });
			onclose();
			onsaved?.(id);
		} catch (e2) {
			err = e2 instanceof Error ? e2.message : String(e2);
		} finally {
			saving = false;
		}
	}

	const isEditing = $derived(listId !== null);
</script>

<dialog
	class="edit-dlg pl-dlg"
	bind:this={dlg}
	onclose={onclose}
	aria-label={isEditing ? 'Edit private list' : 'New private list'}
>
	<form class="dlg-card" method="dialog" onsubmit={submit}>
		<div class="dlg-head">
			<h2>{isEditing ? 'Edit list' : 'New private list'}</h2>
			<button class="dlg-close" type="button" onclick={onclose} aria-label="Close">
				<Icon name="x" size={14} />
			</button>
		</div>
		<p class="muted" style="margin:0">A wishlist for one person — only you can see it.</p>
		<label for="plFirst">First name</label>
		<input id="plFirst" bind:value={firstName} required autocomplete="off" />
		<label for="plLast">Last name</label>
		<input id="plLast" bind:value={lastName} required autocomplete="off" />
		<label for="plMonth">Birthday</label>
		<div class="bday-selects">
			<select id="plMonth" class="bday-select" bind:value={birthMonth} aria-label="Birth month">
				{#each BDAY_MONTHS as m}<option value={m.v}>{m.l}</option>{/each}
			</select>
			<select class="bday-select" bind:value={birthDay} aria-label="Birth day">
				<option value=""></option>
				{#each BDAY_DAYS as d}<option value={d}>{d}</option>{/each}
			</select>
		</div>
		<div class="theme-lbl">Theme</div>
		<div class="theme-grid" role="radiogroup" aria-label="Theme">
			{#each Object.entries(STYLE_LABELS) as [value, label] (value)}
				<button
					type="button"
					class="theme-opt"
					class:sel={theme === value}
					aria-pressed={theme === value}
					onclick={() => (theme = value as ThemeStyle)}
				>
					<span class="swatch" data-style={value} aria-hidden="true"></span>
					{label}
				</button>
			{/each}
		</div>
		{#if err}<p class="error" role="alert">{err}</p>{/if}
		<div class="dlg-menu">
			<button class="secondary" type="button" onclick={onclose}>Cancel</button>
			<button type="submit" disabled={!canSave || saving}>
				{isEditing ? 'Save changes' : 'Create list'}
			</button>
		</div>
	</form>
</dialog>

<style>
	.bday-selects {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.5rem;
	}
	.bday-select {
		font-size: 1rem;
	}
	.dlg-card .error {
		margin: 0.65rem 0 0;
	}
	.theme-lbl {
		display: block;
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.09em;
		text-transform: uppercase;
		color: var(--muted);
		margin: 0.75rem 0 0.3rem;
	}
	.theme-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr));
		gap: 0.45rem;
	}
	.theme-opt {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.4rem 0.6rem;
		border-radius: 12px;
		border: 1px solid var(--line);
		background: var(--solid);
		color: var(--ink);
		font-size: 0.85rem;
		cursor: pointer;
		transition:
			border-color 0.15s ease,
			transform 0.15s ease,
			box-shadow 0.2s ease;
	}
	.theme-opt:hover {
		border-color: var(--acc);
		transform: translateY(-1px);
	}
	.theme-opt.sel {
		border-color: var(--acc);
		box-shadow: 0 0 0 1px var(--acc);
	}
	.swatch {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		flex: 0 0 auto;
		background: var(--acc);
	}
	.swatch[data-style='aurora'] {
		background: linear-gradient(135deg, #ec4899, #8b5cf6);
	}
	.swatch[data-style='twilight'] {
		background: linear-gradient(135deg, #6366f1, #d946ef);
	}
	.swatch[data-style='kinetic'] {
		background: linear-gradient(135deg, #f97316, #ef4444);
	}
	.swatch[data-style='oceanic'] {
		background: linear-gradient(135deg, #0ea5e9, #059669);
	}
	.swatch[data-style='forest'] {
		background: linear-gradient(135deg, #10b981, #84cc16);
	}
	.swatch[data-style='faerie'] {
		background: linear-gradient(135deg, #a855f7, #ec4899);
	}
	.swatch[data-style='daisies'] {
		background: linear-gradient(135deg, #fde047, #fb923c);
	}
	.swatch[data-style='ruby'] {
		background: linear-gradient(135deg, #e11d48, #7f1d1d);
	}
	.swatch[data-style='lagoon'] {
		background: linear-gradient(135deg, #06b6d4, #3b82f6);
	}
	.swatch[data-style='dino'] {
		background: linear-gradient(135deg, #84cc16, #166534);
	}
	.swatch[data-style='pawprint'] {
		background: linear-gradient(135deg, #f59e0b, #92400e);
	}
</style>