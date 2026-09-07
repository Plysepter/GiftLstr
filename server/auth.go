package main

import (
	"context"
	"os"
	"strings"
)

// Identity is the validated profile of a session token's owner. Every field
// comes from the token itself (or the provider's userinfo), never from the
// app's own database, so provisioning can create a local user on first login.
type Identity struct {
	// Subject is the provider's stable external user id (Authentik `sub`).
	Subject string
	// Username is a human-readable handle, e.g. "alice".
	Username string
	// DisplayName is the user's full name, used for headers and claim labels.
	DisplayName string
	// Birthday is an optional MM-DD date carried in provider metadata.
	Birthday string
}

// AuthProvider is the single seam between the app and its identity provider.
// Request handlers, provisioning and admin checks only ever talk to this
// interface, so swapping providers is a matter of implementing VerifySession
// and wiring it up in main() — no handler changes.
type AuthProvider interface {
	// VerifySession validates a raw bearer token and returns the stable
	// external identity it represents. It must error for any invalid,
	// tampered or expired token.
	VerifySession(ctx context.Context, token string) (*Identity, error)
}

// RegisterAuthProvider associates a name (read from the AUTH_PROVIDER env var)
// with a concrete provider so operators can choose one at deploy time.
var authProviders = map[string]AuthProvider{}

func RegisterAuthProvider(name string, p AuthProvider) {
	authProviders[name] = p
}

func init() {
	// Register the default provider shipped with the binary.
	RegisterAuthProvider("authentik", authentikProviderFromEnv())
}

// activeAuth is the provider used by the auth middleware. It is resolved from
// the AUTH_PROVIDER environment variable at startup.
var activeAuth AuthProvider = resolveAuthProvider()

func resolveAuthProvider() AuthProvider {
	name := strings.TrimSpace(os.Getenv("AUTH_PROVIDER"))
	if name == "" {
		name = "authentik"
	}
	if p, ok := authProviders[name]; ok {
		return p
	}
	// Default to Authentik when nothing is wired up.
	return authentikProviderFromEnv()
}
