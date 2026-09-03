// Front-end identity provider abstraction.
//
// auth.ts owns the app's session/`user` store and only talks to a
// `FeAuthProvider`. Swapping Clerk for another provider (Auth0, Firebase, your
// own OIDC, etc.) means implementing this interface and picking it in
// `pickAuthProvider` — no page/route changes are required.

export interface FeAuthProvider {
  /** A stable, human-readable name, e.g. "clerk" or "e2e". */
  name: string;
  /** Load the SDK and make the provider ready to answer getToken(). */
  ready(): Promise<void>;
  /** Return the current bearer token, or null when signed out. */
  getToken(): Promise<string | null>;
  /** Returns true when a session is currently active. */
  hasSession(): Promise<boolean>;
  /**
   * Subscribe to session changes. The callback fires with true when a session
   * appears and false when it is cleared. Returns an unsubscribe function.
   */
  onSessionChange(cb: (active: boolean) => void): () => void;
  /** Start the provider's sign-in flow (may navigate away). */
  openSignIn(): void;
  /** Complete a provider-initiated redirect (SSO callback) if one is pending. */
  completeOAuthSignIn?(): Promise<void>;
  /** Sign the user out at the provider. */
  signOut(): Promise<void>;
}

// ---------------------------------------------------------------------------
// Clerk provider (default)
// ---------------------------------------------------------------------------

const publishableKey = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY as string | undefined;

let clerk: InstanceType<(typeof import("@clerk/clerk-js/no-rhc"))["Clerk"]> | null = null;
let clerkReady: Promise<void> | null = null;

/** Publish a durable (non-serializable) sign-in error out to the Login page. */
function publishSignInError(message: string | null): void {
  window.dispatchEvent(new CustomEvent("giftlstr:signin-error", { detail: message }));
}

function signInErrorMessage(err: unknown): string {
  const e = err as {
    status?: number;
    retryAfter?: number;
    errors?: Array<{ message?: string; long_message?: string }>;
  };
  if (e?.status === 429) {
    const secs = e.retryAfter;
    const wait =
      secs != null && secs > 0
        ? secs >= 60
          ? `about ${Math.ceil(secs / 60)} minute${secs >= 120 ? "s" : ""}`
          : `${secs} seconds`
        : "a few minutes";
    return `Google sign-in is rate-limited (Clerk development keys cap OAuth usage). Try again in ${wait}.`;
  }
  return (
    e?.errors?.[0]?.long_message ??
    e?.errors?.[0]?.message ??
    "Something went wrong starting Google sign-in."
  );
}

async function loadClerk(): Promise<void> {
  if (!publishableKey) {
    console.error("VITE_CLERK_PUBLISHABLE_KEY is not set");
    return;
  }
  // Clerk is loaded on demand so its SDK stays out of the initial bundle.
  // We render our own sign-in UI on top of the headless ("no-rhc") build and
  // the vanilla Frontend API client — no @clerk/ui components are bundled.
  const { Clerk } = await import("@clerk/clerk-js/no-rhc");
  const c = new Clerk(publishableKey);
  await c.load();
  clerk = c;
}

/** Ensure the Clerk SDK is loaded (UI components available), then resolve. */
function ensureReady(): Promise<void> {
  if (!clerkReady) {
    clerkReady = loadClerk();
  }
  return clerkReady;
}

export const clerkProvider: FeAuthProvider = {
  name: "clerk",
  ready: ensureReady,
  async getToken(): Promise<string | null> {
    const session = clerk?.session;
    return session ? session.getToken() : null;
  },
  async hasSession(): Promise<boolean> {
    return Boolean(clerk?.session);
  },
  onSessionChange(cb: (active: boolean) => void): () => void {
    if (!clerk) return () => {};
    return clerk.addListener(({ session }) => cb(Boolean(session)));
  },
  openSignIn(): void {
    // Google SSO: kick off the OAuth flow. clerk-signIn.create() returns the
    // Google authorization URL on firstFactorVerification; newer clerk-js no
    // longer navigates automatically, so we do. Google returns to
    // /signin-oauth, where init() completes the session via completeOAuthSignIn().
    void ensureReady()
      .then(async () => {
        if (!clerk?.client) {
          publishSignInError("Sign-in isn't configured yet.");
          return;
        }
        const signIn = await clerk.client.signIn.create({
          strategy: "oauth_google",
          redirectUrl: `${window.location.origin}/signin-oauth`,
          actionCompleteRedirectUrl: `${window.location.origin}/`,
        });
        const url = signIn.firstFactorVerification?.externalVerificationRedirectURL;
        if (url) {
          window.location.assign(url.toString());
        } else {
          publishSignInError("Couldn't start Google sign-in: no redirect URL was returned.");
        }
      })
      .catch((err) => {
        console.error("failed to start google sign-in", err);
        publishSignInError(signInErrorMessage(err));
      });
  },
  async completeOAuthSignIn(): Promise<void> {
    // Runs once when the browser lands back on /signin-oauth after SSO.
    if (!clerk || window.location.pathname !== "/signin-oauth") return;
    const params = new URLSearchParams(window.location.search);
    const isFlow = [...params.keys()].some(
      (k) => k.toLowerCase().includes("clerk") || k === "error",
    );
    if (!isFlow) return; // stale visit, nothing to complete
    try {
      await clerk.handleRedirectCallback({
        signInForceRedirectUrl: "/",
        signInFallbackRedirectUrl: "/",
        signUpForceRedirectUrl: "/",
        signUpFallbackRedirectUrl: "/",
      });
    } catch (err) {
      console.error("oauth callback failed", err);
    } finally {
      window.history.replaceState(null, "", window.location.origin + "/");
    }
  },
  async signOut(): Promise<void> {
    if (clerk) await clerk.signOut();
  },
};

// ---------------------------------------------------------------------------
// End-to-end test provider. Reads a fixed token from localStorage and skips
// any real identity provider. Enabled only when an e2e token is present.
// ---------------------------------------------------------------------------

export const e2eProvider = (token: string): FeAuthProvider => ({
  name: "e2e",
  async ready(): Promise<void> {},
  async getToken(): Promise<string | null> {
    return token;
  },
  async hasSession(): Promise<boolean> {
    return true;
  },
  onSessionChange(): () => void {
    return () => {};
  },
  openSignIn(): void {
    // no-op in test mode
  },
  async signOut(): Promise<void> {},
});

/** Picks the provider for this environment based on the presence of an e2e token. */
export function pickAuthProvider(): FeAuthProvider {
  const e2eToken = globalThis.localStorage?.getItem("e2e_token");
  if (e2eToken) {
    return e2eProvider(e2eToken);
  }
  return clerkProvider;
}
