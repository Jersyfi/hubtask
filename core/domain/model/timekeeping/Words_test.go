// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"strings"
	"testing"
	"time"
)

// The database stores no zero byte and holds a rate's role to a key: words it would refuse are
// refused here, never left to fail the write as unavailable - an error a client retries for ever.
func TestWordsTheDatabaseRefusesAreRefusedFirst(t *testing.T) {
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	handedIn, _ := OpenSheet(person, day(t, "2026-11-02")).Submit(60, now)
	_, rejected := handedIn.Reject(lead, "too\x00few", now)
	from := day(t, "2026-01-01")
	for _, tc := range []struct {
		name, want string
		err        error
	}{
		{"a kind of work's name", "work_types.name_invalid", CheckWorkType(WorkType{Key: "dev", Name: "Dev\x00"})},
		{"a rejection's reason", "timesheets.reason_malformed", rejected},
		{"a role past its key's length", "time.rate_role_invalid", CheckCostRate(CostRate{Role: "d" + strings.Repeat("e", 64), Currency: "EUR", From: from})},
		{"a role holding a zero byte", "time.rate_role_invalid", CheckCostRate(CostRate{Role: "dev\x00", Currency: "EUR", From: from})},
		{"a role no key spells", "time.rate_role_invalid", CheckCostRate(CostRate{Role: "Senior Dev", Currency: "EUR", From: from})},
		{"a built-in role", "", CheckCostRate(CostRate{Role: "MEMBER", Currency: "EUR", From: from})},
		{"a custom role", "", CheckCostRate(CostRate{Role: "developer", Currency: "EUR", From: from})},
	} {
		if got := code(tc.err); got != tc.want {
			t.Errorf("%s: %v, want %q", tc.name, tc.err, tc.want)
		}
	}
}
