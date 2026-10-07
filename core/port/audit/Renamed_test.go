// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"slices"
	"testing"
)

// A renamed action is one action under two names: new entries carry the new one, stored entries
// keep the old one (the hash covers the stored shape), and a search by either finds both.
func TestARenamedActionIsFoundUnderEitherName(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		want   []string
	}{
		{"auth.", []string{"mfa.recovery_regenerated"}},
		{"auth.mfa", []string{"mfa.recovery_regenerated"}},
		{"auth.mfa_r", []string{"mfa.recovery_regenerated"}},
		{"auth.mfa_recovery_regenerated", []string{"mfa.recovery_regenerated"}},
		{"auth.mfa_recovery_regenerated.and_more", nil},
		{"mfa.", []string{"auth.mfa_recovery_regenerated"}},
		{"mfa.recovery_regenerated", []string{"auth.mfa_recovery_regenerated"}},
		{"auth.session", nil},
		{"", nil},
	} {
		if got := Aliases(tc.prefix); !slices.Equal(got, tc.want) {
			t.Errorf("Aliases(%q) = %v, want %v", tc.prefix, got, tc.want)
		}
	}
}
