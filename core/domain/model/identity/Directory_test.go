// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestExternalID(t *testing.T) {
	cases := []struct {
		name, raw, want string
		code            string
	}{
		{name: "trimmed", raw: "  00u1abc ", want: "00u1abc"},
		{name: "empty clears", raw: "", want: ""},
		{name: "at the bound", raw: strings.Repeat("x", MaxExternalID), want: strings.Repeat("x", MaxExternalID)},
		{name: "too long", raw: strings.Repeat("x", MaxExternalID+1), code: "directory.external_id_too_long"},
		{name: "control character", raw: "a\x00b", code: "directory.external_id_malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExternalID(c.raw)
			if c.code != "" {
				if shared.AsError(err) == nil || shared.AsError(err).DetailCode != c.code {
					t.Fatalf("ExternalID(%q) = %v, want %s", c.raw, err, c.code)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("ExternalID(%q) = %q, %v; want %q", c.raw, got, err, c.want)
			}
		})
	}
}

func TestDirectoryStatus(t *testing.T) {
	cases := []struct {
		name    string
		current AccountStatus
		active  bool
		want    AccountStatus
		changed bool
		err     error
	}{
		{name: "active switched off", current: AccountActive, active: false, want: AccountDisabled, changed: true},
		{name: "invited switched off", current: AccountInvited, active: false, want: AccountDisabled, changed: true},
		{name: "disabled switched on", current: AccountDisabled, active: true, want: AccountActive, changed: true},
		{name: "disabled stays off", current: AccountDisabled, active: false, want: AccountDisabled},
		{name: "active stays on", current: AccountActive, active: true, want: AccountActive},
		{name: "invited stays invited", current: AccountInvited, active: true, want: AccountInvited},
		{name: "restricted stays on", current: AccountRestricted, active: true, want: AccountRestricted},
		{name: "restricted kept", current: AccountRestricted, active: false, err: shared.ErrConflict},
		{name: "anonymised is gone", current: AccountAnonymized, active: true, err: shared.ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed, err := DirectoryStatus(c.current, c.active)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("err = %v, want %v", err, c.err)
				}
				return
			}
			if err != nil || got != c.want || changed != c.changed {
				t.Fatalf("DirectoryStatus(%s, %v) = %s, %v, %v; want %s, %v", c.current, c.active, got, changed, err, c.want, c.changed)
			}
		})
	}
}

func TestDirectoryActive(t *testing.T) {
	for status, want := range map[AccountStatus]bool{
		AccountActive: true, AccountInvited: true, AccountRestricted: true,
		AccountDisabled: false, AccountAnonymized: false,
	} {
		if got := (Account{Status: status}).DirectoryActive(); got != want {
			t.Errorf("%s: DirectoryActive = %v, want %v", status, got, want)
		}
	}
}
