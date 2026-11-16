<script lang="ts">
	import Icon from './Icon.svelte';

	let {
		x,
		y,
		onedit,
		onregenerate,
		onregenerateName,
		onclose
	}: {
		x: number;
		y: number;
		onedit: () => void;
		onregenerate: () => void;
		onregenerateName: () => void;
		onclose: () => void;
	} = $props();

	let el: HTMLDivElement | undefined = $state();
	let size = $state({ w: 0, h: 0 });

	// Clamp the menu inside the viewport once it has been measured.
	$effect(() => {
		if (!el) return;
		const r = el.getBoundingClientRect();
		size = { w: r.width, h: r.height };
	});

	const left = $derived(Math.max(8, Math.min(x, window.innerWidth - size.w - 8)));
	const top = $derived(Math.max(8, Math.min(y, window.innerHeight - size.h - 8)));

	// Dismiss on outside click, Escape, scroll or resize.
	$effect(() => {
		function onDown(e: MouseEvent): void {
			if (el && !el.contains(e.target as Node)) onclose();
		}
		function onKey(e: KeyboardEvent): void {
			if (e.key === 'Escape') onclose();
		}
		window.addEventListener('mousedown', onDown, true);
		window.addEventListener('keydown', onKey);
		window.addEventListener('resize', onclose);
		window.addEventListener('scroll', onclose, true);
		return () => {
			window.removeEventListener('mousedown', onDown, true);
			window.removeEventListener('keydown', onKey);
			window.removeEventListener('resize', onclose);
			window.removeEventListener('scroll', onclose, true);
		};
	});
</script>

<div
	class="item-menu"
	role="menu"
	aria-label="Item actions"
	bind:this={el}
	style="left:{left}px; top:{top}px"
>
	<button role="menuitem" type="button" onclick={onedit}>
		<Icon name="edit" size={15} /> Edit item…
	</button>
	<button role="menuitem" type="button" onclick={onregenerate}>
		<Icon name="undo" size={15} /> Regenerate preview
	</button>
	<button role="menuitem" type="button" onclick={onregenerateName}>
		<Icon name="undo" size={15} /> Regenerate &amp; update name
	</button>
</div>

<style>
	.item-menu {
		position: fixed;
		z-index: 60;
		min-width: 210px;
		background: color-mix(in srgb, var(--solid) 96%, transparent);
		border: 1px solid var(--line);
		border-radius: 14px;
		box-shadow: 0 18px 44px -18px rgba(0, 0, 0, 0.45);
		backdrop-filter: blur(24px);
		padding: 0.35rem;
		display: grid;
		gap: 0.15rem;
	}
	.item-menu button {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		text-align: left;
		border: none;
		background: none;
		color: var(--ink);
		font: inherit;
		font-size: 0.88rem;
		font-weight: 600;
		padding: 0.5rem 0.7rem;
		border-radius: 10px;
		box-shadow: none;
		cursor: pointer;
	}
	.item-menu button:hover {
		background: color-mix(in srgb, var(--acc) 12%, var(--surface));
	}
</style>
