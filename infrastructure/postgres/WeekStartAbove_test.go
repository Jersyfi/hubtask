// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The week start's link above the account, as the credential's read assembles it: the workspace's
// own day first, then the installation's row (an ISO number), and nothing for anything else.
func TestTheWeekStartAboveTheAccount(t *testing.T) {
	for _, tc := range []struct {
		name         string
		workspace    string
		installation []byte
		want         identity.WeekStart
	}{
		{"the workspace's own", "SATURDAY", []byte("7"), identity.WeekStartSaturday},
		{"the installation's", "", []byte("7"), identity.WeekStartSunday},
		{"a workspace day none of the three", "FRIDAY", []byte("1"), identity.WeekStartMonday},
		{"an installation day none of the three", "", []byte("3"), ""},
		{"a row that is not a number", "", []byte(`"SUNDAY"`), ""},
		{"nothing anywhere", "", nil, ""},
	} {
		if got := weekStartAbove(tc.workspace, tc.installation); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
