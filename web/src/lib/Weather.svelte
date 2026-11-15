<script lang="ts">
	import { season, prefs } from './theme.svelte.js';

	let flakes: Array<{ left: string; s: string; t: string; d: string; sway: string }> = $derived.by(
		() => {
			if (prefs.reduceVfx || season.kind !== 'christmas') return [];
			return Array.from({ length: 46 }, () => ({
				left: Math.random() * 100 + '%',
				s: 3 + Math.random() * 4 + 'px',
				t: 8 + Math.random() * 9 + 's',
				d: -Math.random() * 16 + 's',
				sway: Math.random() * 60 - 30 + 'px'
			}));
		}
	);

	let pieces: Array<{ left: string; c: string; t: string; d: string; r: string }> = $derived.by(
		() => {
			if (prefs.reduceVfx || season.kind !== 'birthday') return [];
			const cols: string[] = ['#8B5CF6', '#EC4899', '#F59E0B', '#22D3EE', '#34D399'];
			return Array.from({ length: 34 }, (_, i) => ({
				left: Math.random() * 100 + '%',
				c: cols[i % cols.length] ?? '#FBBF24',
				t: 7 + Math.random() * 8 + 's',
				d: -Math.random() * 13 + 's',
				r: Math.random() * 720 - 360 + 'deg'
			}));
		}
	);
</script>

<div class="weather" aria-hidden="true" hidden={season.kind === 'none'}>
	{#if season.kind === 'christmas'}
		{#each flakes as f, i (i)}
			<i
				class="snow"
				style:left={f.left}
				style:--s={f.s}
				style:--t={f.t}
				style:--d={f.d}
				style:--sway={f.sway}
			></i>
		{/each}
	{:else if season.kind === 'birthday'}
		{#each pieces as p, i (i)}
			<i class="cf" style:left={p.left} style:--c={p.c} style:--t={p.t} style:--d={p.d} style:--r={p.r}></i>
		{/each}
	{/if}
</div>
