// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-19 check 3: an owner changes the address only where the installation allows it, and not
// again inside its cooldown - 30 days unless the installation decides otherwise.
func TestOwnerAddressRuleRefuseOwnerChange(t *testing.T) {
	changed := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name       string
		rule       identity.OwnerAddressRule
		last, now  time.Time
		wantCode   string
		wantFrom   string
		wantStatus error
	}{
		{name: "off by default", rule: identity.OwnerAddressRule{},
			now: changed, wantCode: "access.address_change_not_allowed", wantStatus: shared.ErrForbidden},
		{name: "off, though the cooldown passed", rule: identity.OwnerAddressRule{CooldownDays: 1},
			last: changed, now: changed.AddDate(1, 0, 0),
			wantCode: "access.address_change_not_allowed", wantStatus: shared.ErrForbidden},
		{name: "on, never changed", rule: identity.OwnerAddressRule{Allowed: true}, now: changed},
		{name: "on, the day after a change", rule: identity.OwnerAddressRule{Allowed: true},
			last: changed, now: changed.AddDate(0, 0, 1),
			wantCode: "access.address_change_cooldown", wantFrom: "2026-11-13T09:30:00Z",
			wantStatus: shared.ErrConflict},
		{name: "on, a second before the default 30 days", rule: identity.OwnerAddressRule{Allowed: true},
			last: changed, now: changed.AddDate(0, 0, 30).Add(-time.Second),
			wantCode: "access.address_change_cooldown", wantFrom: "2026-11-13T09:30:00Z",
			wantStatus: shared.ErrConflict},
		{name: "on, at 30 days", rule: identity.OwnerAddressRule{Allowed: true},
			last: changed, now: changed.AddDate(0, 0, 30)},
		{name: "on, the installation's own cooldown", rule: identity.OwnerAddressRule{Allowed: true, CooldownDays: 7},
			last: changed, now: changed.AddDate(0, 0, 6),
			wantCode: "access.address_change_cooldown", wantFrom: "2026-10-21T09:30:00Z",
			wantStatus: shared.ErrConflict},
		{name: "on, past the installation's own cooldown", rule: identity.OwnerAddressRule{Allowed: true, CooldownDays: 7},
			last: changed, now: changed.AddDate(0, 0, 7)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.RefuseOwnerChange(tc.last, tc.now)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var problem *shared.Error
			if !errors.As(err, &problem) || problem.DetailCode != tc.wantCode || !errors.Is(err, tc.wantStatus) {
				t.Fatalf("err = %v, want %s", err, tc.wantCode)
			}
			if got := problem.Params["possible_at"]; got != tc.wantFrom {
				t.Errorf("possible_at = %q, want %q", got, tc.wantFrom)
			}
		})
	}
}

func TestOwnerAddressRuleChangeableFrom(t *testing.T) {
	changed := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	rule := identity.OwnerAddressRule{Allowed: true}
	if got := rule.ChangeableFrom(time.Time{}, changed); !got.IsZero() {
		t.Errorf("never changed answers %v", got)
	}
	if got := rule.ChangeableFrom(changed, changed.AddDate(0, 0, 3)); !got.Equal(changed.AddDate(0, 0, 30)) {
		t.Errorf("inside the cooldown answers %v", got)
	}
	if got := rule.ChangeableFrom(changed, changed.AddDate(0, 0, 31)); !got.IsZero() {
		t.Errorf("past the cooldown answers %v", got)
	}
}

func TestValidAddressCooldownDays(t *testing.T) {
	for _, days := range []int{0, 1, 30, 365} {
		if got, err := identity.ValidAddressCooldownDays(days); err != nil || got != days {
			t.Errorf("%d: %d, %v", days, got, err)
		}
	}
	for _, days := range []int{-1, 366} {
		var problem *shared.Error
		if _, err := identity.ValidAddressCooldownDays(days); !errors.As(err, &problem) ||
			problem.DetailCode != "admin.address_cooldown_invalid" {
			t.Errorf("%d: %v", days, err)
		}
	}
}
