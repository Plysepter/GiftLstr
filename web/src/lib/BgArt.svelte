<script lang="ts">
	// Background art for the special styles (dino / pawprint).
	// Rendered from the hand-made images in web/assets; which theme shows is
	// decided in app.css via [data-style], and the glow (drop-shadow + radial
	// halos) is applied there too for each light/dark.
	import blueDino from '../../assets/blue-dino.webp';
	import greenDino from '../../assets/green-dino.webp';
	import dogImg from '../../assets/dog.webp';
</script>

<div class="bg-art" aria-hidden="true">
	<!-- ── dino ── -->
	<img class="art a-dino" src={greenDino} alt="" loading="lazy" draggable="false" />
	<img class="art a-dino-blue" src={blueDino} alt="" loading="lazy" draggable="false" />

	<!-- ── pawprint ── -->
	<svg class="art a-paw a-paw-1" viewBox="0 0 120 100">
		<g fill="#FBBF24">
			<ellipse cx="60" cy="66" rx="24" ry="20" />
			<ellipse cx="33" cy="42" rx="9" ry="11" />
			<ellipse cx="60" cy="31" rx="9" ry="11" />
			<ellipse cx="87" cy="42" rx="9" ry="11" />
		</g>
	</svg>
	<svg class="art a-paw a-paw-2" viewBox="0 0 120 100">
		<g fill="#D97706">
			<ellipse cx="60" cy="66" rx="24" ry="20" />
			<ellipse cx="33" cy="42" rx="9" ry="11" />
			<ellipse cx="60" cy="31" rx="9" ry="11" />
			<ellipse cx="87" cy="42" rx="9" ry="11" />
		</g>
	</svg>
	<img class="art a-dog" src={dogImg} alt="" loading="lazy" draggable="false" />
</div>
