// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"strings"
	"testing"
)

// A version's name is one line the database can store: a zero byte is refused as malformed,
// never answered as an outage.
func TestAVersionNameIsOneStorableLine(t *testing.T) {
	for _, raw := range []string{strings.Repeat("x", MaxVersionName+1), "a\nb", "a\x00b"} {
		if _, err := CheckVersionName(raw, nil); detail(err) != "pages.version_name_malformed" {
			t.Errorf("%q: %v", raw, err)
		}
	}
	if name, err := CheckVersionName("  Before the review ", nil); err != nil || name != "Before the review" {
		t.Errorf("name %q, err %v", name, err)
	}
}
