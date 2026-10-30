// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PageLinkTokenPrefix marks a page's public link, for secret scanning as the feed's prefix does.
//
//nolint:gosec // G101: a public marker, not a credential - what it prefixes is the credential
const PageLinkTokenPrefix = "hbt_page_"

// PageLinkTokenSecretBytes is the secret half's entropy, the feed token's: the token is the whole
// authorisation of an unauthenticated route.
const PageLinkTokenSecretBytes = 32

// PageLinkToken is a presented public page link: `hbt_page_<tenant>_<secret>`, the feed token's
// shape for the feed token's reason - the lookup needs the tenant before row level security lets
// it find anything, and the credential is the only honest source of one (multi-tenancy.md §2.2).
type PageLinkToken struct {
	tenantID shared.ID
	raw      string
}

// ParsePageLinkToken reads a presented token; every malformed one is the same error.
func ParsePageLinkToken(raw string) (PageLinkToken, error) {
	body, found := strings.CutPrefix(raw, PageLinkTokenPrefix)
	if !found {
		return PageLinkToken{}, errPageLinkMalformed()
	}
	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != feedSecretLength || !isBase64URL(secret) {
		return PageLinkToken{}, errPageLinkMalformed()
	}
	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return PageLinkToken{}, errPageLinkMalformed()
	}
	return PageLinkToken{tenantID: tenantID, raw: raw}, nil
}

// NewPageLinkToken mints one from randomness the caller drew (rule 4).
func NewPageLinkToken(tenantID shared.ID, secret []byte) (PageLinkToken, error) {
	if tenantID.IsZero() {
		return PageLinkToken{}, shared.ErrValidation.WithDetail("pages.link_tenant_missing")
	}
	if len(secret) != PageLinkTokenSecretBytes {
		return PageLinkToken{}, shared.ErrValidation.WithDetail("pages.link_secret_short")
	}
	raw := PageLinkTokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return PageLinkToken{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the lookup runs in.
func (t PageLinkToken) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, tenant half included, which is what gets hashed.
func (t PageLinkToken) Secret() string { return t.raw }

// String, GoString and MarshalText mask: the token travels in a URL (rule 10, T-21).
func (t PageLinkToken) String() string               { return t.masked() }
func (t PageLinkToken) GoString() string             { return t.masked() }
func (t PageLinkToken) MarshalText() ([]byte, error) { return []byte(t.masked()), nil }

func (t PageLinkToken) masked() string {
	if t.raw == "" {
		return ""
	}
	return PageLinkTokenPrefix + "<redacted>"
}

// IsZero reports the empty token.
func (t PageLinkToken) IsZero() bool { return t.raw == "" }

func errPageLinkMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("pages.link_malformed")
}
