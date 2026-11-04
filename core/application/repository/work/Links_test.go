// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "testing"

// A link's or a reference's end draws the key as every other screen does (user-experience.md
// §5.6), never the handle in lower case.
func TestAnEntrysKeyReadsAsEveryScreenDrawsIt(t *testing.T) {
	for _, tc := range []struct {
		summary EntrySummary
		want    string
	}{
		{EntrySummary{KeyPrefix: "INC", Handle: "incidents", Number: 128}, "INC-128"},
		{EntrySummary{Handle: "sup", Number: 128}, "SUP-128"},
		{EntrySummary{Handle: "sup"}, ""},
		{EntrySummary{Number: 3}, ""},
	} {
		if got := tc.summary.Key(); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.summary, got, tc.want)
		}
	}
}
