// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package identity holds the model of who acts: accounts, their credentials, and the tokens that
// stand in for them when nobody is at a keyboard.
package identity

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TokenPrefix marks a personal access token. It is fixed and public on purpose: GitHub's and
// GitLab's secret scanning match on a prefix, so a token pasted into a commit is found by
// somebody other than an attacker (security.md §5).
const TokenPrefix = "hbt_pat_"

// TokenSecretBytes is the entropy of the secret half. 32 bytes is far past anything guessable and
// keeps the token short enough to paste.
const TokenSecretBytes = 32

// tokenSecretLength is what those bytes come to in base64url without padding.
const tokenSecretLength = 43

// tenantHexLength is a UUID without its hyphens.
const tenantHexLength = 32

// Token is a presented credential: `hbt_pat_<tenant>_<secret>`.
//
// The tenant travels in the token because the lookup needs it before it can happen. Every table
// is behind row level security, `access_token` included, so a query for the token's hash returns
// nothing until a tenant context is set - and the only place that context can come from, for a
// non-interactive credential, is the credential itself (multi-tenancy.md §3, "token claim / PAT
// binding"). Claiming the wrong tenant gains nothing: the hash is unique across the installation,
// so a token quoting a tenant it does not belong to simply is not found.
type Token struct {
	tenantID shared.ID
	raw      string
}

// ParseToken reads a presented credential. It checks the shape and nothing else - whether the
// token exists, is revoked, or has expired is a question for the repository and the use case.
//
// Every failure is the same error. A parser that distinguished "wrong prefix" from "wrong length"
// would be an oracle for guessing the format, and the format is the one part of a token an
// attacker does not have to guess at.
func ParseToken(raw string) (Token, error) {
	body, found := strings.CutPrefix(raw, TokenPrefix)
	if !found {
		return Token{}, errTokenMalformed()
	}

	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != tokenSecretLength {
		return Token{}, errTokenMalformed()
	}
	if !isBase64URL(secret) {
		return Token{}, errTokenMalformed()
	}

	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return Token{}, errTokenMalformed()
	}
	return Token{tenantID: tenantID, raw: raw}, nil
}

// NewToken builds a token for a tenant from freshly drawn randomness. The bytes come from the
// caller rather than from crypto/rand here, because the domain draws nothing itself (rule 4) -
// and because a test that cannot fix the secret cannot assert on the result.
func NewToken(tenantID shared.ID, secret []byte) (Token, error) {
	if tenantID.IsZero() {
		return Token{}, shared.ErrValidation.WithDetail("access.token_tenant_missing")
	}
	if len(secret) != TokenSecretBytes {
		return Token{}, shared.ErrValidation.WithDetail("access.token_secret_short")
	}

	raw := TokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return Token{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the token is bound to, and therefore the tenant its lookup runs in.
func (t Token) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, which is what gets hashed. The whole string rather than only
// its secret half: the tenant part is then covered by the hash too, so a token cannot be
// rewritten to point at another tenant and still match.
//
// It is deliberately not called String and there is no String method, so that a token cannot end
// up in a log line by way of %v (rule 10, security.md §5).
func (t Token) Secret() string { return t.raw }

// IsZero reports the empty token.
func (t Token) IsZero() bool { return t.raw == "" }

func errTokenMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("access.token_malformed")
}

// tenantFromHex turns 32 hex digits back into the canonical 8-4-4-4-12 form.
func tenantFromHex(hex string) (shared.ID, error) {
	for _, c := range hex {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", errTokenMalformed()
		}
	}
	var b strings.Builder
	b.Grow(36)
	for i, group := range []int{8, 4, 4, 4, 12} {
		if i > 0 {
			b.WriteByte('-')
		}
		b.WriteString(hex[:group])
		hex = hex[group:]
	}
	return shared.ParseID(b.String())
}

func isBase64URL(s string) bool {
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
