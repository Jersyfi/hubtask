// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "testing"

func TestAZoneIsAnIANANameAndNeverTheServersOwn(t *testing.T) {
	for name, known := range map[string]bool{
		"Europe/Berlin": true, "UTC": true, "Pacific/Auckland": true,
		"Local": false, "local": false, "": false, "Mars/Olympus": false,
	} {
		if _, ok := LoadZone(name); ok != known {
			t.Errorf("LoadZone(%q) = %v, want %v", name, ok, known)
		}
	}
}
