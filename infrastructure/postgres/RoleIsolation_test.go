// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Each way past row level security is named; the application role as multi-tenancy.md §2.1 makes
// it raises nothing.
func TestARoleRowLevelSecurityDoesNotBindIsWarnedOf(t *testing.T) {
	yes, no := true, false
	if got := isolationWarnings(sqlc.DatabaseRoleIsolationRow{Superuser: &no, BypassRls: &no}); len(got) != 0 {
		t.Errorf("the application role raised %v", got)
	}
	got := isolationWarnings(sqlc.DatabaseRoleIsolationRow{Superuser: &yes, BypassRls: &yes, OwnsTables: true, BackfillPolicies: 1})
	want := []string{"superuser", "bypass_rls", "table_owner", "backfill_policy"}
	if len(got) != len(want) {
		t.Fatalf("warnings %v", got)
	}
	for i, w := range got {
		if w.Code != IsolationBypassedCode || w.Severity != "critical" || w.Params["cause"] != want[i] {
			t.Errorf("warning %d = %+v, want cause %s", i, w, want[i])
		}
	}
}
