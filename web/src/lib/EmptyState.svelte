<script lang="ts">
	import Icon from './Icon.svelte';
	import type { Snippet } from 'svelte';

	let {
		icon = 'gift',
		title,
		hint = '',
		ready = true,
		children
	}: { icon?: string; title: string; hint?: string; ready?: boolean; children?: Snippet } = $props();
</script>

<div class="empty" data-ready={ready ? 'true' : 'false'} data-testid="empty-state">
	<div class="art" aria-hidden="true">
		<span class="ring"></span>
		<span class="halo"></span>
		<span class="main"><Icon name={icon} size={34} /></span>
		<span class="spark s1"><Icon name="sparkle" size={11} /></span>
		<span class="spark s2"><Icon name="sparkle" size={8} /></span>
		<span class="spark s3"><Icon name="heart" size={9} /></span>
	</div>
	<p class="t">{title}</p>
	{#if hint}<p class="h">{hint}</p>{/if}
	{@render children?.()}
</div>

<style>
	.empty {
		text-align: center;
		padding: 2.4rem 1rem 2.2rem;
		border: 1.5px dashed color-mix(in srgb, var(--acc) 28%, var(--line));
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--surface) 55%, transparent);
		margin: 0.35rem 0 0.85rem;
		overflow: hidden;
		transition:
			opacity 320ms cubic-bezier(0.2, 0.8, 0.25, 1),
			transform 320ms cubic-bezier(0.2, 0.8, 0.25, 1),
			max-height 320ms cubic-bezier(0.2, 0.8, 0.25, 1),
			margin 320ms cubic-bezier(0.2, 0.8, 0.25, 1),
			padding 320ms cubic-bezier(0.2, 0.8, 0.25, 1),
			border-width 320ms cubic-bezier(0.2, 0.8, 0.25, 1),
			visibility 320ms;
	}
	.empty[data-ready='false'] {
		opacity: 0;
		transform: translateY(8px) scale(0.99);
		pointer-events: none;
		visibility: hidden;
		max-height: 0;
		margin: 0;
		padding: 0 1rem;
		border-width: 0;
	}
	.empty[data-ready='true'] {
		opacity: 1;
		transform: none;
		visibility: visible;
		max-height: 400px;
	}
	.art {
		position: relative;
		width: 92px;
		height: 92px;
		margin: 0 auto 0.8rem;
		display: grid;
		place-items: center;
	}
	.main {
		position: relative;
		z-index: 2;
		display: grid;
		place-items: center;
		width: 64px;
		height: 64px;
		border-radius: 22px;
		color: #fff;
		background-image: linear-gradient(100deg, var(--g1), var(--g2), var(--g3), var(--g1));
		background-size: 400% 100%;
		animation:
			grad-cycle 14s ease infinite,
			gift-float 3.2s ease-in-out infinite;
		box-shadow: 0 14px 30px -12px var(--glow);
	}
	.ring {
		position: absolute;
		inset: 0;
		border-radius: 50%;
		border: 1.5px dashed color-mix(in srgb, var(--acc) 45%, transparent);
		animation: slow-spin 16s linear infinite;
	}
	.halo {
		position: absolute;
		inset: 10px;
		border-radius: 50%;
		background: radial-gradient(circle, color-mix(in srgb, var(--acc) 20%, transparent), transparent 70%);
		animation: halo-pulse 3.2s ease-in-out infinite;
	}
	.spark {
		position: absolute;
		color: var(--acc-ink);
		animation: twinkle 2.6s ease-in-out infinite;
	}
	.s1 {
		top: 2px;
		right: 6px;
		animation-delay: 0s;
	}
	.s2 {
		bottom: 6px;
		left: 4px;
		animation-delay: 0.9s;
	}
	.s3 {
		top: 14px;
		left: -2px;
		animation-delay: 1.7s;
	}
	.t {
		margin: 0;
		font-weight: 700;
		font-size: 1rem;
	}
	.h {
		margin: 0.3rem auto 0;
		max-width: 34ch;
		color: var(--muted);
		font-size: 0.86rem;
		line-height: 1.5;
	}
	@keyframes slow-spin {
		to {
			transform: rotate(360deg);
		}
	}
	@keyframes halo-pulse {
		0%,
		100% {
			transform: scale(0.9);
			opacity: 0.55;
		}
		50% {
			transform: scale(1.15);
			opacity: 1;
		}
	}
	@keyframes twinkle {
		0%,
		100% {
			transform: scale(0.55) rotate(-12deg);
			opacity: 0.25;
		}
		50% {
			transform: scale(1.15) rotate(10deg);
			opacity: 1;
		}
	}
</style>
