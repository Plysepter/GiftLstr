import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import { startAutoSync } from "./lib/offlineQueue.js";
import { authE2e } from "./lib/providers.js";

startAutoSync();

// In e2e/demo mode there is no offline story (ephemeral data, no identity
// provider), and the service worker's blanket fetch interception tripped a
// Chromium compositor SEGV under headless-shell; keep the real app's PWA
// behavior for normal (Authentik) deployments only.
if (!authE2e && "serviceWorker" in navigator) {
  window.addEventListener("load", () => navigator.serviceWorker.register("/sw.js"));
}

const app = mount(App, { target: document.getElementById("app")! });

export default app;
