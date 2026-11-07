// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// FormLinkTokenPrefix marks a collection's public form link, for secret scanning as the feed's prefix does.
const FormLinkTokenPrefix = "hbt_form_"

// FormLinkTokenSecretBytes is the secret half's entropy, the feed token's: the token is the whole
// authorisation of an unauthenticated route.
const FormLinkTokenSecretBytes = 32

// FormLinkToken is a presented public form link: `hbt_form_<tenant>_<secret>`, the feed token's
// shape for the feed token's reason - the lookup needs the tenant before row level security lets
// it find anything, and the credential is the only honest source of one (multi-tenancy.md §2.2).
type FormLinkToken struct {
	tenantID shared.ID
	raw      string
}

// ParseFormLinkToken reads a presented token; every malformed one is the same error.
func ParseFormLinkToken(raw string) (FormLinkToken, error) {
	body, found := strings.CutPrefix(raw, FormLinkTokenPrefix)
	if !found {
		return FormLinkToken{}, errFormLinkMalformed()
	}
	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != feedSecretLength || !isBase64URL(secret) {
		return FormLinkToken{}, errFormLinkMalformed()
	}
	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return FormLinkToken{}, errFormLinkMalformed()
	}
	return FormLinkToken{tenantID: tenantID, raw: raw}, nil
}

// NewFormLinkToken mints one from randomness the caller drew (rule 4).
func NewFormLinkToken(tenantID shared.ID, secret []byte) (FormLinkToken, error) {
	if tenantID.IsZero() {
		return FormLinkToken{}, shared.ErrValidation.WithDetail("public_forms.link_tenant_missing")
	}
	if len(secret) != FormLinkTokenSecretBytes {
		return FormLinkToken{}, shared.ErrValidation.WithDetail("public_forms.link_secret_short")
	}
	raw := FormLinkTokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return FormLinkToken{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the lookup runs in.
func (t FormLinkToken) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, tenant half included, which is what gets hashed.
func (t FormLinkToken) Secret() string { return t.raw }

// String, GoString and MarshalText mask: the token travels in a URL (rule 10, T-21).
func (t FormLinkToken) String() string               { return t.masked() }
func (t FormLinkToken) GoString() string             { return t.masked() }
func (t FormLinkToken) MarshalText() ([]byte, error) { return []byte(t.masked()), nil }

func (t FormLinkToken) masked() string {
	if t.raw == "" {
		return ""
	}
	return FormLinkTokenPrefix + "<redacted>"
}

// IsZero reports the empty token.
func (t FormLinkToken) IsZero() bool { return t.raw == "" }

func errFormLinkMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("public_forms.link_malformed")
}
