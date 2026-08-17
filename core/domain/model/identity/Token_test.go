// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const tenant = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")

func mint(t *testing.T, tenantID shared.ID) Token {
	t.Helper()
	secret := make([]byte, TokenSecretBytes)
	for i := range secret {
		secret[i] = byte(i)
	}
	token, err := NewToken(tenantID, secret)
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}
	return token
}

func TestAMintedTokenParsesBackToItsTenant(t *testing.T) {
	token := mint(t, tenant)

	if !strings.HasPrefix(token.Secret(), TokenPrefix) {
		t.Errorf("token %q does not carry the scanning prefix", token.Secret())
	}

	parsed, err := ParseToken(token.Secret())
	if err != nil {
		t.Fatalf("parsing what was just minted failed: %v", err)
	}
	if parsed.TenantID() != tenant {
		t.Errorf("tenant = %q, want %q", parsed.TenantID(), tenant)
	}
	if parsed.Secret() != token.Secret() {
		t.Error("the parsed token is not the presented one")
	}
}

// The hash covers the whole string, so rewriting the tenant half has to produce a token that is
// either malformed or simply unknown - never one that reaches another tenant's row.
func TestTheTenantIsPartOfTheToken(t *testing.T) {
	token := mint(t, tenant)
	other := shared.ID("018f2a1b-0000-7000-8000-0000000000cd")

	rewritten := strings.Replace(token.Secret(),
		strings.ReplaceAll(tenant.String(), "-", ""),
		strings.ReplaceAll(other.String(), "-", ""), 1)

	parsed, err := ParseToken(rewritten)
	if err != nil {
		t.Fatalf("the rewritten token did not even parse: %v", err)
	}
	if parsed.TenantID() != other {
		t.Fatalf("tenant = %q", parsed.TenantID())
	}
	if parsed.Secret() == token.Secret() {
		t.Error("the rewrite left the hashed material unchanged")
	}
}

func TestParseTokenRefusesAnythingMisshapen(t *testing.T) {
	valid := mint(t, tenant).Secret()

	cases := map[string]string{
		"empty":                 "",
		"no prefix":             strings.TrimPrefix(valid, TokenPrefix),
		"a different prefix":    "ghp_" + strings.TrimPrefix(valid, TokenPrefix),
		"no separator":          strings.Replace(valid, "_", "", 2),
		"a short tenant":        TokenPrefix + "018f_" + strings.Repeat("a", tokenSecretLength),
		"a non-hex tenant":      TokenPrefix + strings.Repeat("z", tenantHexLength) + "_" + strings.Repeat("a", tokenSecretLength),
		"an upper-case tenant":  TokenPrefix + strings.Repeat("A", tenantHexLength) + "_" + strings.Repeat("a", tokenSecretLength),
		"a short secret":        valid[:len(valid)-1],
		"a long secret":         valid + "a",
		"a secret with padding": valid[:len(valid)-1] + "=",
		"whitespace":            " " + valid,
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			token, err := ParseToken(raw)
			if err == nil {
				t.Fatalf("accepted, tenant %q", token.TenantID())
			}
			if !errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("error = %v, want unauthenticated", err)
			}
		})
	}
}

// Every shape failure answers the same thing: the format is the one part of a credential an
// attacker does not have to guess at, so it must not be discoverable one message at a time.
func TestEveryShapeFailureLooksTheSame(t *testing.T) {
	var seen []string
	for _, raw := range []string{"", "hbt_pat_", "nonsense", TokenPrefix + "zz_zz"} {
		_, err := ParseToken(raw)
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) {
			t.Fatalf("%q: error = %v", raw, err)
		}
		seen = append(seen, domainErr.DetailCode)
	}
	for _, detail := range seen {
		if detail != seen[0] {
			t.Errorf("the detail codes differ: %v", seen)
			break
		}
	}
}

func TestNewTokenRefusesWhatCannotBeACredential(t *testing.T) {
	t.Run("without a tenant", func(t *testing.T) {
		if _, err := NewToken("", make([]byte, TokenSecretBytes)); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("with too little entropy", func(t *testing.T) {
		if _, err := NewToken(tenant, make([]byte, 8)); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("error = %v", err)
		}
	})
}

func TestTheZeroTokenSaysSo(t *testing.T) {
	if !(Token{}).IsZero() {
		t.Error("the zero token does not report itself")
	}
	if mint(t, tenant).IsZero() {
		t.Error("a minted token reports itself as zero")
	}
}
