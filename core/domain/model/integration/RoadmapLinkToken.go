// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RoadmapLinkTokenPrefix marks a portfolio roadmap's public link, for secret scanning as the feed's prefix does.
const RoadmapLinkTokenPrefix = "hbt_road_"

// RoadmapLinkTokenSecretBytes is the secret half's entropy, the feed token's: the token is the whole
// authorisation of an unauthenticated route.
const RoadmapLinkTokenSecretBytes = 32

// RoadmapLinkToken is a presented public roadmap link: `hbt_road_<tenant>_<secret>`, the feed token's
// shape for the feed token's reason - the lookup needs the tenant before row level security lets
// it find anything, and the credential is the only honest source of one (multi-tenancy.md §2.2).
type RoadmapLinkToken struct {
	tenantID shared.ID
	raw      string
}

// ParseRoadmapLinkToken reads a presented token; every malformed one is the same error.
func ParseRoadmapLinkToken(raw string) (RoadmapLinkToken, error) {
	body, found := strings.CutPrefix(raw, RoadmapLinkTokenPrefix)
	if !found {
		return RoadmapLinkToken{}, errRoadmapLinkMalformed()
	}
	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != feedSecretLength || !isBase64URL(secret) {
		return RoadmapLinkToken{}, errRoadmapLinkMalformed()
	}
	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return RoadmapLinkToken{}, errRoadmapLinkMalformed()
	}
	return RoadmapLinkToken{tenantID: tenantID, raw: raw}, nil
}

// NewRoadmapLinkToken mints one from randomness the caller drew (rule 4).
func NewRoadmapLinkToken(tenantID shared.ID, secret []byte) (RoadmapLinkToken, error) {
	if tenantID.IsZero() {
		return RoadmapLinkToken{}, shared.ErrValidation.WithDetail("portfolios.link_tenant_missing")
	}
	if len(secret) != RoadmapLinkTokenSecretBytes {
		return RoadmapLinkToken{}, shared.ErrValidation.WithDetail("portfolios.link_secret_short")
	}
	raw := RoadmapLinkTokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return RoadmapLinkToken{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the lookup runs in.
func (t RoadmapLinkToken) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, tenant half included, which is what gets hashed.
func (t RoadmapLinkToken) Secret() string { return t.raw }

// String, GoString and MarshalText mask: the token travels in a URL (rule 10, T-21).
func (t RoadmapLinkToken) String() string               { return t.masked() }
func (t RoadmapLinkToken) GoString() string             { return t.masked() }
func (t RoadmapLinkToken) MarshalText() ([]byte, error) { return []byte(t.masked()), nil }

func (t RoadmapLinkToken) masked() string {
	if t.raw == "" {
		return ""
	}
	return RoadmapLinkTokenPrefix + "<redacted>"
}

// IsZero reports the empty token.
func (t RoadmapLinkToken) IsZero() bool { return t.raw == "" }

func errRoadmapLinkMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("portfolios.link_malformed")
}
