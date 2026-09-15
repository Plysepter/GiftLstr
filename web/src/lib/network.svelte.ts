/** Global network state — stays in sync across route changes. */
export const network = $state({
  offline: typeof navigator !== "undefined" ? !navigator.onLine : false,
  /** Incremented each time connectivity returns, so the current view refetches. */
  synced: 0,
});

if (typeof window !== "undefined") {
  window.addEventListener("online", () => {
    network.offline = false;
    network.synced += 1;
  });
  window.addEventListener("offline", () => {
    network.offline = true;
  });
}
