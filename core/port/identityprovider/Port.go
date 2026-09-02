// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package identityprovider is the relying-party half of ADR-0005, as the application layer needs
// it: send somebody to their company's provider, and turn what comes back into a verified
// identity.
//
// The interface is deliberately two methods and no library vocabulary. Discovery, JWKS caching,
// key rotation and the whole of the token verification live behind it, in `infrastructure/oidc`
// and nowhere else (ADR-0036) - so the core describes what signing in through somebody else's
// provider means, and the library that does it can be swapped without a single use case
// changing.
package identityprovider

import (
	"context"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Config is one workspace's provider, as a flow needs it.
//
// It is passed per call rather than held by the adapter, because the adapter is one object for
// the installation and the configuration belongs to a workspace: an adapter that remembered
// "the" issuer would be a cross-tenant bug waiting for its second customer.
type Config struct {
	// Issuer identifies the provider and is checked against every ID token's `iss` exactly.
	Issuer   string
	ClientID string
	// ClientSecret is opened from its E-02 envelope for the length of one exchange.
	ClientSecret secret.Secret
	// RedirectURL is this installation's own callback, derived from its base URL. It is never
	// taken from a request: a redirect target a caller chooses is how authorization codes end
	// up somewhere else.
	RedirectURL string
}

// Authorization is what one sign-in needs on its way out. The three unguessable values are drawn
// by the application layer through the entropy port (rule 4), never by the adapter, so a test can
// fix them and production draws from crypto/rand.
type Authorization struct {
	// State is the flow's handle, presented back at the callback.
	State string
	// Nonce is minted into the authorization request and must come back inside the ID token.
	Nonce string
	// CodeVerifier is PKCE's secret half. The adapter derives the challenge from it - which
	// method that is, is the library's business rather than the core's.
	CodeVerifier string
	// LoginHint is an address to save somebody typing it twice, and nothing more: it never
	// decides which account is signed in.
	LoginHint string
}

// Exchange is what the callback carries into the token endpoint.
type Exchange struct {
	Code         string
	CodeVerifier string
	// Nonce is what the ID token's claim must equal. Compared inside the adapter, because a
	// verification split across two layers is a verification somebody eventually skips half of.
	Nonce string
}

// Identity is what a fully verified ID token says about a person - and only what this product
// has a use for. A claim nobody consumes is a claim nobody has to reason about.
type Identity struct {
	// Subject is the provider's stable identifier for the person: what lands in
	// `account.external_subject` and what finds them again on every later sign-in.
	Subject string
	// Email is the address the token carried, empty when it carried none.
	Email string
	// EmailVerified is the provider's own claim about that address. Linking an arriving subject
	// to an existing local account depends on it: an unverified address is an assertion, and
	// acting on it hands somebody an account by typing.
	EmailVerified bool
	// DisplayName is what to call them until they say otherwise.
	DisplayName string
}

// Port is the relying party.
type Port interface {
	// Check confirms an issuer answers discovery and agrees with its own metadata, so that a
	// configuration that cannot work is refused where somebody is looking at the form rather
	// than by the first colleague who tries to sign in.
	Check(ctx context.Context, issuer string) error

	// AuthorizationURL is where to send the browser. It performs discovery for the issuer, so
	// an unreachable provider is refused here rather than after somebody has left the page.
	AuthorizationURL(ctx context.Context, cfg Config, auth Authorization) (string, error)

	// Exchange trades the authorization code for an identity, and answers only once the ID
	// token has passed every check T-13 names: the signature against the provider's current
	// keys, `iss`, `aud`, `exp`, the nonce, an algorithm allowlist without `none`, and a clock
	// skew no wider than a minute.
	Exchange(ctx context.Context, cfg Config, exchange Exchange) (Identity, error)
}
