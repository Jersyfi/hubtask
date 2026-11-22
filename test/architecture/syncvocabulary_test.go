// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
)

// TestTheWebAppDecidesEveryWalkedEntity holds the web app's hand-kept list of change-log entities
// (`pathsFor`, apps/webapp/src/lib/data/live.ts) to the walk's: every entity a device is handed is
// a `case` there, whether it names the reads the record makes stale or says that none is. An entity
// added to the walk without one would fall into the client's `default` and be dropped without a
// word - the stale screen the record was added to end (offline-sync.md §3.1).
func TestTheWebAppDecidesEveryWalkedEntity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "apps", "webapp", "src", "lib", "data", "live.ts"))
	if err != nil {
		t.Fatalf("reading the web app's list: %v", err)
	}
	decided := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^\s*case '([a-z_]+)':`).FindAllSubmatch(raw, -1) {
		decided[string(match[1])] = true
	}
	if len(decided) == 0 {
		t.Fatal("no case found in live.ts: the list moved, and this test reads nothing")
	}
	for _, entity := range syncservice.WalkedEntities() {
		if !decided[entity] {
			t.Errorf("the walk hands out %q and pathsFor has no case for it: name what its record makes stale, or that nothing is", entity)
		}
	}
}
