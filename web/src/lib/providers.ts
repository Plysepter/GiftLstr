// Front-end identity provider abstraction.
//
// auth.ts owns the app's session/`user` store and only talks to a
// `FeAuthProvider`. Swapping providers means implementing this interface and
// picking it in `pickAuthProvider` — no page/route changes are required.

export interface FeAuthProvider {
  /** A stable, human-readable name, e.g. "oidc" or "e2e". */
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
  /** Start the provider's sign-in flow (may navigate away). `prompt` is passed
   * through as the OIDC `prompt` parameter — e.g. "login" to force the
   * provider's credentials form even when an SSO session already exists. */
  openSignIn(opts?: { prompt?: string }): void;
  /** Complete a provider-initiated redirect (SSO callback) if one is pending. */
  completeOAuthSignIn?(): Promise<void>;
  /** Sign the user out at the provider. */
  signOut(): Promise<void>;
}

// ---------------------------------------------------------------------------
// OIDC provider (Authentik, default)
// ---------------------------------------------------------------------------

// Authentik's OIDC configuration is public (issuer + client id) and is
// injected into the served page at runtime by the backend (see serveIndex in
// server/main.go). During local dev the values come from VITE_AUTHENTIK_*.
interface AuthConfig {
  authIssuer?: string;
  authClientId?: string;
  // Served true in e2e/demo mode. Informational: the frontend decides e2e
  // behaviour from the VITE_E2E build flag instead, so the flag value here no
  // longer gates anything.
  authE2e?: boolean;
}

const runtimeConfig = (globalThis as { __GIFTLSTR__?: AuthConfig }).__GIFTLSTR__ ?? {};
const issuer =
  runtimeConfig.authIssuer || (import.meta.env.VITE_AUTHENTIK_ISSUER as string | undefined);
const clientId =
  runtimeConfig.authClientId || (import.meta.env.VITE_AUTHENTIK_CLIENT_ID as string | undefined);

/**
 * Compile-time flag: build the web app with VITE_E2E=true for the E2E/demo
 * backend (see e2e/e2e-server.sh). With it unset — every normal production
 * build — Vite defines this to `false`, so Rollup folds the branches below
 * away and the e2e provider / demo-login code is dropped from the bundle.
 */
const e2eBuild = import.meta.env.VITE_E2E === "true";

/** True when this build targets the E2E/demo backend (auth disabled). */
export const authE2e = e2eBuild;

interface OIDCDiscovery {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  end_session_endpoint?: string;
}

const TOKENS_KEY = "giftlstr:oidc:tokens";
const PKCE_KEY = "giftlstr:oidc:pkce";
const STATE_KEY = "giftlstr:oidc:state";
const LOGOUT_EVENT = "giftlstr:oidc:logout";

interface Tokens {
  accessToken: string;
  refreshToken?: string;
  expiresAt: number;
}

let discovery: Promise<OIDCDiscovery> | null = null;
let lastNotified: boolean | null = null;

/** Publish a durable (non-serializable) sign-in error out to the Login page. */
export function publishSignInError(message: string | null): void {
  window.dispatchEvent(
    new CustomEvent<string | null>("giftlstr-signin-error", { detail: message }),
  );
}

function readTokens(): Tokens | null {
  try {
    const raw = localStorage.getItem(TOKENS_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Tokens;
    return parsed.accessToken ? parsed : null;
  } catch {
    return null;
  }
}

function clearTokens(): void {
  localStorage.removeItem(TOKENS_KEY);
}

/** Base64url-encode raw bytes (unpadded), the OIDC PKCE/state encoding. */
function toBase64Url(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function randomString(bytes: number): string {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  return toBase64Url(buf);
}

async function sha256Hex(input: string): Promise<string> {
  const data = new TextEncoder().encode(input);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return toBase64Url(new Uint8Array(digest));
}

/** Fetch and cache Authentik's OIDC discovery document via the server proxy
 * (browser- and server-reachable, with endpoints rewritten to the browser
 * issuer), so the SPA never needs CORS access to Authentik directly. */
function getDiscovery(): Promise<OIDCDiscovery> {
  if (!discovery) {
    discovery = fetch("/api/oidc/discovery")
      .then((r) => {
        if (r.status === 503) {
          throw new Error("Authentik is not configured");
        }
        if (!r.ok) throw new Error(`OIDC discovery failed (${r.status})`);
        return r.json() as Promise<OIDCDiscovery>;
      })
      .catch((err) => {
        discovery = null; // allow retry next time
        throw err;
      });
  }
  return discovery;
}

function redirectUri(): string {
  return `${window.location.origin}/signin-oauth`;
}

/** Exchange an authorization code or refresh token via the server proxy. */
function unauthedTokenExchange(body: Record<string, string>): Promise<Tokens> {
  const params = new URLSearchParams(body);
  return fetch("/api/oidc/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: params.toString(),
  }).then(async (res) => {
    const data = (await res.json().catch(() => ({}))) as {
      access_token?: string;
      refresh_token?: string;
      expires_in?: number;
      error?: string;
      error_description?: string;
    };
    if (!res.ok || !data.access_token) {
      throw new Error(
        data.error_description || data.error || `Token exchange failed (${res.status})`,
      );
    }
    const tokens: Tokens = {
      accessToken: data.access_token,
      expiresAt: Date.now() + (data.expires_in ? data.expires_in * 1000 : 3600 * 1000),
    };
    if (data.refresh_token) {
      tokens.refreshToken = data.refresh_token;
    }
    return tokens;
  });
}

/** Try to slide the session forward with the stored refresh token. */
async function tryRefresh(): Promise<boolean> {
  const t = readTokens();
  if (!t?.refreshToken) return false;
  try {
    const next = await unauthedTokenExchange({
      grant_type: "refresh_token",
      refresh_token: t.refreshToken,
      client_id: clientId ?? "",
    });
    // Authentik does NOT rotate refresh tokens by default: the refresh grant
    // returns a fresh access_token but omits refresh_token, so a blindly
    // stored `next` would silently DROP the only thing that keeps the session
    // alive. Carry the existing one forward unless the provider rotated it.
    if (!next.refreshToken && t.refreshToken) {
      next.refreshToken = t.refreshToken;
    }
    localStorage.setItem(TOKENS_KEY, JSON.stringify(next));
    return true;
  } catch {
    return false;
  }
}

/** Whether a usable session exists, refreshing if necessary. */
async function sessionActive(): Promise<boolean> {
  const t = readTokens();
  if (!t?.accessToken) return false;
  if (t.expiresAt > Date.now() + 30_000) return true;
  if (await tryRefresh()) return true;
  clearTokens();
  return false;
}

function notify(cb: (active: boolean) => void): void {
  void sessionActive().then((active) => {
    if (active !== lastNotified) {
      lastNotified = active;
      cb(active);
    }
  });
}

export const oidcProvider: FeAuthProvider = {
  name: "oidc",
  async ready(): Promise<void> {
    if (!clientId) {
      console.error("Authentik OIDC config missing (AUTHENTIK_CLIENT_ID at runtime)");
      return;
    }
    if (!issuer) {
      console.warn("AUTHENTIK_ISSUER not injected; sign-in may not be fully configured");
    }
    // Warm the discovery cache so sign-in starts fast.
    getDiscovery().catch((err) => console.error("OIDC discovery failed", err));
  },
  async getToken(): Promise<string | null> {
    const t = readTokens();
    if (!t?.accessToken) return null;
    if (t.expiresAt > Date.now() + 30_000) return t.accessToken;
    if (await tryRefresh()) {
      return readTokens()?.accessToken ?? null;
    }
    clearTokens();
    return null;
  },
  async hasSession(): Promise<boolean> {
    return sessionActive();
  },
  onSessionChange(cb: (active: boolean) => void): () => void {
    lastNotified = null;
    notify(cb);
    const interval = window.setInterval(() => notify(cb), 30_000);
    const onStorage = (): void => notify(cb);
    const onLogout = (): void => notify(cb);
    window.addEventListener("storage", onStorage);
    window.addEventListener(LOGOUT_EVENT, onLogout);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(LOGOUT_EVENT, onLogout);
      lastNotified = null;
    };
  },
  openSignIn(opts?: { prompt?: string }): void {
    // Authorization Code + PKCE: send the browser to Authentik (via the
    // authorization endpoint from the proxied discovery document), which
    // either shows its own login (email/password, sources, MFA…) or completes
    // SSO, then returns to /signin-oauth.
    if (!clientId) {
      publishSignInError("Sign-in isn't configured yet.");
      return;
    }
    const state = randomString(32);
    const verifier = randomString(32);
    void sha256Hex(verifier).then((challenge) =>
      getDiscovery().then(
        (doc) => {
          localStorage.setItem(STATE_KEY, state);
          localStorage.setItem(PKCE_KEY, verifier);
          const url = new URL(doc.authorization_endpoint);
          url.searchParams.set("client_id", clientId);
          url.searchParams.set("redirect_uri", redirectUri());
          url.searchParams.set("response_type", "code");
          url.searchParams.set("scope", "openid profile email offline_access");
          url.searchParams.set("state", state);
          url.searchParams.set("code_challenge", challenge);
          url.searchParams.set("code_challenge_method", "S256");
          if (opts?.prompt) {
            url.searchParams.set("prompt", opts.prompt);
          }
          window.location.assign(url.toString());
        },
        (err) => {
          console.error("failed to start OIDC sign-in", err);
          publishSignInError("Couldn't start sign-in: the identity provider is unreachable.");
        },
      ),
    );
  },
  async completeOAuthSignIn(): Promise<void> {
    // Runs once when the browser lands back on /signin-oauth after SSO.
    if (window.location.pathname !== "/signin-oauth") return;
    const params = new URLSearchParams(window.location.search);
    if (params.has("error")) {
      publishSignInError(
        params.get("error_description") || params.get("error") || "Sign-in failed.",
      );
      window.history.replaceState(null, "", window.location.origin + "/");
      return;
    }
    const code = params.get("code");
    const state = params.get("state");
    if (!code || !state) {
      window.history.replaceState(null, "", window.location.origin + "/");
      return; // stale visit, nothing to complete
    }
    if (state !== localStorage.getItem(STATE_KEY)) {
      publishSignInError("Sign-in state did not match; please try again.");
      window.history.replaceState(null, "", window.location.origin + "/");
      return;
    }
    const verifier = localStorage.getItem(PKCE_KEY);
    if (!verifier) {
      publishSignInError("Sign-in could not be completed; please try again.");
      window.history.replaceState(null, "", window.location.origin + "/");
      return;
    }
    try {
      const tokens = await unauthedTokenExchange({
        grant_type: "authorization_code",
        code,
        redirect_uri: redirectUri(),
        client_id: clientId ?? "",
        code_verifier: verifier,
      });
      localStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
    } catch (err) {
      console.error("oauth callback failed", err);
      publishSignInError("Sign-in failed; please try again.");
    } finally {
      localStorage.removeItem(STATE_KEY);
      localStorage.removeItem(PKCE_KEY);
      window.history.replaceState(null, "", window.location.origin + "/");
    }
  },
  async signOut(): Promise<void> {
    clearTokens();
    window.dispatchEvent(new Event(LOGOUT_EVENT));
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
  openSignIn(_opts?: { prompt?: string }): void {
    // no-op in test mode
  },
  async signOut(): Promise<void> {
    // Clear the demo/e2e session token so a later reload returns to the Login
    // page (where the demo picker can sign back in).
    globalThis.localStorage?.removeItem("e2e_token");
  },
});

/** Picks the provider for this environment based on the presence of an e2e token. */
export function pickAuthProvider(): FeAuthProvider {
  if (e2eBuild) {
    const e2eToken = globalThis.localStorage?.getItem("e2e_token");
    if (e2eToken) {
      return e2eProvider(e2eToken);
    }
  }
  return oidcProvider;
}
