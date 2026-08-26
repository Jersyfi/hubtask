// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package integration holds the model of what reaches into this system from outside it, or lets
// something outside read it: today the calendar feed, later the webhooks that share its table
// section in the schema.
package integration

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// FeedTokenPrefix marks a calendar feed token. Public and fixed for the reason the personal
// access token's prefix is: secret scanning matches on a prefix, so a feed URL pasted into an
// issue is found by somebody other than an attacker (security.md §5).
//
//nolint:gosec // G101: a public marker, not a credential - what it prefixes is the credential
const FeedTokenPrefix = "hbt_cal_"

// FeedTokenSecretBytes is the entropy of the secret half, and the same 32 bytes the access token
// draws. This token is the whole of the authorisation on an unauthenticated route, so it gets no
// less than the credential that at least travels in a header.
const FeedTokenSecretBytes = 32

// feedSecretLength is what those bytes come to in base64url without padding.
const feedSecretLength = 43

// tenantHexLength is a UUID without its hyphens.
const tenantHexLength = 32

// FeedToken is a presented feed credential: `hbt_cal_<tenant>_<secret>`.
//
// The tenant travels inside it for the reason it travels inside a personal access token, and here
// there is not even a header to argue about: `calendar_feed` is behind row level security like
// every other table, so the lookup by hash returns nothing until a tenant context is set, and the
// only honest source of that context on a route with no authentication is the credential itself
// (multi-tenancy.md §2.2, §3). A token naming a tenant it does not belong to gains nothing - the
// hash covers the whole string, tenant half included, and is unique across the installation.
//
// The shape is the access token's on purpose. Two credential formats where one would do is two
// parsers, two scanning patterns and two chances to get the tenant handling wrong; what keeps them
// apart is the prefix and, where it matters, the purpose label the hash is derived under.
type FeedToken struct {
	tenantID shared.ID
	raw      string
}

// ParseFeedToken reads a presented token. It checks the shape and nothing else: whether the feed
// exists, is revoked, or still serves a view is decided further in.
//
// Every failure is the same error, for the reason ParseToken's are: a parser that said which
// check failed would be an oracle for the format, and the format is the one part of a token
// somebody guessing does not have to guess at.
func ParseFeedToken(raw string) (FeedToken, error) {
	body, found := strings.CutPrefix(raw, FeedTokenPrefix)
	if !found {
		return FeedToken{}, errFeedTokenMalformed()
	}

	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != feedSecretLength {
		return FeedToken{}, errFeedTokenMalformed()
	}
	if !isBase64URL(secret) {
		return FeedToken{}, errFeedTokenMalformed()
	}

	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return FeedToken{}, errFeedTokenMalformed()
	}
	return FeedToken{tenantID: tenantID, raw: raw}, nil
}

// NewFeedToken mints one from freshly drawn randomness. The bytes come from the caller because
// the domain draws nothing itself (rule 4), and because a test that cannot fix the secret cannot
// assert on the result.
func NewFeedToken(tenantID shared.ID, secret []byte) (FeedToken, error) {
	if tenantID.IsZero() {
		return FeedToken{}, shared.ErrValidation.WithDetail("calendar.token_tenant_missing")
	}
	if len(secret) != FeedTokenSecretBytes {
		return FeedToken{}, shared.ErrValidation.WithDetail("calendar.token_secret_short")
	}

	raw := FeedTokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return FeedToken{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the token is bound to, and therefore the tenant its lookup runs in.
func (t FeedToken) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, which is what gets hashed - the tenant half included, so a
// token cannot be rewritten to name another tenant and still match.
//
// Reading it is a deliberate act with a name, and every other way of printing this type is masked
// below. That matters more here than for any other credential in the system: this is the only one
// that travels in a URL, where an access log, a proxy and a browser history all keep a copy of
// whatever is printed (rule 10, security.md §4 T-21).
func (t FeedToken) Secret() string { return t.raw }

// String, GoString and MarshalText mask. Leaving them off would not be enough: %v over a struct
// prints its unexported fields, so a token handed to a log line by mistake would print itself in
// full. The prefix is kept because it is public by design and says what was redacted.
func (t FeedToken) String() string   { return t.masked() }
func (t FeedToken) GoString() string { return t.masked() }

// MarshalText covers the encoders as well - a token that reached a JSON body by mistake writes
// the mask rather than the credential.
func (t FeedToken) MarshalText() ([]byte, error) { return []byte(t.masked()), nil }

func (t FeedToken) masked() string {
	if t.raw == "" {
		return ""
	}
	return FeedTokenPrefix + "<redacted>"
}

// IsZero reports the empty token.
func (t FeedToken) IsZero() bool { return t.raw == "" }

func errFeedTokenMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("calendar.token_malformed")
}

// tenantFromHex turns 32 hex digits back into the canonical 8-4-4-4-12 form.
func tenantFromHex(hex string) (shared.ID, error) {
	for _, c := range hex {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", errFeedTokenMalformed()
		}
	}
	return shared.ParseID(hex[0:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" +
		hex[16:20] + "-" + hex[20:32])
}

// isBase64URL reports whether every character belongs to the unpadded base64url alphabet. A
// length check alone would let a token carry a character that decodes to something else.
func isBase64URL(s string) bool {
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
