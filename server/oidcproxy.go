package main

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OIDC proxy endpoints. The SPA performs the Authorization Code + PKCE dance
// against the app's own origin so the browser never needs CORS access to
// Authentik: it reads the discovery document and exchanges tokens here, and
// only the sign-in redirect (a top-level navigation) goes to Authentik
// directly. The browser-facing URLs in the served document are rewritten to
// AUTHENTIK_ISSUER when the server reaches Authentik through a different
// address (AUTHENTIK_ISSUER_BACKEND, e.g. the compose-internal `server` DNS
// name).

// oidcProxyProvider returns the active Authentik provider backing the OIDC
// proxy endpoints. Non-Authentik providers (or an unconfigured issuer) have
// no proxied endpoints.
func oidcProxyProvider() (authentikProvider, bool) {
	p, ok := activeAuth.(authentikProvider)
	if !ok || p.issuer == "" {
		return authentikProvider{}, false
	}
	return p, true
}

// publicDiscoveryDoc rewrites the discovery document so the endpoints the
// browser will navigate to point at AUTHENTIK_ISSUER. Only the browser-facing
// endpoints (issuer, authorize, end-session) are rewritten; token_endpoint and
// jwks_uri keep the server-facing address, since only the server calls them.
func (p authentikProvider) publicDiscoveryDoc(doc *discoveryDocument) discoveryDocument {
	out := *doc
	if p.issuerBackend == "" || p.issuerBackend == p.issuer {
		return out
	}
	out.Issuer = p.issuer
	out.AuthorizationEndpoint = browserURL(doc.AuthorizationEndpoint, p.issuerBackend, p.issuer)
	out.EndSessionEndpoint = browserURL(doc.EndSessionEndpoint, p.issuerBackend, p.issuer)
	return out
}

// browserURL swaps the origin (scheme://host:port) of raw for the origin of
// public when raw points at the server-facing Authentik address. Authentik's
// endpoint paths (e.g. /application/o/authorize/) are shared across
// applications, so only the origin can be rewritten; URLs on a different host
// are left untouched.
func browserURL(raw, backend, public string) string {
	if raw == "" {
		return ""
	}
	backendURL, berr := url.Parse(backend)
	publicURL, perr := url.Parse(public)
	rawURL, rerr := url.Parse(raw)
	if berr != nil || perr != nil || rerr != nil || rawURL.Host != backendURL.Host {
		return raw
	}
	rawURL.Scheme = publicURL.Scheme
	rawURL.Host = publicURL.Host
	return rawURL.String()
}

// handleOIDCDiscovery serves Authentik's OpenID discovery document for the
// SPA, with endpoints rewritten for the browser (see publicDiscoveryDoc).
func handleOIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	p, ok := oidcProxyProvider()
	if !ok {
		fail(w, http.StatusServiceUnavailable, "identity provider not configured")
		return
	}
	doc, err := p.discovery(r.Context())
	if err != nil {
		fail(w, http.StatusBadGateway, "identity provider unreachable")
		return
	}
	writeJSON(w, http.StatusOK, p.publicDiscoveryDoc(doc))
}

// allowedOIDCGrants are the only OAuth grant types the SPA proxy will relay to
// Authentik's token endpoint. Anything else (password, client credentials, …)
// is rejected locally so the endpoint can't act as a generic OAuth relay even
// if Authentik has more grant types enabled on the application.
var allowedOIDCGrants = map[string]bool{
	"authorization_code": true,
	"refresh_token":      true,
}

// handleOIDCToken forwards the SPA's token request (authorization_code
// exchange or refresh) to Authentik's token endpoint. The destination comes
// from the cached discovery document — never from client input — and the
// client_id is pinned to the configured value, so the endpoint can't be used
// as a generic OAuth token relay.
func handleOIDCToken(w http.ResponseWriter, r *http.Request) {
	p, ok := oidcProxyProvider()
	if !ok || p.clientID == "" {
		fail(w, http.StatusServiceUnavailable, "identity provider not configured")
		return
	}
	if err := r.ParseForm(); err != nil {
		fail(w, http.StatusBadRequest, "invalid form")
		return
	}
	if got := r.PostForm.Get("client_id"); got != p.clientID {
		fail(w, http.StatusBadRequest, "invalid client_id")
		return
	}
	if grant := r.PostForm.Get("grant_type"); !allowedOIDCGrants[grant] {
		fail(w, http.StatusBadRequest, "unsupported grant_type")
		return
	}
	doc, err := p.discovery(r.Context())
	if err != nil {
		fail(w, http.StatusBadGateway, "identity provider unreachable")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, doc.TokenEndpoint, strings.NewReader(r.PostForm.Encode()))
	if err != nil {
		fail(w, http.StatusInternalServerError, "proxy error")
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := discoveryHTTP.Do(req)
	if err != nil {
		fail(w, http.StatusBadGateway, "identity provider unreachable")
		return
	}
	defer resp.Body.Close()
	// Pass Authentik's JSON through unchanged; on failure the SPA reads
	// error/error_description from the body.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
