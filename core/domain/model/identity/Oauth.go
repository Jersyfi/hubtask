// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The OAuth2 provider's shapes (H-05, api-guidelines.md §7): authorization code + PKCE only,
// issuing H-01's own token shapes with the grant as their leash.

// OauthCodePrefix and OauthClientSecretPrefix mark the two credentials this side mints, with
// TokenPrefix's reasoning: public markers for scanning, and the tenant inside because the public
// token endpoint needs a context before any lookup can happen.
//
//nolint:gosec // G101: public format markers, not credentials
const (
	OauthCodePrefix         = "hbt_oac_"
	OauthClientSecretPrefix = "hbt_ocs_"
)

// ParseOauthCode and NewOauthCode, ParseToken's discipline.
func ParseOauthCode(raw string) (Token, error) { return parsePrefixed(raw, OauthCodePrefix) }

func NewOauthCode(tenantID shared.ID, secret []byte) (Token, error) {
	return newPrefixed(OauthCodePrefix, tenantID, secret)
}

// NewOauthClientSecret mints a confidential client's credential. There is no parse: the secret
// is presented beside the client id and compared as a hash, never resolved on its own.
func NewOauthClientSecret(tenantID shared.ID, secret []byte) (Token, error) {
	return newPrefixed(OauthClientSecretPrefix, tenantID, secret)
}

// OauthCodeLifetime is how long an authorization code may sit between the consent and the
// exchange. Two minutes: an app exchanges immediately, and a code that could wait an hour is a
// credential lying around.
const OauthCodeLifetime = 2 * time.Minute

// MaxOauthRedirectURIs bounds a registration; nobody legitimate needs more.
const MaxOauthRedirectURIs = 10

// OauthClient is a registered third-party app.
type OauthClient struct {
	ID       shared.ID
	TenantID shared.ID
	// Name is what people read on the consent screen and in their grant list.
	Name string
	// Confidential says whether the app can keep a secret on a server. A public one cannot, has
	// none, and must bring PKCE to every authorization.
	Confidential bool
	// RedirectURIs are matched exactly, byte for byte (RFC 6749 §3.1.2's registered-URI rule,
	// taken at its strictest): a prefix or wildcard match is how codes land on
	// attacker-controlled pages.
	RedirectURIs []string
	CreatedAt    time.Time
	CreatedBy    shared.ID
}

// NewOauthClientInput is what registering one needs.
type NewOauthClientInput struct {
	ID           shared.ID
	TenantID     shared.ID
	Name         string
	Confidential bool
	RedirectURIs []string
	CreatedBy    shared.ID
	Now          time.Time
}

// NewOauthClient validates the registration.
func NewOauthClient(in NewOauthClientInput) (OauthClient, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return OauthClient{}, shared.ErrInternal.WithDetail("oauth.client_incomplete")
	}
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return OauthClient{}, fieldError("/name", "oauth.client_name_required")
	case utf8.RuneCountInString(name) > 200:
		return OauthClient{}, fieldError("/name", "oauth.client_name_too_long")
	}

	if len(in.RedirectURIs) == 0 {
		return OauthClient{}, fieldError("/redirect_uris", "oauth.redirect_uri_required")
	}
	if len(in.RedirectURIs) > MaxOauthRedirectURIs {
		return OauthClient{}, fieldError("/redirect_uris", "oauth.redirect_uris_too_many")
	}
	uris := make([]string, 0, len(in.RedirectURIs))
	for _, raw := range in.RedirectURIs {
		uri := strings.TrimSpace(raw)
		parsed, err := url.Parse(uri)
		// Absolute, no fragment (RFC 6749 §3.1.2): a fragment never reaches the server, so a
		// registered one is a promise nothing could keep.
		if err != nil || !parsed.IsAbs() || parsed.Fragment != "" || parsed.Host == "" && parsed.Opaque == "" {
			return OauthClient{}, shared.ErrValidation.
				WithDetail("oauth.redirect_uri_invalid").
				WithParams(map[string]string{"uri": uri}).
				WithFields(shared.FieldError{Path: "/redirect_uris", Code: "oauth.redirect_uri_invalid"})
		}
		if slices.Contains(uris, uri) {
			continue
		}
		uris = append(uris, uri)
	}

	return OauthClient{
		ID: in.ID, TenantID: in.TenantID, Name: name,
		Confidential: in.Confidential, RedirectURIs: uris,
		CreatedAt: in.Now.UTC(), CreatedBy: in.CreatedBy,
	}, nil
}

// AllowsRedirect is the exact match, and nothing cleverer.
func (c OauthClient) AllowsRedirect(uri string) bool {
	return slices.Contains(c.RedirectURIs, uri)
}

// OauthGrant is what a person allowed one app: the row they see and revoke beside their
// sessions.
type OauthGrant struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	ClientID  shared.ID
	Scopes    []string
	CreatedAt time.Time
	RevokedAt time.Time
}

// Verify decides whether sessions may still be issued under the grant.
func (g OauthGrant) Verify() error {
	if !g.RevokedAt.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("oauth.grant_revoked")
	}
	return nil
}

// OauthCode is the stored half of a single-use authorization code.
type OauthCode struct {
	ID          shared.ID
	TenantID    shared.ID
	ClientID    shared.ID
	AccountID   shared.ID
	GrantID     shared.ID
	Challenge   string
	RedirectURI string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ConsumedAt  time.Time
}

// The PKCE verifier's bounds, RFC 7636 §4.1.
const (
	MinPKCEVerifierLength = 43
	MaxPKCEVerifierLength = 128
)

// CheckPKCEChallenge validates what an authorization registers: the base64url form of a SHA-256
// digest, 43 characters, no padding.
func CheckPKCEChallenge(challenge string) error {
	if len(challenge) != 43 || !isBase64URL(challenge) {
		return shared.ErrValidation.
			WithDetail("oauth.pkce_challenge_invalid").
			WithFields(shared.FieldError{Path: "/code_challenge", Code: "oauth.pkce_challenge_invalid"})
	}
	return nil
}

// VerifyPKCE judges the exchange's verifier against the authorization's challenge (RFC 7636
// §4.6, S256 only): BASE64URL(SHA256(verifier)) has to equal the challenge, compared in
// constant time. `plain` is deliberately not implemented - it would put the verifier on the
// wire twice and prove nothing.
func VerifyPKCE(verifier, challenge string) bool {
	if len(verifier) < MinPKCEVerifierLength || len(verifier) > MaxPKCEVerifierLength {
		return false
	}
	digest := sha256.Sum256([]byte(verifier))
	derived := base64.RawURLEncoding.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(derived), []byte(challenge)) == 1
}
