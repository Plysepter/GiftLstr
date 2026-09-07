package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// allowedAlgs is the set of JWS algorithms the app will accept. Anything else
// (including "none" and every symmetric option) is rejected before any key
// lookup so an attacker can never steer verification down an unintended
// hash/key path.
var allowedAlgs = map[string]bool{
	"RS256": true, "RS384": true, "RS512": true,
	"ES256": true, "ES384": true, "ES512": true,
}

// authentikProvider validates Authentik session tokens (OIDC access tokens
// signed by Authentik's provider JWKS) and derives the profile fields the app
// stores. Authentik signs standard JWTs, so verification is plain OIDC: fetch
// the provider JWKS via discovery, check the signature with the matching key,
// then read the claims — no SDK required.
type authentikProvider struct {
	// issuer is the issuer value the app expects in tokens' `iss` claim. This
	// is the browser-facing URL (where Authentik is reachable from clients),
	// so it is also safe to embed in the served page for the SPA OIDC flow.
	issuer string
	// issuerBackend is where the server fetches the OIDC discovery document
	// and JWKS from. It defaults to issuer and only needs to differ when the
	// server cannot reach the browser-facing URL directly (e.g. Authentik
	// behind a reverse proxy the server must reach under a different origin).
	issuerBackend string
	// clientID, when set, is checked against the token's `aud` claim.
	clientID string
	// birthdayClaim is the custom claim (MM-DD) carrying the user's birthday.
	birthdayClaim string
}

func authentikProviderFromEnv() authentikProvider {
	p := authentikProvider{
		issuer:        strings.TrimSuffix(strings.TrimSpace(os.Getenv("AUTHENTIK_ISSUER")), "/"),
		issuerBackend: strings.TrimSuffix(strings.TrimSpace(os.Getenv("AUTHENTIK_ISSUER_BACKEND")), "/"),
		clientID:      strings.TrimSpace(os.Getenv("AUTHENTIK_CLIENT_ID")),
		birthdayClaim: strings.TrimSpace(os.Getenv("AUTHENTIK_BIRTHDAY_CLAIM")),
	}
	if p.issuerBackend == "" {
		p.issuerBackend = p.issuer
	}
	if p.birthdayClaim == "" {
		p.birthdayClaim = "birthday"
	}
	return p
}

func (p authentikProvider) VerifySession(ctx context.Context, token string) (*Identity, error) {
	if p.issuer == "" {
		return nil, errors.New("AUTHENTIK_ISSUER is not configured")
	}
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return nil, errors.New("invalid session token")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(decoded(segments[0]), &header); err != nil {
		return nil, errors.New("invalid session token")
	}
	if !allowedAlgs[header.Alg] {
		return nil, errors.New("invalid session token algorithm")
	}
	key, err := p.publicKey(ctx, header.Kid, header.Alg)
	if err != nil {
		return nil, err
	}
	signature, err := decodedBytes(segments[2])
	if err != nil {
		return nil, errors.New("invalid session token")
	}
	if !verifySignature(key, header.Alg, []byte(segments[0]+"."+segments[1]), signature) {
		return nil, errors.New("invalid session token")
	}
	var claims struct {
		Sub          string `json:"sub"`
		Iss          string `json:"iss"`
		Aud          any    `json:"aud"`
		Exp          int64  `json:"exp"`
		Nbf          int64  `json:"nbf"`
		PreferredUsr string `json:"preferred_username"`
		Name         string `json:"name"`
		GivenName    string `json:"given_name"`
		FamilyName   string `json:"family_name"`
		Email        string `json:"email"`
	}
	if err := json.Unmarshal(decoded(segments[1]), &claims); err != nil {
		return nil, errors.New("invalid session token")
	}
	// The birthday lives in a custom claim whose name is configurable, so it is
	// read separately from the fixed fields above.
	var extraBirthday string
	var claimsMap map[string]any
	if err := json.Unmarshal(decoded(segments[1]), &claimsMap); err == nil {
		if v, ok := claimsMap[p.birthdayClaim].(string); ok {
			extraBirthday = v
		}
	}
	now := time.Now().Unix()
	if claims.Exp != 0 && now > claims.Exp+60 {
		return nil, errors.New("session token expired")
	}
	if claims.Nbf != 0 && now < claims.Nbf-60 {
		return nil, errors.New("session token not yet valid")
	}
	if !p.issuerMatches(claims.Iss) {
		return nil, fmt.Errorf("invalid session token issuer: got %q want %q", claims.Iss, p.issuer)
	}
	if p.clientID != "" && !audienceContains(claims.Aud, p.clientID) {
		return nil, errors.New("invalid session token audience")
	}
	if claims.Sub == "" {
		return nil, errors.New("invalid session token")
	}

	ident := &Identity{Subject: claims.Sub, Birthday: extraBirthday}
	ident.Username = claims.PreferredUsr
	if ident.Username == "" && claims.Email != "" {
		ident.Username = strings.SplitN(claims.Email, "@", 2)[0]
	}
	if ident.Username == "" {
		ident.Username = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(claims.GivenName+claims.FamilyName), " ", ""))
	}
	ident.DisplayName = strings.TrimSpace(claims.Name)
	if ident.DisplayName == "" {
		ident.DisplayName = strings.TrimSpace(claims.GivenName + " " + claims.FamilyName)
	}
	if ident.DisplayName == "" {
		ident.DisplayName = ident.Username
	}
	if ident.Username == "" {
		ident.Username = ident.DisplayName
	}
	if ident.Username == "" {
		ident.Username = claims.Sub
	}
	if ident.DisplayName == "" {
		ident.DisplayName = ident.Username
	}
	return ident, nil
}

// issuerMatches reports whether a token's `iss` claim originates from the
// identity provider we configured. Authentik bakes the request host it actually
// served into iss (e.g. the server-facing backend origin), while we advertise
// the browser-facing issuer to the SPA — so either configured URL is accepted.
// The whole URL (scheme, host and path) is compared so tokens issued for a
// sibling application on the same host are rejected.
func (p *authentikProvider) issuerMatches(iss string) bool {
	for _, raw := range []string{p.issuer, p.issuerBackend} {
		if canonicalIssuer(raw) != "" && canonicalIssuer(raw) == canonicalIssuer(iss) {
			return true
		}
	}
	return false
}

// canonicalIssuer normalizes a URL into a comparable identity: lowercased
// scheme and host plus the path with the trailing slash removed (Authentik
// issuer URLs end in "/"). An empty result means the value isn't a usable URL.
func canonicalIssuer(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/")
}

// ---- OIDC discovery + JWKS ----

var (
	discoveryMu   sync.Mutex
	discoveryDoc  *discoveryDocument
	discoveryAt   time.Time
	discoveryTTL  = 15 * time.Minute
	discoveryHTTP = &http.Client{Timeout: 10 * time.Second}
)

// discoveryDocument is the subset of Authentik's OpenID discovery document
// used by the app: the SPA flow (browser) and token/JWKS fetching (server).
type discoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint,omitempty"`
	JWKSURI               string `json:"jwks_uri"`
}

// discovery fetches (and caches) the OIDC discovery document from the issuer
// the server reaches Authentik at (issuerBackend). Safe for concurrent use.
func (p authentikProvider) discovery(ctx context.Context) (*discoveryDocument, error) {
	discoveryMu.Lock()
	defer discoveryMu.Unlock()
	if doc := discoveryDoc; doc != nil && time.Since(discoveryAt) < discoveryTTL {
		return doc, nil
	}
	docURL := p.issuerBackend + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := discoveryHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch oidc discovery: status %s", resp.Status)
	}
	var doc discoveryDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode oidc discovery: %w", err)
	}
	if doc.JWKSURI == "" || doc.TokenEndpoint == "" {
		return nil, errors.New("oidc discovery missing required endpoints")
	}
	discoveryDoc, discoveryAt = &doc, time.Now()
	return &doc, nil
}

type jwkEntry struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (p authentikProvider) jwksURI(ctx context.Context) (string, error) {
	doc, err := p.discovery(ctx)
	if err != nil {
		return "", err
	}
	return doc.JWKSURI, nil
}

var (
	jwksMu   sync.Mutex
	jwksKeys map[string]jwkEntry // kid -> raw JWKS entry (alg/kty included)
	jwksAt   time.Time
)

// publicKey resolves the JWKS entry matching the token's kid (or alg when no
// kid is present), validates that it is compatible with the token's algorithm,
// and returns the parsed public key.
func (p authentikProvider) publicKey(ctx context.Context, kid, alg string) (any, error) {
	jwksMu.Lock()
	defer jwksMu.Unlock()
	if e, ok := jwksKeys[kid]; ok && time.Since(jwksAt) < discoveryTTL {
		return e.verificationKey(alg)
	}
	uri, err := p.jwksURI(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := discoveryHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: status %s", resp.Status)
	}
	var set struct {
		Keys []jwkEntry `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}
	keys := map[string]jwkEntry{}
	for _, j := range set.Keys {
		if j.Use != "" && j.Use != "sig" {
			continue
		}
		if j.Kid != "" {
			keys[j.Kid] = j
		} else {
			// Key sets without kids are keyed by their declared alg so tokens
			// without a kid still resolve.
			keys[j.Alg] = j
		}
	}
	jwksKeys, jwksAt = keys, time.Now()
	e, ok := keys[kid]
	if !ok && alg != "" {
		e, ok = keys[alg]
	}
	if !ok {
		return nil, errors.New("no signing key found for session token")
	}
	return e.verificationKey(alg)
}

// verificationKey checks the JWKS entry is usable with the token's algorithm —
// the alg the JWKS declares must match, and the key type must match the
// algorithm family (RSA for RS*, EC for ES*) — and returns the Go public key.
func (j jwkEntry) verificationKey(alg string) (any, error) {
	if j.Alg != "" && j.Alg != alg {
		return nil, fmt.Errorf("signing key algorithm mismatch: key %q vs token %q", j.Alg, alg)
	}
	switch j.Kty {
	case "RSA":
		if !strings.HasPrefix(alg, "RS") {
			return nil, errors.New("signing key type mismatch")
		}
		n, err := decodedBytes(j.N)
		if err != nil {
			return nil, errors.New("invalid signing key")
		}
		e, err := decodedBytes(j.E)
		if err != nil {
			return nil, errors.New("invalid signing key")
		}
		exp := 0
		for _, b := range e {
			exp = exp<<8 | int(b)
		}
		if exp <= 0 {
			return nil, errors.New("invalid signing key")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}, nil
	case "EC":
		if !strings.HasPrefix(alg, "ES") {
			return nil, errors.New("signing key type mismatch")
		}
		x, err := decodedBytes(j.X)
		if err != nil {
			return nil, errors.New("invalid signing key")
		}
		y, err := decodedBytes(j.Y)
		if err != nil {
			return nil, errors.New("invalid signing key")
		}
		var curve elliptic.Curve
		switch j.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		}
		if curve == nil {
			return nil, errors.New("invalid signing key curve")
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}, nil
	}
	return nil, errors.New("unsupported signing key type")
}

// verifySignature checks the compact JWT signature against the token's alg.
// The digest is always derived from the algorithm itself and RSA/EC keys are
// bound to their family, so an allowlisted algorithm can never be verified
// with the wrong hash or curve.
func verifySignature(key any, alg string, input, signature []byte) bool {
	switch alg {
	case "RS256", "RS384", "RS512":
		rk, ok := key.(*rsa.PublicKey)
		if !ok {
			return false
		}
		hash := rsaHash(alg)
		h := hash.New()
		h.Write(input)
		return rsa.VerifyPKCS1v15(rk, hash, h.Sum(nil), signature) == nil
	case "ES256", "ES384", "ES512":
		ek, ok := key.(*ecdsa.PublicKey)
		if !ok || ek.Curve.Params().Name != esCurve(alg) {
			return false
		}
		hash := esHash(alg)
		h := hash.New()
		h.Write(input)
		return verifyECDSA(ek, h.Sum(nil), signature)
	}
	return false
}

func rsaHash(alg string) crypto.Hash {
	switch alg {
	case "RS384":
		return crypto.SHA384
	case "RS512":
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

func esHash(alg string) crypto.Hash {
	switch alg {
	case "ES384":
		return crypto.SHA384
	case "ES512":
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

func esCurve(alg string) string {
	switch alg {
	case "ES384":
		return "P-384"
	case "ES512":
		return "P-521"
	default:
		return "P-256"
	}
}

// verifyECDSA validates an ECDSA signature, which JOSE encodes as the raw r||s
// concatenation rather than an ASN.1 structure.
func verifyECDSA(k *ecdsa.PublicKey, digest, signature []byte) bool {
	half := (k.Curve.Params().BitSize + 7) / 8
	if len(signature) != 2*half {
		return false
	}
	r := new(big.Int).SetBytes(signature[:half])
	s := new(big.Int).SetBytes(signature[half:])
	return ecdsa.Verify(k, digest, r, s)
}

// decoded returns the raw bytes of a base64url (unpadded) JWT segment, best
// effort — the caller treats nil as an invalid token.
func decoded(seg string) []byte {
	b, err := decodedBytes(seg)
	if err != nil {
		return nil
	}
	return b
}

func decodedBytes(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(seg)
}

// audienceContains reports whether the JWT `aud` (string or array) contains
// the given audience.
func audienceContains(aud any, want string) bool {
	switch a := aud.(type) {
	case string:
		return a == want
	case []any:
		for _, v := range a {
			if s, ok := v.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}
