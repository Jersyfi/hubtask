// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"encoding/base64"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// InboundTokenPrefix marks the address of an INBOUND_WEBHOOK rule (G-08, automation.md §1.1).
//
// Public and fixed for the reason the feed token's prefix is: secret scanning matches on a prefix,
// so an inbound URL pasted into an issue is found by somebody other than an attacker
// (security.md §5). Its own prefix rather than the calendar feed's, so that a scanner and a reader
// can both tell at a glance what a leaked string opens.
//
//nolint:gosec // G101: a public marker, not a credential - what it prefixes is the credential
const InboundTokenPrefix = "hbt_hook_"

// InboundTokenSecretBytes is the entropy of the secret half. The same 32 bytes the calendar feed
// draws, and for a stronger reason: this token is the whole of the authorisation on a route that
// *starts a run* rather than one that reads a list.
const InboundTokenSecretBytes = 32

// MaxInboundPayloadBytes is how much body an inbound delivery may carry.
//
// Far below the request limit that refuses a 2 MB body at the boundary, and deliberately: this
// document becomes a CEL activation, and 64 KiB of nested JSON is already more than any condition
// a person writes will read. The boundary bound exists so nothing large is ever transferred; this
// one exists so nothing large is ever *evaluated* (automation.md §1.2's third limit is about the
// expression, not about what it reads).
const MaxInboundPayloadBytes = 64 * 1024

// InboundToken is a presented inbound credential: `hbt_hook_<tenant>_<secret>`.
//
// The shape is the calendar feed's on purpose, and so is the reason the tenant travels inside it:
// `automation_rule` is behind row level security like every other table, so the lookup by hash
// returns nothing until a tenant context is set, and the only honest source of that context on a
// route with no authentication is the credential itself (multi-tenancy.md §2.2, §3). A token
// naming a tenant it does not belong to gains nothing - the hash covers the whole string, tenant
// half included, and is unique across the installation.
//
// **It authenticates the rule, never a person.** There is no account behind it and it grants
// nothing beyond starting that one rule's run; what the run may then do is the rule's `run_as`
// account's business, checked per action exactly as it is for every other trigger (rule 2).
type InboundToken struct {
	tenantID shared.ID
	raw      string
}

// ParseInboundToken reads a presented token. It checks the shape and nothing else: whether a rule
// has this address, whether it is enabled and whether its trigger is still an inbound webhook are
// decided further in.
//
// Every failure is the same error, for ParseFeedToken's reason: a parser that said which check
// failed would be an oracle for the format.
func ParseInboundToken(raw string) (InboundToken, error) {
	body, found := strings.CutPrefix(raw, InboundTokenPrefix)
	if !found {
		return InboundToken{}, errInboundTokenMalformed()
	}

	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != feedSecretLength {
		return InboundToken{}, errInboundTokenMalformed()
	}
	if !isBase64URL(secret) {
		return InboundToken{}, errInboundTokenMalformed()
	}

	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return InboundToken{}, errInboundTokenMalformed()
	}
	return InboundToken{tenantID: tenantID, raw: raw}, nil
}

// NewInboundToken mints one from freshly drawn randomness. The bytes come from the caller because
// the domain draws nothing itself (rule 4), and because a test that cannot fix the secret cannot
// assert on the result.
func NewInboundToken(tenantID shared.ID, secret []byte) (InboundToken, error) {
	if tenantID.IsZero() {
		return InboundToken{}, shared.ErrValidation.WithDetail("automation.inbound_token_tenant_missing")
	}
	if len(secret) != InboundTokenSecretBytes {
		return InboundToken{}, shared.ErrValidation.WithDetail("automation.inbound_token_secret_short")
	}

	raw := InboundTokenPrefix + strings.ReplaceAll(tenantID.String(), "-", "") + "_" +
		base64.RawURLEncoding.EncodeToString(secret)
	return InboundToken{tenantID: tenantID, raw: raw}, nil
}

// TenantID is the tenant the token is bound to, and therefore the tenant its lookup runs in.
func (t InboundToken) TenantID() shared.ID { return t.tenantID }

// Secret is the whole credential, which is what gets hashed - the tenant half included, so a token
// cannot be rewritten to name another tenant and still match.
//
// Reading it is a deliberate act with a name, and every other way of printing this type is masked
// below. It matters here for the reason it matters for the feed token: this credential travels in
// a URL, where an access log, a proxy and a browser history all keep a copy of whatever is printed
// (rule 10, security.md §4 T-21).
func (t InboundToken) Secret() string { return t.raw }

// String, GoString and MarshalText mask. Leaving them off would not be enough: %v over a struct
// prints its unexported fields, so a token handed to a log line by mistake would print itself.
func (t InboundToken) String() string   { return t.masked() }
func (t InboundToken) GoString() string { return t.masked() }

// MarshalText covers the encoders as well - a token that reached a JSON body by mistake writes the
// mask rather than the credential.
func (t InboundToken) MarshalText() ([]byte, error) { return []byte(t.masked()), nil }

func (t InboundToken) masked() string {
	if t.raw == "" {
		return ""
	}
	return InboundTokenPrefix + "<redacted>"
}

// IsZero reports the empty token.
func (t InboundToken) IsZero() bool { return t.raw == "" }

func errInboundTokenMalformed() error {
	return shared.ErrUnauthenticated.WithDetail("automation.inbound_token_malformed")
}
