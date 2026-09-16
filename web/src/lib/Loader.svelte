<script lang="ts">
	interface Props {
		/** Min height of the centring box. Defaults to a viewport-filling box. */
		minHeight?: string;
		/** Accessible label. */
		label?: string;
	}

	let { minHeight = 'calc(100dvh - var(--nav-h) - 9rem)', label = 'Loading' }: Props = $props();
</script>

<div class="loader-box" style={`min-height:${minHeight}`} role="status" aria-label={label}>
	<span class="loader" aria-hidden="true"></span>
</div>

<style>
	.loader-box {
		position: relative;
		width: 100%;
	}

	/* Centered bouncy-dot loader; colours follow the theme. */
	.loader {
		--color-1: var(--acc, #ff3d00);
		--color-2: var(--ink, #fff);
		--size: 1px;
		width: calc(50 * var(--size));
		height: calc(165 * var(--size));
		position: absolute;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
	}
	.loader::before {
		content: '';
		position: absolute;
		left: 50%;
		top: 0;
		transform: translate(-50%, 0);
		width: calc(16 * var(--size));
		height: calc(16 * var(--size));
		background: var(--color-1);
		border-radius: 50%;
		animation: bounce 2.86s linear infinite;
	}
	.loader::after {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		margin: auto;
		height: calc(48 * var(--size));
		width: calc(48 * var(--size));
		background: var(--color-2);
		border-radius: calc(4 * var(--size));
		animation: rotate 2.86s linear infinite;
	}
	@keyframes bounce {
		0%,
		50%,
		100% {
			transform: translate(-50%, 0);
			height: calc(20 * var(--size));
		}
		20% {
			transform: translate(-25%, calc(85 * var(--size)));
			height: calc(28 * var(--size));
		}
		25% {
			transform: translate(-25%, calc(110 * var(--size)));
			height: calc(12 * var(--size));
		}
		70% {
			transform: translate(-75%, calc(85 * var(--size)));
			height: calc(28 * var(--size));
		}
		75% {
			transform: translate(-75%, calc(108 * var(--size)));
			height: calc(12 * var(--size));
		}
	}
	@keyframes rotate {
		0%,
		50%,
		100% {
			transform: rotate(0deg);
		}
		25% {
			transform: rotate(90deg);
		}
		75% {
			transform: rotate(-90deg);
		}
	}
</style>