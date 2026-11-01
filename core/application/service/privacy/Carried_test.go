// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
)

// carried names, per use case check that puts its data into the workspace's archive - and, where
// it says so, into a person's copy of their data - the tables that hold it.
var carried = []struct {
	check  string
	tables []string
	copied []string
}{
	// UC-AGT-03 check 5: the models join the workspace's archive.
	{"UC-AGT-03/5", []string{"agent_model"}, nil},
	// UC-PLN-04 check 4: a baseline with the entries' dates it holds.
	{"UC-PLN-04/4", []string{"plan_baseline", "plan_baseline_entry"}, nil},
	// UC-SVC-07 check 5: a request's rating.
	{"UC-SVC-07/5", []string{"satisfaction_rating"}, nil},
	// UC-SVC-08 check 4: approvals, their decisions and delegations, in the archive and the copy.
	{"UC-SVC-08/4", []string{"approval", "approval_decision", "approval_delegation"}, []string{"approval", "approval_decision", "approval_delegation"}},
}

func TestTheArchiveAndTheCopyCarryWhatTheUseCasesPutIntoThem(t *testing.T) {
	restored := map[string]bool{}
	for _, e := range archive.RestoredEntities() {
		restored[e.Table] = true
	}
	byPerson, _, _ := SubjectTables()
	for _, c := range carried {
		t.Run(c.check, func(t *testing.T) {
			for _, table := range c.tables {
				if _, ok := archive.FindEntityByTable(table); !ok {
					t.Errorf("%s: the archive leaves %s out", c.check, table)
				}
				if !restored[table] {
					t.Errorf("%s: a restore does not write %s back", c.check, table)
				}
			}
			for _, table := range c.copied {
				// The copy is the archive's rows filtered to those naming the person by a column
				// of SubjectTables; a table with none of them leaves nothing in it.
				if len(byPerson[table]) == 0 {
					t.Errorf("%s: a person's copy holds no rows of %s", c.check, table)
				}
			}
		})
	}
}
