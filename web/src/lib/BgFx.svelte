<script lang="ts">
	// Coloured blobs background from F1 Aurora prototype (shared-f_PG5v.css:314-443)
	// Always visible; colours come from --b1/--b2/--b3 defined per theme/style.
	// The prototype used <body class="fx-blobs"> – we render the same three <i> circles
	// without requiring a body class so it just works.
</script>

<div class="bgfx" aria-hidden="true">
	<i></i>
	<i></i>
	<i></i>
</div>

<style>
	/* intentionally empty – styles live in app.css so they can react to data-theme / data-style */
</style>
