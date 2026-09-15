import { writable } from "svelte/store";
import { api } from "./api.js";
import { setTokenGetter } from "./token.js";
import { clearQueue } from "./offlineQueue.js";
import { authE2e, pickAuthProvider, type FeAuthProvider } from "./providers.js";
import type { User } from "./types.js";

/** The locally provisioned app user (from our backend), null until signed in. */
export const user = writable<User | null>(null);
/** True once the auth provider is loaded and local state resolved. */
export const loaded = writable(false);

const provider: FeAuthProvider = pickAuthProvider();

// Automatic sign-in redirect policy. Redirecting to the identity provider is
// skipped (the in-app Login screen is shown instead) when:
//   - the app is running in E2E/demo mode (Login offers seeded demo accounts);
//   - the user signed out explicitly (Authentik SSO would silently re-log them in);
//   - the last sign-in attempt reported an error (the Login page displays it);
//   - an OAuth callback is still being processed, or a redirect is already on its way.
let explicitSignOut = false;
let signInErrorShown = false;
let inOAuthCallback = false;
let redirectRequested = false;

if (typeof window !== "undefined") {
  window.addEventListener("giftlstr-signin-error", () => {
    signInErrorShown = true;
  });
}

/** Send the user to the provider's login page unless the in-app one is meant to show. */
function maybeRedirectToSignIn(): void {
  if (authE2e || explicitSignOut || signInErrorShown || inOAuthCallback || redirectRequested) return;
  redirectRequested = true;
  provider.openSignIn();
}

async function onSessionChange(active: boolean): Promise<void> {
  if (!active) {
    user.set(null);
    maybeRedirectToSignIn();
    return;
  }
  explicitSignOut = false;
  signInErrorShown = false;
  try {
    const data = await api<{ user: User }>("/api/me");
    user.set(data.user);
  } catch (err) {
    console.error("failed to load profile", err);
    user.set(null);
  }
}

async function init(): Promise<void> {
  explicitSignOut = false;
  redirectRequested = false;
  // While the OAuth callback is being exchanged, a (premature) "no session"
  // notification from the subscription below must not bounce the user back to
  // Authentik before the callback has had a chance to complete.
  inOAuthCallback =
    typeof window !== "undefined" && window.location.pathname === "/signin-oauth";
  try {
    await provider.ready();
    setTokenGetter(() => provider.getToken());
    provider.onSessionChange((active) => void onSessionChange(active));
    // Complete a pending SSO redirect (landing on /signin-oauth) before
    // checking for a session, so a fresh OAuth sign-in flows straight in.
    await provider.completeOAuthSignIn?.();
    if (await provider.hasSession()) {
      await onSessionChange(true);
    } else {
      // No usable session: a fresh visit, or an expired access token whose
      // refresh failed. Send the user to the identity provider's login page.
      maybeRedirectToSignIn();
    }
  } catch (err) {
    console.error("auth init failed", err);
    maybeRedirectToSignIn();
  } finally {
    loaded.set(true);
  }
}

void init();

/** Opens the active provider's sign-in UI. */
export function openSignIn(): void {
  provider.openSignIn();
}

export async function signOut(): Promise<void> {
  explicitSignOut = true;
  await provider.signOut();
  user.set(null);
  // No cached lists, offline queue, or other account's data may survive a sign-out.
  clearQueue();
  if ("serviceWorker" in navigator) {
    try {
      const reg = await navigator.serviceWorker.getRegistration();
      reg?.active?.postMessage({ type: "giftlstr-clear" });
    } catch {
      // Service worker unreachable; nothing to clear.
    }
  }
  window.location.hash = "/";
}
