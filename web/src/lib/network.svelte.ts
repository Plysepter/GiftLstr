/** Global network state — stays in sync across route changes. */
export const network = $state({
  offline: typeof navigator !== "undefined" ? !navigator.onLine : false,
});

if (typeof window !== "undefined") {
  window.addEventListener("online", () => {
    network.offline = false;
  });
  window.addEventListener("offline", () => {
    network.offline = true;
  });
}
