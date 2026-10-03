// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import "testing"

// The manifest lists every generated row, with its day: what a client reads before a field goes.
func TestTheManifestListsEveryDeprecatedField(t *testing.T) {
	listed := *deprecationManifest()
	if len(listed) != len(deprecatedFields) || len(listed) == 0 {
		t.Fatalf("the manifest lists %d of %d deprecated fields", len(listed), len(deprecatedFields))
	}
	for _, entry := range listed {
		if entry.Since.IsZero() || entry.RemovedIn == "" || entry.Reason == "" {
			t.Errorf("%s %s %s is listed without its day, version or reason: %+v", entry.Method, entry.Path, entry.Field, entry)
		}
	}
}
