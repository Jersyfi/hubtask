// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The OAuth2 provider's shapes (identity.md §15.3, api-guidelines.md §7): authorization code + PKCE
// only, issuing the session's own token shapes (identity.md §14.1) with the grant as their leash.

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
	// AppPlatform marks a row of the built-in client of the installed apps; empty for a
	// registered client. Such a row is one consent on one device, and Name is that device's.
	AppPlatform AppPlatform
}

// AppClientID is the built-in client of the installed apps (identity.md §15.3, ADR-0096): the same
// identifier in every workspace of every installation, so an app needs nobody to register it.
const AppClientID shared.ID = "0b5e1a7f-3c2d-4e8a-9f61-7a4c2e9d1b30"

// AppPlatform is the system an installed app runs on, which decides its one kind of redirect.
type AppPlatform string

const (
	AppWindows AppPlatform = "windows"
	AppLinux   AppPlatform = "linux"
	AppAndroid AppPlatform = "android"
)

// AppRedirectAndroid is the Android app's redirect, a private-use scheme of the app's own
// identifier (RFC 8252 §7.1).
const AppRedirectAndroid = "eu.hubtask.app:/oauth"

// appRedirectPath is the desktop listener's one path.
const appRedirectPath = "/oauth"

// MaxAppDeviceName bounds the device's name, as the device list bounds it.
const MaxAppDeviceName = 120

// ParseAppPlatform accepts the three systems the apps ship for, nothing else.
func ParseAppPlatform(raw string) (AppPlatform, error) {
	switch platform := AppPlatform(raw); platform {
	case AppWindows, AppLinux, AppAndroid:
		return platform, nil
	}
	return "", fieldError("/platform", "oauth.app_platform_invalid")
}

// AllowsAppRedirect is the built-in client's whole redirect rule: on the desktop a loopback
// address by IP literal on any port with the listener's path (RFC 8252 §7.3) - never `localhost`,
// which a hosts file can point elsewhere -, on Android exactly AppRedirectAndroid.
func (p AppPlatform) AllowsAppRedirect(uri string) bool {
	if p == AppAndroid {
		return uri == AppRedirectAndroid
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != appRedirectPath {
		return false
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "::1" {
		return false
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	}
	return p == AppWindows || p == AppLinux
}

// NewAppClientInput is what one consent through the built-in client needs.
type NewAppClientInput struct {
	ID          shared.ID
	TenantID    shared.ID
	Platform    string
	Device      string
	RedirectURI string
	CreatedBy   shared.ID
	Now         time.Time
	Text        text.Normalizer
}

// NewAppClient is the built-in client's row for one consent: public, the device's name, and the
// one redirect the consent named, so the exchange's exact match holds as for any client.
func NewAppClient(in NewAppClientInput) (OauthClient, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return OauthClient{}, shared.ErrInternal.WithDetail("oauth.client_incomplete")
	}
	platform, err := ParseAppPlatform(in.Platform)
	if err != nil {
		return OauthClient{}, err
	}
	device, err := shared.NFC(strings.TrimSpace(in.Device), in.Text)
	if err != nil {
		return OauthClient{}, err
	}
	switch {
	case device == "":
		return OauthClient{}, fieldError("/device", "oauth.app_device_required")
	case utf8.RuneCountInString(device) > MaxAppDeviceName:
		return OauthClient{}, fieldError("/device", "oauth.app_device_too_long")
	}
	if !platform.AllowsAppRedirect(in.RedirectURI) {
		return OauthClient{}, shared.ErrValidation.
			WithDetail("oauth.redirect_uri_mismatch").
			WithFields(shared.FieldError{Path: "/redirect_uri", Code: "oauth.redirect_uri_mismatch"})
	}
	return OauthClient{
		ID: in.ID, TenantID: in.TenantID, Name: device,
		RedirectURIs: []string{in.RedirectURI},
		CreatedAt:    in.Now.UTC(), CreatedBy: in.CreatedBy,
		AppPlatform: platform,
	}, nil
}

// IsApp reports a row of the built-in client.
func (c OauthClient) IsApp() bool { return c.AppPlatform != "" }

// NewOauthClientInput is what registering one needs.
type NewOauthClientInput struct {
	ID           shared.ID
	TenantID     shared.ID
	Name         string
	Confidential bool
	RedirectURIs []string
	CreatedBy    shared.ID
	Now          time.Time

	// Text brings the name to normal form C before it is bounded and stored (i18n-l10n.md §5);
	// work.NewWorkItemInput says why it is handed in.
	Text text.Normalizer
}

// NewOauthClient validates the registration.
func NewOauthClient(in NewOauthClientInput) (OauthClient, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return OauthClient{}, shared.ErrInternal.WithDetail("oauth.client_incomplete")
	}
	name, err := shared.NFC(strings.TrimSpace(in.Name), in.Text)
	if err != nil {
		return OauthClient{}, err
	}
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
