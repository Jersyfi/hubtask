// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var now = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

func TestAccessTokenVerify(t *testing.T) {
	cases := []struct {
		name       string
		token      AccessToken
		wantDetail string
	}{
		{
			name:  "valid until tomorrow",
			token: AccessToken{ExpiresAt: now.Add(24 * time.Hour)},
		},
		{
			name:       "expired an hour ago",
			token:      AccessToken{ExpiresAt: now.Add(-time.Hour)},
			wantDetail: "access.token_expired",
		},
		{
			// The boundary belongs to the past: a token that expires "now" is expired.
			name:       "expiring exactly now",
			token:      AccessToken{ExpiresAt: now},
			wantDetail: "access.token_expired",
		},
		{
			name:       "without an expiry at all",
			token:      AccessToken{},
			wantDetail: "access.token_expired",
		},
		{
			name:       "revoked",
			token:      AccessToken{ExpiresAt: now.Add(24 * time.Hour), RevokedAt: now.Add(-time.Minute)},
			wantDetail: "access.token_revoked",
		},
		{
			// Revocation is reported before expiry: one is a security event, the other routine.
			name:       "revoked and expired",
			token:      AccessToken{ExpiresAt: now.Add(-time.Hour), RevokedAt: now.Add(-time.Hour)},
			wantDetail: "access.token_revoked",
		},
		{
			name:  "revoked in the future is not yet revoked",
			token: AccessToken{ExpiresAt: now.Add(24 * time.Hour), RevokedAt: now.Add(time.Minute)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.token.Verify(now)
			if c.wantDetail == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) {
				t.Fatalf("error = %v", err)
			}
			if !errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("error = %v, want unauthenticated", err)
			}
			if domainErr.DetailCode != c.wantDetail {
				t.Errorf("detail = %q, want %q", domainErr.DetailCode, c.wantDetail)
			}
		})
	}
}

func TestNeedsTouch(t *testing.T) {
	interval := 5 * time.Minute

	cases := map[string]struct {
		lastUsed time.Time
		want     bool
	}{
		"never used":              {time.Time{}, true},
		"used just now":           {now.Add(-time.Second), false},
		"used a minute ago":       {now.Add(-time.Minute), false},
		"used an hour ago":        {now.Add(-time.Hour), true},
		"exactly at the interval": {now.Add(-interval), true},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			token := AccessToken{LastUsedAt: c.lastUsed}
			if got := token.NeedsTouch(now, interval); got != c.want {
				t.Errorf("NeedsTouch = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAccountVerify(t *testing.T) {
	if err := (Account{Status: AccountActive}).Verify(); err != nil {
		t.Errorf("an active account was refused: %v", err)
	}

	for _, status := range []AccountStatus{AccountInvited, AccountDisabled, ""} {
		err := Account{Status: status}.Verify()
		if !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("status %q: error = %v, want forbidden", status, err)
		}
		var domainErr *shared.Error
		if errors.As(err, &domainErr) && domainErr.Params["status"] != string(status) {
			t.Errorf("status %q: params = %v", status, domainErr.Params)
		}
	}
}

// The three identifiers a mint needs, fixed so that a failure names a value rather than a
// generated one.
var (
	mintTenant  = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	mintAccount = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
	mintToken   = shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
)

func validMint() NewAccessTokenInput {
	return NewAccessTokenInput{
		ID: mintToken, TenantID: mintTenant, AccountID: mintAccount,
		Name:      "  the nightly export  ",
		Scopes:    []string{"items:write", "items:read", "items:read", " "},
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		Now:       now,
	}
}

func TestNewAccessTokenNormalisesWhatItStores(t *testing.T) {
	token, err := NewAccessToken(validMint())
	if err != nil {
		t.Fatalf("a valid mint was refused: %v", err)
	}

	if token.Name != "the nightly export" {
		t.Errorf("the name was not trimmed: %q", token.Name)
	}
	// Sorted and deduplicated, so that two tokens asking for the same rights are stored
	// identically and a listing reads the same way twice.
	if got := strings.Join(token.Scopes, ","); got != "items:read,items:write" {
		t.Errorf("scopes = %q", got)
	}
	if token.CreatedAt != now {
		t.Errorf("created at %v, want %v", token.CreatedAt, now)
	}
	if token.IsRevoked() {
		t.Error("a freshly minted token reports itself revoked")
	}
	// The moment it stops working is the moment the caller asked for, and nothing about the
	// plaintext is on the row at all.
	if err := token.Verify(now); err != nil {
		t.Errorf("a fresh token does not verify: %v", err)
	}
}

func TestNewAccessTokenRefusesWhatSecurityForbids(t *testing.T) {
	cases := map[string]struct {
		change     func(*NewAccessTokenInput)
		wantDetail string
		wantPath   string
	}{
		"no name": {
			func(in *NewAccessTokenInput) { in.Name = "   " },
			"access.token_name_required", "/name",
		},
		"a name past the column": {
			func(in *NewAccessTokenInput) { in.Name = strings.Repeat("n", MaxTokenNameLength+1) },
			"access.token_name_too_long", "/name",
		},
		"no scopes at all": {
			func(in *NewAccessTokenInput) { in.Scopes = nil },
			"access.token_scopes_required", "/scopes",
		},
		"scopes that are all blank": {
			func(in *NewAccessTokenInput) { in.Scopes = []string{"", "  "} },
			"access.token_scopes_required", "/scopes",
		},
		"no expiry - there is no default": {
			func(in *NewAccessTokenInput) { in.ExpiresAt = time.Time{} },
			"access.token_expiry_required", "/expires_at",
		},
		"an expiry already past": {
			func(in *NewAccessTokenInput) { in.ExpiresAt = now.Add(-time.Second) },
			"access.token_expiry_past", "/expires_at",
		},
		"a day past the year": {
			func(in *NewAccessTokenInput) { in.ExpiresAt = now.Add(MaxTokenLifetime + time.Hour) },
			"access.token_expiry_too_far", "/expires_at",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := validMint()
			c.change(&in)

			_, err := NewAccessToken(in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error = %v, want a validation error", err)
			}

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) {
				t.Fatalf("error = %v, want a domain error", err)
			}
			if domainErr.DetailCode != c.wantDetail {
				t.Errorf("detail = %q, want %q", domainErr.DetailCode, c.wantDetail)
			}
			// A refusal a client can act on names the field, because "invalid request" is not
			// something a caller can fix (api-guidelines.md §6).
			if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != c.wantPath {
				t.Errorf("fields = %v, want one at %s", domainErr.Fields, c.wantPath)
			}
		})
	}
}

func TestNewAccessTokenAllowsExactlyAYear(t *testing.T) {
	in := validMint()
	in.ExpiresAt = now.Add(MaxTokenLifetime)

	if _, err := NewAccessToken(in); err != nil {
		t.Errorf("a year to the second was refused: %v", err)
	}
}

func TestRevokedKeepsTheFirstWithdrawal(t *testing.T) {
	token, err := NewAccessToken(validMint())
	if err != nil {
		t.Fatalf("a valid mint was refused: %v", err)
	}

	first := token.Revoked(now)
	second := first.Revoked(now.Add(time.Hour))

	// The moment somebody pulled it is the one an auditor asks about, and the second call is
	// somebody making sure rather than a new event.
	if second.RevokedAt != now {
		t.Errorf("revoked at %v, want the first withdrawal at %v", second.RevokedAt, now)
	}
	if err := second.Verify(now.Add(time.Minute)); err == nil {
		t.Error("a revoked token still verifies")
	}
}
