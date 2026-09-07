package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// oidcProxyTestServer serves a full discovery document plus a fake token
// endpoint, mimicking Authentik for the OIDC proxy handlers.
type oidcProxyTestServer struct {
	srv        *httptest.Server
	lastToken  url.Values
	tokenHits  int
	clientBase string
}

func newOIDCProxyTestServer(t *testing.T) *oidcProxyTestServer {
	t.Helper()
	s := &oidcProxyTestServer{}
	mux := http.NewServeMux()
	s.clientBase = "/application/o/giftlstr"
	mux.HandleFunc("/application/o/giftlstr/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 s.srv.URL + "/application/o/giftlstr/",
			"authorization_endpoint": s.srv.URL + "/application/o/giftlstr/authorize/",
			"token_endpoint":         s.srv.URL + "/application/o/giftlstr/token/",
			"end_session_endpoint":   s.srv.URL + "/application/o/giftlstr/end_session/",
			"jwks_uri":               s.srv.URL + "/application/o/giftlstr/jwks",
		})
	})
	mux.HandleFunc("/application/o/giftlstr/token/", func(w http.ResponseWriter, r *http.Request) {
		s.tokenHits++
		r.ParseForm()
		s.lastToken = r.PostForm
		json.NewEncoder(w).Encode(map[string]string{
			"access_token":  "tok-" + r.PostForm.Get("code"),
			"refresh_token": "rt-" + r.PostForm.Get("grant_type"),
			"expires_in":    "300",
		})
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *oidcProxyTestServer) url() string { return s.srv.URL }

func withActiveAuth(t *testing.T, p authentikProvider) {
	t.Helper()
	orig := activeAuth
	activeAuth = p
	resetOIDCCaches()
	t.Cleanup(func() {
		activeAuth = orig
		resetOIDCCaches()
	})
}

func TestHandleOIDCDiscoveryPassesThroughWhenSameEndpoint(t *testing.T) {
	s := newOIDCProxyTestServer(t)
	issuer := s.url() + "/application/o/giftlstr/"
	withActiveAuth(t, authentikProvider{issuer: issuer, issuerBackend: issuer, clientID: "c"})

	rr := httptest.NewRecorder()
	handleOIDCDiscovery(rr, httptest.NewRequest(http.MethodGet, "/api/oidc/discovery", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	var doc discoveryDocument
	json.Unmarshal(rr.Body.Bytes(), &doc)
	if doc.AuthorizationEndpoint != issuer+"authorize/" {
		t.Errorf("authorization_endpoint = %q, want %q", doc.AuthorizationEndpoint, issuer+"authorize/")
	}
}

func TestHandleOIDCDiscoveryRewritesBrowserEndpointsButKeepsServerEndpoints(t *testing.T) {
	// Server reaches Authentik at one origin (the reachable test server),
	// browsers at another (public). Authentik's authorize/end-session
	// endpoints live on the shared /application/o/ paths — authorize has no
	// provider segment — so only the origin (host) must be rewritten.
	public := "http://localhost:9000/application/o/giftlstr/"
	mux := http.NewServeMux()
	mux.HandleFunc("/application/o/giftlstr/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 "http://server:9000/application/o/giftlstr/",
			"authorization_endpoint": base + "/application/o/authorize/",
			"token_endpoint":         base + "/application/o/token/",
			"end_session_endpoint":   base + "/application/o/giftlstr/end-session/",
			"jwks_uri":               base + "/application/o/giftlstr/jwks",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	withActiveAuth(t, authentikProvider{issuer: public, issuerBackend: srv.URL + "/application/o/giftlstr/"})

	rr := httptest.NewRecorder()
	handleOIDCDiscovery(rr, httptest.NewRequest(http.MethodGet, "/api/oidc/discovery", nil))

	var doc discoveryDocument
	json.Unmarshal(rr.Body.Bytes(), &doc)
	if doc.Issuer != public {
		t.Errorf("issuer = %q, want %q", doc.Issuer, public)
	}
	// Browser-facing endpoints are rewritten to the public origin; the shared
	// `/application/o/authorize/` path is preserved (no provider segment).
	if doc.AuthorizationEndpoint != "http://localhost:9000/application/o/authorize/" {
		t.Errorf("authorization_endpoint = %q, want rewritten to localhost", doc.AuthorizationEndpoint)
	}
	if doc.EndSessionEndpoint != "http://localhost:9000/application/o/giftlstr/end-session/" {
		t.Errorf("end_session_endpoint = %q, want rewritten to localhost", doc.EndSessionEndpoint)
	}
	// Server-facing endpoints stay on the address the server can reach.
	if doc.TokenEndpoint != srv.URL+"/application/o/token/" {
		t.Errorf("token_endpoint = %q, want kept as server address", doc.TokenEndpoint)
	}
	if doc.JWKSURI != srv.URL+"/application/o/giftlstr/jwks" {
		t.Errorf("jwks_uri = %q, want kept as server address", doc.JWKSURI)
	}
}

func TestHandleOIDCDiscoveryUnconfigured(t *testing.T) {
	withActiveAuth(t, authentikProvider{})

	rr := httptest.NewRecorder()
	handleOIDCDiscovery(rr, httptest.NewRequest(http.MethodGet, "/api/oidc/discovery", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestHandleOIDCTokenForwardsAndPinsClientID(t *testing.T) {
	s := newOIDCProxyTestServer(t)
	issuer := s.url() + "/application/o/giftlstr/"
	withActiveAuth(t, authentikProvider{issuer: issuer, issuerBackend: issuer, clientID: "c1"})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"abc123"},
		"redirect_uri":  {"http://localhost:8080/signin-oauth"},
		"client_id":     {"c1"},
		"code_verifier": {"v"},
	}.Encode()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/oidc/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleOIDCToken(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	json.Unmarshal(rr.Body.Bytes(), &got)
	if got["access_token"] != "tok-abc123" {
		t.Errorf("access_token = %q, want %q", got["access_token"], "tok-abc123")
	}
	if s.tokenHits != 1 {
		t.Fatalf("token hits = %d, want 1", s.tokenHits)
	}
	if s.lastToken.Get("client_id") != "c1" {
		t.Errorf("proxied client_id = %q, want c1", s.lastToken.Get("client_id"))
	}
	if s.lastToken.Get("grant_type") != "authorization_code" || s.lastToken.Get("code_verifier") != "v" {
		t.Errorf("proxied body = %v", s.lastToken)
	}
}

func TestHandleOIDCTokenRejectsDifferentClientID(t *testing.T) {
	s := newOIDCProxyTestServer(t)
	issuer := s.url() + "/application/o/giftlstr/"
	withActiveAuth(t, authentikProvider{issuer: issuer, issuerBackend: issuer, clientID: "c1"})

	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"other"}}.Encode()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/oidc/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleOIDCToken(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if s.tokenHits != 0 {
		t.Errorf("upstream token called %d times for rejected client_id", s.tokenHits)
	}
}

func TestHandleOIDCTokenRejectsUnsupportedGrant(t *testing.T) {
	s := newOIDCProxyTestServer(t)
	issuer := s.url() + "/application/o/giftlstr/"
	withActiveAuth(t, authentikProvider{issuer: issuer, issuerBackend: issuer, clientID: "c1"})

	form := url.Values{"grant_type": {"password"}, "client_id": {"c1"}}.Encode()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/oidc/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleOIDCToken(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if s.tokenHits != 0 {
		t.Errorf("upstream token called %d times for rejected grant_type", s.tokenHits)
	}
}

func TestHandleOIDCTokenUnconfiguredClient(t *testing.T) {
	s := newOIDCProxyTestServer(t)
	issuer := s.url() + "/application/o/giftlstr/"
	withActiveAuth(t, authentikProvider{issuer: issuer, issuerBackend: issuer})

	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {""}}.Encode()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/oidc/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleOIDCToken(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}
