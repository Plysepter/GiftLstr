package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// oidcTestServer serves a tiny OIDC discovery doc plus a JWKS set backed by a
// single RSA signing key, mimicking an Authentik provider.
type oidcTestServer struct {
	key *rsa.PrivateKey
	srv *httptest.Server
}

func newOIDCTestServer(t *testing.T) *oidcTestServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s := &oidcTestServer{key: key}
	// Real Authentik serves discovery under the application path.
	mux.HandleFunc("/application/o/giftlstr/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 s.url() + "/application/o/giftlstr/",
			"authorization_endpoint": s.url() + "/application/o/giftlstr/authorize/",
			"token_endpoint":         s.url() + "/application/o/giftlstr/token/",
			"jwks_uri":               s.url() + "/application/o/giftlstr/jwks",
		})
	})
	mux.HandleFunc("/application/o/giftlstr/jwks", func(w http.ResponseWriter, r *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(s.key.PublicKey.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.PublicKey.E)).Bytes())
		json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-kid", "alg": "RS256", "use": "sig", "n": n, "e": e,
			}},
		})
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *oidcTestServer) url() string { return s.srv.URL }

func (s *oidcTestServer) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "kid": "test-kid", "typ": "JWT"}
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func newTestProvider(s *oidcTestServer, clientID string) authentikProvider {
	issuer := s.url() + "/application/o/giftlstr/"
	return authentikProvider{
		issuer:        issuer,
		issuerBackend: issuer,
		clientID:      clientID,
		birthdayClaim: "birthday",
	}
}

func resetOIDCCaches() {
	discoveryDoc, discoveryAt = nil, time.Time{}
	jwksKeys, jwksAt = nil, time.Time{}
}

func goodClaims(s *oidcTestServer, sub string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"sub": sub, "iss": s.url() + "/application/o/giftlstr/",
		"aud": "test-client", "exp": now + 3600, "iat": now - 10,
		"preferred_username": "alice", "name": "Alice Example",
		"email": "alice@example.com", "birthday": "04-21",
	}
}

func TestAuthentikProviderVerifiesAndDerivesIdentity(t *testing.T) {
	s := newOIDCTestServer(t)
	resetOIDCCaches()
	p := newTestProvider(s, "test-client")

	id, err := p.VerifySession(context.Background(), s.token(t, goodClaims(s, "a1b2c3")))
	if err != nil {
		t.Fatalf("VerifySession: %v", err)
	}
	if id.Subject != "a1b2c3" {
		t.Errorf("subject = %q, want a1b2c3", id.Subject)
	}
	if id.Username != "alice" || id.DisplayName != "Alice Example" || id.Birthday != "04-21" {
		t.Errorf("identity = %+v, want username=alice display=Alice Example birthday=04-21", id)
	}
}

func TestAuthentikProviderRejectsBadTokens(t *testing.T) {
	s := newOIDCTestServer(t)
	resetOIDCCaches()
	p := newTestProvider(s, "test-client")
	ctx := context.Background()

	valid := s.token(t, goodClaims(s, "sub1"))

	// Tampered payload: signature no longer matches.
	segments := strings.Split(valid, ".")
	tampered := segments[0] + "." + segments[1][:len(segments[1])-2] + "XX" + "." + segments[2]
	if _, err := p.VerifySession(ctx, tampered); err == nil {
		t.Error("tampered token accepted")
	}

	// Garbage token.
	if _, err := p.VerifySession(ctx, "not.a.jwt"); err == nil {
		t.Error("garbage token accepted")
	}

	// Expired.
	expired := goodClaims(s, "sub1")
	expired["exp"] = time.Now().Unix() - 120
	if _, err := p.VerifySession(ctx, s.token(t, expired)); err == nil {
		t.Error("expired token accepted")
	}

	// Wrong audience.
	badAud := goodClaims(s, "sub1")
	badAud["aud"] = "other-client"
	if _, err := p.VerifySession(ctx, s.token(t, badAud)); err == nil {
		t.Error("token with wrong audience accepted")
	}

	// Wrong iss.
	badIss := goodClaims(s, "sub1")
	badIss["iss"] = "https://evil.example/"
	if _, err := p.VerifySession(ctx, s.token(t, badIss)); err == nil {
		t.Error("token with wrong issuer accepted")
	}
}

func TestAuthentikProviderAcceptsBackendIssuerOrigin(t *testing.T) {
	// Mirror the compose setup: the SPA talks to a browser-facing issuer while
	// the server exchanges tokens against the backend origin, which is what
	// Authentik bakes into the access token's iss claim (full path included).
	s := newOIDCTestServer(t)
	resetOIDCCaches()
	p := newTestProvider(s, "test-client")
	p.issuer = "http://localhost:9000/application/o/giftlstr/" // browser-facing
	p.issuerBackend = s.url() + "/application/o/giftlstr/"     // server-facing, reachable

	claims := goodClaims(s, "sub1")
	claims["iss"] = p.issuerBackend
	if _, err := p.VerifySession(context.Background(), s.token(t, claims)); err != nil {
		t.Fatalf("token with backend-origin issuer rejected: %v", err)
	}
}

func TestAuthentikProviderRejectsSiblingApplicationIssuer(t *testing.T) {
	// Same host-origin but a different Authentik application path: the full
	// issuer URL must match, not just scheme+host.
	s := newOIDCTestServer(t)
	resetOIDCCaches()
	p := newTestProvider(s, "")
	p.issuer = "http://localhost:9000/application/o/giftlstr/"

	claims := goodClaims(s, "sub1")
	claims["iss"] = "http://localhost:9000/application/o/other-app/"
	if _, err := p.VerifySession(context.Background(), s.token(t, claims)); err == nil {
		t.Fatal("token from a sibling application on the same host was accepted")
	}
}

func TestAuthentikProviderSkipsAudienceCheckWhenUnconfigured(t *testing.T) {
	s := newOIDCTestServer(t)
	resetOIDCCaches()
	p := newTestProvider(s, "")
	if _, err := p.VerifySession(context.Background(), s.token(t, goodClaims(s, "sub1"))); err != nil {
		t.Fatalf("token without client_id configured should verify: %v", err)
	}
}

func TestAuthentikProviderDerivesNamesFromFallbackClaims(t *testing.T) {
	s := newOIDCTestServer(t)
	resetOIDCCaches()
	p := newTestProvider(s, "")

	// No preferred_username, but email present.
	claims := goodClaims(s, "sub2")
	claims["preferred_username"] = ""
	claims["name"] = ""
	id, err := p.VerifySession(context.Background(), s.token(t, claims))
	if err != nil {
		t.Fatal(err)
	}
	if id.Username != "alice" {
		t.Errorf("username = %q, want email local part alice", id.Username)
	}

	// Nothing but sub: names fall back to the subject.
	claims["email"] = ""
	claims["given_name"] = ""
	claims["family_name"] = ""
	id, err = p.VerifySession(context.Background(), s.token(t, claims))
	if err != nil {
		t.Fatal(err)
	}
	if id.Username == "" || id.DisplayName == "" {
		t.Errorf("username/display should never be empty, got %+v", id)
	}
}
