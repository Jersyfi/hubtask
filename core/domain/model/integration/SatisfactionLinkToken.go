// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SatisfactionLinkTokenPrefix marks a satisfaction link, for secret scanning as the feed's prefix does.
//
//nolint:gosec // G101: a public marker, not a credential - what it prefixes is the credential
const SatisfactionLinkTokenPrefix = "hbt_rate_"

// SatisfactionLinkTokenSecretBytes is the secret half's entropy, the feed token's.
const SatisfactionLinkTokenSecretBytes = 32

// SatisfactionLinkToken is a presented satisfaction link (UC-SVC-07 check 6):
// `hbt_rate_<tenant>_<secret>`, the form link's shape for its reason - the lookup needs the
// tenant before row level security lets it find anything (multi-tenancy.md §2.2).
type SatisfactionLinkToken struct {
	tenantID shared.ID
	raw      string
}

// ParseSatisfactionLinkToken reads a presented token; every malformed one is the same error.
func ParseSatisfactionLinkToken(raw string) (SatisfactionLinkToken, error) {
	body, found := strings.CutPrefix(raw, SatisfactionLinkTokenPrefix)
	if !found {
		return SatisfactionLinkToken{}, errSatisfactionLinkMalformed()
	}
	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != feedSecretLength || !isBase64URL(secret) {
		return SatisfactionLinkToken{}, errSatisfactionLinkMalformed()
	}
	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return SatisfactionLinkToken{}, errSatisfactionLinkMalformed()
	}
	return SatisfactionLinkToken{tenantID: tenantID, raw: raw}, nil
}

// NewSatisfactionLinkToken mints one from randomness the caller drew (rule 4).
func NewSatisfactionLinkToken(tenantID shared.ID, secret []byte) (SatisfactionLinkToken, error) {
	if tenantID.IsZero() {
		return SatisfactionLinkToken{}, shared.ErrValidation.WithDetail("satisfaction.link_tenant_missing")
	}
	if len(secret) != SatisfactionLinkTokenSecretBytes {
		return SatisfactionLinkToken{}, shared.ErrValidation.WithDetail("satisfaction.link_secret_short")
	}
	raw := SatisfactionLinkTokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return SatisfactionLinkToken{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the lookup runs in.
func (t SatisfactionLinkToken) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, tenant half included, which is what gets hashed.
func (t SatisfactionLinkToken) Secret() string { return t.raw }

// String, GoString and MarshalText mask: the token travels in a mail and a URL (rule 10, T-21).
func (t SatisfactionLinkToken) String() string               { return t.masked() }
func (t SatisfactionLinkToken) GoString() string             { return t.masked() }
func (t SatisfactionLinkToken) MarshalText() ([]byte, error) { return []byte(t.masked()), nil }

func (t SatisfactionLinkToken) masked() string {
	if t.raw == "" {
		return ""
	}
	return SatisfactionLinkTokenPrefix + "<redacted>"
}

// IsZero reports the empty token.
func (t SatisfactionLinkToken) IsZero() bool { return t.raw == "" }

func errSatisfactionLinkMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("satisfaction.link_malformed")
}
