<script lang="ts">
	import { api } from './api.js';
	import { user } from './auth.js';
	import { BDAY_MONTHS, BDAY_DAYS, HOLIDAY_OPTIONS, formatBday } from './birthday.js';

	let dialogEl: HTMLDialogElement | undefined = $state();
	let month = $state('');
	let day = $state('');
	let holiday = $state('');
	let saving = $state(false);

	let show = $derived($user?.isNew === true);

	$effect(() => {
		if (show && dialogEl && !dialogEl.open) dialogEl.showModal();
		if (!show && dialogEl?.open) dialogEl.close();
	});

	function dismiss(): void {
		if (dialogEl?.open) dialogEl.close();
		user.update((u) => (u ? { ...u, isNew: false } : u));
	}

	async function save(): Promise<void> {
		if (saving) return;
		saving = true;
		try {
			if (month && day) {
				const b = formatBday(month, day);
				await api('/api/profile', {
					method: 'PATCH',
					body: { birthday: b, holidayReset: holiday }
				});
				user.update((u) => (u ? { ...u, birthday: b, holidayReset: holiday, isNew: false } : u));
			} else {
				user.update((u) => (u ? { ...u, isNew: false } : u));
			}
		} catch {
			// keep the dialog open so the user can retry or skip
		} finally {
			saving = false;
		}
	}
</script>

{#if show}
	<dialog
		class="birth-dlg"
		bind:this={dialogEl}
		onclose={dismiss}
		aria-label="Set your birthday"
	>
		<form class="dlg-card" method="dialog" onsubmit={(e) => { e.preventDefault(); void save(); }}>
			<div class="dlg-head">
				<h2>What’s your birthday?</h2>
			</div>
			<p class="muted">
				Your list archives around your birthday and Christmas, carrying unclaimed
				items over to a fresh list. You can add Mother's or Father's Day as a
				third reset date below. This step is optional; you can always set it
				later in your profile.
			</p>
			<div class="bday-selects">
				<select class="birth-select" bind:value={month} aria-label="Birth month">
					{#each BDAY_MONTHS as m}<option value={m.v}>{m.l}</option>{/each}
				</select>
				<select class="birth-select" bind:value={day} aria-label="Birth day">
					<option value=""></option>
					{#each BDAY_DAYS as d}<option value={d}>{d}</option>{/each}
				</select>
			</div>
			<label class="birth-holiday" for="birthHolidaySelect">Celebrate Mothers or Fathers Day</label>
			<select class="birth-select" id="birthHolidaySelect" bind:value={holiday} aria-label="Additional reset holiday">
				{#each HOLIDAY_OPTIONS as o}<option value={o.v}>{o.l}</option>{/each}
			</select>
			<div class="dlg-menu">
				<button class="secondary" type="button" onclick={dismiss}>Skip</button>
				<button type="submit" disabled={saving}>Save</button>
			</div>
		</form>
	</dialog>
{/if}

<style>
	dialog.birth-dlg {
		border: none;
		padding: 0;
		background: transparent;
		max-width: none;
	}
	dialog.birth-dlg .dlg-card {
		padding: 1.2rem 1.3rem 1.1rem;
		max-width: 420px;
		width: calc(100vw - 2.5rem);
		background: var(--solid);
		color: var(--ink);
		border-radius: 22px;
		box-shadow: var(--shadow);
		border: 1px solid var(--line);
	}
	dialog.birth-dlg[open] .dlg-card { animation: dlg-in 300ms cubic-bezier(.2,.9,.3,1.2); }
	dialog.birth-dlg::backdrop { background: rgba(4,6,12,.5); backdrop-filter: blur(6px); }
	.bday-selects { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; margin-top: 1rem; }
	.birth-holiday { display: block; font-size: 0.68rem; font-weight: 700; letter-spacing: 0.09em; text-transform: uppercase; color: var(--muted); margin: 1rem 0 0.35rem; }
	.birth-select { font-size: 1rem; }
</style>