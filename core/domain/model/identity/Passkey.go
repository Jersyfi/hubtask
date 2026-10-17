// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Passkeys and security keys (identity.md §23): to the server both are a WebAuthn credential, a
// public key the account holds. What one proves is read from each ceremony, not from what the
// person calls it (§23.1).

const (
	// MaxPasskeys is how many an account holds beside at most one authenticator app (§23.2): one
	// for each device and a spare, and a bound on what a sign-in offers.
	MaxPasskeys = 10
	// MaxPasskeyNameLength bounds the person's name for one, in characters.
	MaxPasskeyNameLength = 64
	// PasskeyUserHandleBytes is the opaque value an authenticator stores for the account.
	PasskeyUserHandleBytes = 32
	// PasskeyRegistrationLifetime is how long a registration's challenge is accepted: as long as a
	// sign-in's second step, long enough to find the key in a drawer.
	PasskeyRegistrationLifetime = 5 * time.Minute
)

// PasskeyCredential is one credential an account holds.
type PasskeyCredential struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	// CredentialID is the authenticator's identifier for it, what a ceremony names it by.
	CredentialID []byte
	// PublicKey is the COSE key exactly as the authenticator sent it.
	PublicKey []byte
	Algorithm int64
	SignCount uint32
	// UserHandle is what the authenticator stores for the account; never the address.
	UserHandle     []byte
	BackupEligible bool
	BackedUp       bool
	Transports     []string
	// Name is the person's own word for it - user content, shown on their profile and nowhere
	// else (rule 10).
	Name       string
	CreatedAt  time.Time
	LastUsedAt time.Time
	// RPID is the host it was made for; empty for one made before it was recorded, which counts as
	// the workspace's own.
	RPID string
}

// BelongsTo is whether the credential answers at the relying party rpID: one made elsewhere - the
// workspace moved, or the account was restored into another - can never sign here, so it counts as
// no factor (P-16).
func (c PasskeyCredential) BelongsTo(rpID string) bool {
	return c.RPID == "" || c.RPID == rpID
}

// PasskeyName checks the person's name for a credential: trimmed, one to MaxPasskeyNameLength
// characters, no control character and no bidirectional embedding, override or isolate - each
// would let a name rearrange the line it is shown on. A joiner stays allowed: emoji sequences use it.
func PasskeyName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxPasskeyNameLength {
		return "", errPasskeyName()
	}
	for _, r := range name {
		if unicode.IsControl(r) || (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069') {
			return "", errPasskeyName()
		}
	}
	return name, nil
}

func errPasskeyName() error {
	return shared.ErrValidation.WithDetail("auth.passkey_name_invalid").
		WithParams(map[string]string{"max": "64"})
}

// PasskeyRelyingParty is the relying party a workspace's ceremonies name (identity.md §23.3): the RP
// ID is the workspace's canonical host - never a parent of it, so one workspace's passkeys are never
// offered in another (P-01) - and the origin is the installation's public scheme with that host and
// the public port where it is not the scheme's default. The host comes in ASCII form, as a browser
// compares it.
//
// False where there is no host, which is an installation without a public address: nothing can be
// bound to it, so nothing is offered.
func PasskeyRelyingParty(host, scheme, port string) (rpID, origin string, ok bool) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	scheme = strings.ToLower(scheme)
	if host == "" || (scheme != "https" && scheme != "http") {
		return "", "", false
	}
	origin = scheme + "://" + host
	schemesOwn := (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
	if port != "" && !schemesOwn {
		origin += ":" + port
	}
	return host, origin, true
}

// PasskeyChallengeTokenPrefix marks a registration's challenge as a token of its own kind. The
// challenge is public - it travels to the page, the browser and the authenticator - so it is held to
// a prefix no other door parses: hashed under it, it can never equal a pending sign-in's token.
const PasskeyChallengeTokenPrefix = "hbt_pkr_" //nolint:gosec // G101: a public format marker, not a credential

// PendingPasskeyRegister is a registration's challenge waiting for the ceremony (identity.md §23.3):
// bound to the session that started it after a step-up, five minutes, single use.
const PendingPasskeyRegister PendingPurpose = "PASSKEY_REGISTER"

// NewPasskeyChallengeToken is the challenge as the token its row is found by. The secret half is the
// challenge itself, in the unpadded base64url a client data carries it in.
func NewPasskeyChallengeToken(tenantID shared.ID, challenge []byte) (Token, error) {
	return newPrefixed(PasskeyChallengeTokenPrefix, tenantID, challenge)
}

// PasskeyChallenge is a registration waiting for its ceremony.
type PasskeyChallenge struct {
	ID        shared.ID
	AccountID shared.ID
	// SessionID is the session that started it, the only one that may finish it.
	SessionID shared.ID
	// UserHandle is the handle the new credential will carry.
	UserHandle []byte
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// PasskeySignInTokenPrefix marks a passkey sign-in's challenge, a token of its own kind for the
// registration challenge's reason: public, so held to a prefix no other door parses.
const PasskeySignInTokenPrefix = "hbt_pks_" //nolint:gosec // G101: a public format marker, not a credential

// PendingPasskeySignIn is a passkey sign-in's challenge (identity.md §23.4): no account, five
// minutes, single use.
const PendingPasskeySignIn PendingPurpose = "PASSKEY_SIGN_IN"

// NewPasskeySignInToken is a sign-in's challenge as the token its row is found by.
func NewPasskeySignInToken(tenantID shared.ID, challenge []byte) (Token, error) {
	return newPrefixed(PasskeySignInTokenPrefix, tenantID, challenge)
}
