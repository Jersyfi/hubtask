// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every fixed advisory lock key has one holder. Session and transaction advisory locks share one
// key space, so two purposes on one number wait for each other: a leader that holds its lock for
// as long as it leads stops every writer of the other purpose, and a migrator waits for it until
// its run bound ends the upgrade.
func TestEveryFixedAdvisoryLockKeyHasOneHolder(t *testing.T) {
	// Released shared: the installation events' numbering moved to 8_010_004 and still takes the
	// migrator's 8_010_002 for the release before's pods, which number under it alone. The
	// release after drops it (versioning-release.md §4), and this exception with it.
	shared := map[string]int{"8010002": 2}

	goKey := regexp.MustCompile(`(?m)^const \w+ (?:\w+ )?= (8_0\d\d_\d{3})$`)
	sqlKey := regexp.MustCompile(`pg_(?:try_)?advisory_(?:xact_)?lock\((\d+)\)`)
	holders := map[string][]string{}
	for _, root := range []string{"../../cmd", "../../infrastructure", "../../db/queries", "../../db/migrations"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/sqlc/") {
				return err
			}
			content := string(readFile(t, path))
			switch filepath.Ext(path) {
			case ".go":
				for _, m := range goKey.FindAllStringSubmatch(content, -1) {
					key := strings.ReplaceAll(m[1], "_", "")
					holders[key] = append(holders[key], path)
				}
			case ".sql":
				for _, m := range sqlKey.FindAllStringSubmatch(content, -1) {
					holders[m[1]] = append(holders[m[1]], path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if len(holders["8010001"]) == 0 {
		t.Fatal("the scheduler's lock 8_010_001 was not found - the patterns have stopped matching")
	}
	for key, sites := range holders {
		allowed := 1
		if n, ok := shared[key]; ok {
			allowed = n
		}
		if len(sites) > allowed {
			sort.Strings(sites)
			t.Errorf("advisory lock %s is taken for %d purposes (%s): give each its own number",
				key, len(sites), strings.Join(sites, ", "))
		}
	}
}
