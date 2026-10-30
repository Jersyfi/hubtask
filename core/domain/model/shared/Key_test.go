// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"strings"
	"testing"
)

func TestValidKey(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{"mine", true},
		{"about_to_breach", true},
		{"p1", true},
		{"a" + strings.Repeat("b", 62), true},
		{"", false},
		{"a" + strings.Repeat("b", 63), false},
		{"1st", false},
		{"_mine", false},
		{"Mine", false},
		{"about-to-breach", false},
		{"über", false},
	} {
		if got := ValidKey(tc.key); got != tc.want {
			t.Errorf("ValidKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}
