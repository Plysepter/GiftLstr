import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import { startAutoSync } from "./lib/offlineQueue.js";

startAutoSync();

if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => navigator.serviceWorker.register("/sw.js"));
}

const app = mount(App, { target: document.getElementById("app")! });

export default app;
