// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Configuration items and connectors are switched off, never deleted (accepted.md B,
// docs/privacy/data-catalog.md): a CI is retired and a connector disabled, and both go only with
// the workspace - its purge, or a replacing restore that empties every table first. So no statement
// outside the restore's own file deletes from either table.
func TestNoStatementDeletesAConfigurationItemOrAConnector(t *testing.T) {
	files, err := filepath.Glob("../../db/queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("reading the queries: %v", err)
	}
	deletes := regexp.MustCompile(`(?i)DELETE\s+FROM\s+(cmdb_ci|cmdb_connector)\b`)
	for _, file := range files {
		if filepath.Base(file) == "BackupImport.sql" {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if found := deletes.FindString(string(body)); found != "" {
			t.Errorf("%s: %q - a CI is retired and a connector switched off, never deleted", filepath.Base(file), found)
		}
	}
}
