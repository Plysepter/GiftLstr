import { writable } from "svelte/store";
import { api } from "./api.js";
import { setTokenGetter } from "./token.js";
import { pickAuthProvider, type FeAuthProvider } from "./providers.js";
import type { User } from "./types.js";

/** The locally provisioned app user (from our backend), null until signed in. */
export const user = writable<User | null>(null);
/** True once the auth provider is loaded and local state resolved. */
export const loaded = writable(false);

const provider: FeAuthProvider = pickAuthProvider();

async function onSessionChange(active: boolean): Promise<void> {
  if (!active) {
    user.set(null);
    return;
  }
  try {
    const data = await api<{ user: User }>("/api/me");
    user.set(data.user);
  } catch (err) {
    console.error("failed to load profile", err);
    user.set(null);
  }
}

async function init(): Promise<void> {
  try {
    await provider.ready();
    setTokenGetter(() => provider.getToken());
    provider.onSessionChange((active) => void onSessionChange(active));
    // Complete a pending SSO redirect (landing on /signin-oauth) before
    // checking for a session, so a fresh OAuth sign-in flows straight in.
    await provider.completeOAuthSignIn?.();
    if (await provider.hasSession()) {
      await onSessionChange(true);
    }
  } catch (err) {
    console.error("auth init failed", err);
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
  await provider.signOut();
  user.set(null);
  window.location.hash = "/";
}
