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
