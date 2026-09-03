import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import { startAutoSync } from "./lib/offlineQueue.js";

startAutoSync();

const app = mount(App, { target: document.getElementById("app")! });

export default app;
