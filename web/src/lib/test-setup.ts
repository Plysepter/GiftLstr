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

export {};
