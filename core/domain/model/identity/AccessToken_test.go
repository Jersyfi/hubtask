// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
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
