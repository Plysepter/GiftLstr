package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	clerkuser "github.com/clerk/clerk-sdk-go/v2/user"
)

// AuthProvider is the single seam between the app and its identity provider.
// Request handlers, provisioning and admin checks only ever talk to this
// interface, so swapping Clerk for a different provider is a matter of
// implementing the two methods and wiring it up in main() — no handler changes.
type AuthProvider interface {
	// VerifySession validates a raw bearer token and returns the stable
	// external user id it represents (e.g. a Clerk user id). It must error for
	// any invalid, tampered or expired token.
	VerifySession(ctx context.Context, token string) (subject string, err error)
	// ProvisionInfo returns default profile details (username, display name,
	// optional MM-DD birthday) for a user signing in for the first time. The
	// birthday comes from provider metadata when available.
	ProvisionInfo(ctx context.Context, subject string) (username, displayName, birthday string, err error)
}

// RegisterAuthProvider associates a name (read from the AUTH_PROVIDER env var)
// with a concrete provider so operators can choose one at deploy time. The
// default build registers "clerk".
var authProviders = map[string]AuthProvider{}

func RegisterAuthProvider(name string, p AuthProvider) {
	authProviders[name] = p
}

func init() {
	// Register the default provider shipped with the binary.
	RegisterAuthProvider("clerk", clerkProvider{})
}

// activeAuth is the provider used by the auth middleware. It is resolved from
// the AUTH_PROVIDER environment variable at startup.
var activeAuth AuthProvider = resolveAuthProvider()

func resolveAuthProvider() AuthProvider {
	name := strings.TrimSpace(os.Getenv("AUTH_PROVIDER"))
	if name == "" {
		name = "clerk"
	}
	if p, ok := authProviders[name]; ok {
		return p
	}
	// Default to Clerk when nothing is wired up.
	return clerkProvider{}
}

// clerkProvider is the default identity provider, backed by Clerk.
type clerkProvider struct{}

// VerifySession decodes and verifies the Clerk session JWT via the JWKS client
// configured in main().
func (clerkProvider) VerifySession(ctx context.Context, token string) (string, error) {
	unsafe, err := clerkjwt.Decode(ctx, &clerkjwt.DecodeParams{Token: token})
	if err != nil {
		return "", errors.New("invalid session token")
	}
	jwk, err := clerkjwt.GetJSONWebKey(ctx, &clerkjwt.GetJSONWebKeyParams{
		KeyID:      unsafe.KeyID,
		JWKSClient: jwksClient,
	})
	if err != nil {
		return "", errors.New("failed to fetch signing key")
	}
	claims, err := clerkjwt.Verify(ctx, &clerkjwt.VerifyParams{Token: token, JWK: jwk})
	if err != nil {
		return "", errors.New("invalid session token")
	}
	return claims.Subject, nil
}

// ProvisionInfo maps a Clerk user to the profile fields the app stores.
func (clerkProvider) ProvisionInfo(ctx context.Context, subject string) (string, string, string, error) {
	u, err := clerkUser(ctx, subject)
	if err != nil {
		return "", "", "", err
	}
	username := ""
	if u.Username != nil {
		username = *u.Username
	}
	if username == "" && len(u.EmailAddresses) > 0 {
		email := ""
		for _, e := range u.EmailAddresses {
			if u.PrimaryEmailAddressID != nil && e.ID == *u.PrimaryEmailAddressID {
				email = e.EmailAddress
			}
		}
		if email == "" && len(u.EmailAddresses) > 0 {
			email = u.EmailAddresses[0].EmailAddress
		}
		parts := strings.SplitN(email, "@", 2)
		username = parts[0]
	}
	if username == "" {
		username = strings.ToLower(strings.ReplaceAll(strVal(u.FirstName)+strVal(u.LastName), " ", ""))
	}
	displayName := strings.TrimSpace(strVal(u.FirstName) + " " + strVal(u.LastName))
	if displayName == "" {
		displayName = username
	}
	var meta struct {
		Birthday string `json:"birthday"`
	}
	if len(u.PublicMetadata) > 0 {
		if err := json.Unmarshal(u.PublicMetadata, &meta); err != nil {
			meta.Birthday = ""
		}
	}
	return username, displayName, normalizeMMDD(meta.Birthday), nil
}

// clerkUser fetches profile info for a Clerk user id. Overridable in tests.
var clerkUser = func(ctx context.Context, clerkID string) (*clerk.User, error) {
	return clerkuser.Get(ctx, clerkID)
}
