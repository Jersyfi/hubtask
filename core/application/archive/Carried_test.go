// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"slices"
	"testing"
)

// carried names, per use case check that puts its data into the workspace's archive, the tables
// that hold that data. A table here is one the archive writes and a restore applies; the restore
// itself is the same code for every entity (backup-restore.md §7).
var carried = []struct {
	check  string
	tables []string
}{
	// UC-AST-01 check 7: every class, relation type, source, CI, identifier, relation, run, hold
	// and duplicate.
	{"UC-AST-01/7", []string{"cmdb_class", "cmdb_relation_type", "cmdb_source", "cmdb_ci", "cmdb_ci_identifier", "cmdb_relation", "cmdb_run", "cmdb_hold", "cmdb_duplicate"}},
	// UC-DOC-09 check 3: pages, their versions and their comments.
	{"UC-DOC-09/3", []string{"page", "page_version", "page_comment"}},
	// UC-CNV-03 check 4: every board - its objects are its scene - and its history.
	{"UC-CNV-03/4", []string{"whiteboard", "whiteboard_version"}},
	// UC-ID-26 check 9: a role with its permissions, and its grants.
	{"UC-ID-26/9", []string{"custom_role", "custom_role_grant"}},
}

func TestTheArchiveCarriesWhatTheUseCasesPutIntoIt(t *testing.T) {
	restored := map[string]bool{}
	for _, e := range RestoredEntities() {
		restored[e.Table] = true
	}
	for _, c := range carried {
		t.Run(c.check, func(t *testing.T) {
			for _, table := range c.tables {
				if !slices.ContainsFunc(Entities(), func(e Entity) bool { return e.Table == table }) {
					t.Errorf("%s: the archive leaves %s out", c.check, table)
				}
				if !restored[table] {
					t.Errorf("%s: a restore does not write %s back", c.check, table)
				}
			}
		})
	}
}
