<script lang="ts">
	import { user, loaded, signOut } from '../lib/auth.js';
	import { api } from '../lib/api.js';
	import { theme, prefs, setTheme, setStyle, setEvents, setUseOwnerTheme } from '../lib/theme.svelte.js';
	import type { ThemeChoice, ThemeStyle } from '../lib/theme.svelte.js';
	import PageHeader from '../lib/PageHeader.svelte';
	import Icon from '../lib/Icon.svelte';
	import { copyToClipboard } from '../lib/listText.js';

	const styleOptions: ReadonlyArray<{ value: ThemeStyle; label: string; hint: string }> = [
		{ value: 'aurora', label: 'Aurora', hint: 'Cool' },
		{ value: 'kinetic', label: 'Kinetic', hint: 'Minimal' },
		{ value: 'aurora-warm', label: 'Warm', hint: 'Sunset' },
		{ value: 'ocean', label: 'Ocean', hint: 'Blue' },
		{ value: 'forest', label: 'Forest', hint: 'Green' },
		{ value: 'berry', label: 'Berry', hint: 'Plum' },
		{ value: 'gold', label: 'Gold', hint: 'Yellow' },
		{ value: 'ruby', label: 'Ruby', hint: 'Red' },
		{ value: 'teal', label: 'Teal', hint: 'Aqua' },
		{ value: 'dino', label: 'Dino', hint: '🦕 Jungle' },
		{ value: 'dogs', label: 'Dogs', hint: '🐾 Paws' }
	];

	async function pickStyle(style: ThemeStyle): Promise<void> {
		setStyle(style);
		// keep auth store in sync so App's server-style sync doesn't revert us
		user.update((u) => (u ? { ...u, themeStyle: style } : u));
		try {
			await api('/api/profile', { method: 'PATCH', body: { themeStyle: style } });
		} catch {
			// offline or error — local choice already applied
		}
	}

	const themeOptions: ReadonlyArray<{ value: ThemeChoice; label: string }> = [
		{ value: 'system', label: 'System' },
		{ value: 'light', label: 'Light' },
		{ value: 'dark', label: 'Dark' }
	];

	let birthday = $state('');
	let birthState = $state<'idle' | 'saving' | 'done' | 'error'>('idle');

	$effect(() => {
		const u = $user;
		if (u) {
			birthday = u.birthday;
		}
	});

	async function saveBirthday(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (birthState === 'saving') return;
		birthState = 'saving';
		try {
			await api('/api/profile', { method: 'PATCH', body: { birthday } });
			user.update((u) => (u ? { ...u, birthday } : u));
			birthState = 'done';
			setTimeout(() => (birthState = 'idle'), 1500);
		} catch {
			birthState = 'error';
			setTimeout(() => (birthState = 'idle'), 2000);
		}
	}
	const birthdayPattern = '\\d{1,2}(-|/)\\d{1,2}|\\d{4}-\\d{2}-\\d{2}';

	// ---------- connect with others ----------
	let shareEnabled = $state(false);
	let shareCode = $state('');
	let shareBusy = $state(false);
	let shareCopied = $state(false);

	$effect(() => {
		const u = $user;
		if (u) {
			shareCode = u.shareCode ?? '';
			shareEnabled = shareCode !== '';
		}
	});

	async function toggleShare(e: Event): Promise<void> {
		const enabled = (e.currentTarget as HTMLInputElement).checked;
		shareBusy = true;
		try {
			const res = await api<{ shareCode: string }>('/api/connect/share', {
				method: 'PUT',
				body: { enabled }
			});
			shareCode = res.shareCode;
			shareEnabled = enabled;
			user.update((u) => (u ? { ...u, shareCode: res.shareCode } : u));
		} catch {
			shareEnabled = !enabled;
			(e.currentTarget as HTMLInputElement).checked = shareEnabled;
		} finally {
			shareBusy = false;
		}
	}

	async function copyCode(): Promise<void> {
		try {
			await copyToClipboard(shareCode);
			shareCopied = true;
			setTimeout(() => (shareCopied = false), 1500);
		} catch {
			// ignore copy failures
		}
	}
</script>

{#if !$loaded}
	<p class="muted">Loading…</p>
{:else if !$user}
	<p class="muted">Sign in required.</p>
{:else}
	<PageHeader owner={$user.displayName} accent="Profile" />

	<div class="card">
		<h2 class="sect">Colour mode</h2>
		<div class="row" role="radiogroup" aria-label="Theme">
			{#each themeOptions as opt (opt.value)}
				<button
					class={theme.choice === opt.value ? '' : 'secondary'}
					onclick={() => setTheme(opt.value)}
					aria-pressed={theme.choice === opt.value}
				>
					{opt.label}
				</button>
			{/each}
		</div>
	</div>

	<div class="card">
		<h2 class="sect">Appearance</h2>
		<p class="lbl">Style</p>
		<div class="style-grid" role="radiogroup" aria-label="Theme style">
			{#each styleOptions as opt (opt.value)}
				<button type="button" class="style-opt" class:on={prefs.style === opt.value} onclick={() => void pickStyle(opt.value)}>
					<span class="swatch sw-{opt.value}"></span>
					<b>{opt.label}</b>
					<span class="hint">{opt.hint}</span>
				</button>
			{/each}
		</div>

		<label class="event-row" for="eventsToggle">
			<input id="eventsToggle" type="checkbox" checked={prefs.events} onchange={(e) => setEvents(e.currentTarget.checked)} />
			<span>
				<b>Event theming</b>
				<span class="hint">Festive snow &amp; palettes when your birthday or Christmas is less than a month away</span>
			</span>
		</label>

		<label class="event-row" for="useOwnerToggle">
			<input id="useOwnerToggle" type="checkbox" checked={prefs.useOwnerTheme} onchange={(e) => setUseOwnerTheme(e.currentTarget.checked)} />
			<span>
				<b>Show other’s theme</b>
				<span class="hint">When viewing someone else’s list, use their selected theme (your light/dark choice is kept)</span>
			</span>
		</label>
	</div>

	<div class="card">
		<h2 class="sect">Connect with others</h2>
		<p class="muted">Share your code so others can connect to you, or use someone else’s code on the Lists page.</p>
		<label class="event-row" for="shareToggle">
			<input
				id="shareToggle"
				type="checkbox"
				checked={shareEnabled}
				disabled={shareBusy}
				onchange={(e) => void toggleShare(e)}
			/>
			<span>
				<b>Enable others to connect to me</b>
				<span class="hint">Turning this on generates a code others can use to connect. Turning it off revokes the code.</span>
			</span>
		</label>
		{#if shareEnabled && shareCode}
			<div class="share-row">
				<code class="share-code" aria-label="Your share code">{shareCode}</code>
				<button class="secondary copy-btn" type="button" class:copied={shareCopied} onclick={() => void copyCode()} aria-label="Copy share code">
					<Icon name={shareCopied ? 'check' : 'copy'} size={14} />
				</button>
			</div>
		{/if}
	</div>

	<form onsubmit={saveBirthday} class="card">
		<h2 class="sect">Birthday</h2>
		<p class="muted">One week after your birthday (and Christmas), your list archives and unclaimed items carry over.</p>
		<div class="bday-row">
			<input
				class="bday-input"
				placeholder="MM-DD"
				bind:value={birthday}
				required
				pattern={birthdayPattern}
				title="e.g. 05-15 (also accepts 5/15 or 2026-05-15)"
			/>
			<button
				class="bday-btn"
				class:done={birthState === 'done'}
				class:err={birthState === 'error'}
				type="submit"
				disabled={birthState === 'saving'}
				aria-label="Save birthday"
			>
				{#if birthState === 'done'}
					<Icon name="check" size={18} strokeWidth={3} />
				{:else if birthState === 'error'}
					<Icon name="x" size={18} strokeWidth={3} />
				{:else}
					Save
				{/if}
			</button>
		</div>
	</form>

	<div class="card">
		<p class="muted">
			Signed in as @{$user.username}{#if $user.isAdmin}&nbsp;· admin{/if}
		</p>
		<button class="secondary" onclick={() => void signOut()}>Sign out</button>
	</div>
{/if}

<style>
	h2.sect { font-size: 1.02rem; margin: 0 0 0.6rem; }
	p.lbl { font-size: 0.68rem; font-weight: 700; letter-spacing: 0.09em; text-transform: uppercase;
		color: var(--muted); margin: 0.9rem 0 0.4rem; }
	button.on { outline: 2px solid var(--acc); outline-offset: 1px; }
	.style-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.55rem; }
	.style-opt { display: flex; flex-direction: column; align-items: flex-start; gap: 0.15rem;
		background: var(--surface); border: 1.5px solid var(--line); border-radius: 14px;
		padding: 0.6rem 0.7rem; color: var(--ink); box-shadow: none; text-align: left; }
	.style-opt.on { border-color: var(--acc);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--acc) 18%, transparent); }
	.swatch { width: 100%; height: 26px; border-radius: 8px; margin-bottom: 0.35rem; }
	.sw-aurora { background: linear-gradient(100deg, #818cf8, #e879f9 50%, #22d3ee); }
	.sw-aurora-warm { background: linear-gradient(100deg, #f59e0b, #fb7185 55%, #f472b6); }
	.sw-kinetic { background: linear-gradient(100deg, #fafafa 60%, #4f46e5); }
	.sw-ocean { background: linear-gradient(100deg, #0ea5e9, #22d3ee 45%, #2563eb); }
	.sw-forest { background: linear-gradient(100deg, #16a34a, #34d399 50%, #a3e635); }
	.sw-berry { background: linear-gradient(100deg, #9333ea, #ec4899 50%, #e879f9); }
	.sw-gold { background: linear-gradient(100deg, #f59e0b, #fde047 50%, #a3e635); }
	.sw-ruby { background: linear-gradient(100deg, #dc2626, #f43f5e 50%, #fb923c); }
	.sw-teal { background: linear-gradient(100deg, #14b8a6, #2dd4bf 45%, #38bdf8); }
	.sw-dino { background: linear-gradient(100deg, #3f6212, #84cc16 45%, #facc15 90%); }
	.sw-dogs { background: linear-gradient(100deg, #92400e, #d97706 45%, #fbbf24); }
	.hint { font-size: 0.72rem; color: var(--muted); }
	.event-row { display: flex; align-items: flex-start; gap: 0.65rem; margin-top: 1.1rem; cursor: pointer; }
	.event-row input { width: 20px; height: 20px; accent-color: var(--acc); margin-top: 0.15rem; }
	.share-row { display: flex; align-items: center; gap: 0.6rem; margin-top: 1rem; }
	.share-code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 1.15rem; font-weight: 700; letter-spacing: 0.18em;
		color: var(--acc-ink);
		background: color-mix(in srgb, var(--solid) 55%, transparent);
		border: 1.5px dashed color-mix(in srgb, var(--acc) 40%, var(--line));
		border-radius: 12px; padding: 0.6rem 0.9rem;
	}
	.copy-btn { width: 46px; height: 46px; padding: 0; display: grid; place-items: center; flex-shrink: 0; }
	.copy-btn.copied { background-color: #15803d; border-color: #15803d; color: #fff; }
	.bday-row { display: grid; grid-template-columns: 1fr auto; gap: 0.5rem; margin-top: 0.9rem; }
	.bday-input { font-size: 1rem; }
	.bday-btn {
min-width: 74px; height: 46px; padding: 0 1.1rem;
		display: grid; place-items: center; position: relative;
		transition: transform 170ms cubic-bezier(.2,.8,.25,1), background-color 300ms ease, box-shadow .25s ease;
	}
	.bday-btn:not(:disabled):hover { transform: translateY(-1px); filter: brightness(1.07); }
	.bday-btn:not(:disabled):active { transform: scale(.96); }
	.bday-btn.done {
		background-image: none; background-color: #15803d;
		box-shadow: 0 6px 24px -6px rgba(52, 211, 153, 0.5);
	}
	.bday-btn.err {
		background-image: none; background-color: #dc2626;
		box-shadow: 0 6px 24px -6px rgba(248, 113, 113, 0.5);
		animation: shake-x 380ms cubic-bezier(.36,.07,.19,.97) 3;
	}
	@keyframes shake-x {
		0%, 100% { transform: translateX(0); }
		25% { transform: translateX(3px); }
		75% { transform: translateX(-3px); }
	}
</style>
