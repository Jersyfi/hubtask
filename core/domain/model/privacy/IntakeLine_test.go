// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import "testing"

func TestStripIntakeLine(t *testing.T) {
	for _, tc := range []struct {
		name, text, address, want string
		stripped                  bool
	}{
		{"name and address", "From: Erika Muster <erika@example.org>\n\nThe printer is broken", "erika@example.org", "The printer is broken", true},
		{"address only", "From: erika@example.org\n\nHello", "erika@example.org", "Hello", true},
		{"case of the address", "From: Erika <Erika@Example.org>\n\nHello", "erika@example.org", "Hello", true},
		{"the line alone", "From: erika@example.org", "erika@example.org", "", true},
		{"somebody else", "From: Max <max@example.org>\n\nHello", "erika@example.org", "From: Max <max@example.org>\n\nHello", false},
		{"an address inside another", "From: anna-erika@example.org\n\nHi", "erika@example.org", "From: anna-erika@example.org\n\nHi", false},
		{"not the first line", "Hello\nFrom: erika@example.org", "erika@example.org", "Hello\nFrom: erika@example.org", false},
		{"no address given", "From: erika@example.org", "", "From: erika@example.org", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, stripped := StripIntakeLine(tc.text, tc.address)
			if got != tc.want || stripped != tc.stripped {
				t.Errorf("got %q, %v; want %q, %v", got, stripped, tc.want, tc.stripped)
			}
		})
	}
}
