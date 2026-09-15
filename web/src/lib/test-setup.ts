// Vitest setup file (jsdom environment configured in vite.config.js).
// Adds the browser APIs jsdom is missing that the app relies on.

interface MockMediaQueryList extends MediaQueryList {
  onchange: ((this: MediaQueryList, ev: MediaQueryListEvent) => void) | null;
}

/** jsdom does not implement window.matchMedia; provide a inert stub. */
if (typeof window !== "undefined" && typeof window.matchMedia !== "function") {
  window.matchMedia = ((query: string): MockMediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as {
    (query: string): MediaQueryList;
  };
}

/** jsdom lacks <dialog> open methods; implement the parts the app uses. */
if (typeof HTMLDialogElement !== "undefined") {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement): void {
      this.open = true;
      this.returnValue = "";
    };
    HTMLDialogElement.prototype.close = function (this: HTMLDialogElement): void {
      if (!this.open) return;
      this.open = false;
      this.dispatchEvent(new Event("close"));
    };
  }
}

/** jsdom with an opaque origin has no usable localStorage; fall back to an
 * in-memory implementation so modules that read it (e.g. the offline queue)
 * behave in tests. */
if (typeof window !== "undefined") {
  let broken = false;
  if (typeof window.localStorage === "object") {
    try {
      window.localStorage.setItem("__giftlstr_probe__", "1");
      window.localStorage.removeItem("__giftlstr_probe__");
    } catch {
      broken = true;
    }
  } else {
    broken = true;
  }
  if (broken) {
    const store = new Map<string, string>();
    const memStorage: Storage = {
      get length() {
        return store.size;
      },
      clear: () => store.clear(),
      getItem: (k) => (store.has(k) ? store.get(k)! : null),
      key: (i) => [...store.keys()][i] ?? null,
      removeItem: (k) => void store.delete(k),
      setItem: (k, v) => void store.set(k, String(v)),
    };
    Object.defineProperty(window, "localStorage", {
      value: memStorage,
      configurable: true,
    });
    Object.defineProperty(globalThis, "localStorage", {
      value: memStorage,
      configurable: true,
    });
  }
}

export {};
