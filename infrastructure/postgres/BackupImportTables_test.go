// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres_test

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The archive's entity list and this adapter's statement table have to name the same tables. An
// entity added to the archive without a statement here is a restore that stops in the middle with
// an internal error; a statement here for an entity nothing exports is dead code that looks like
// coverage.
func TestEveryRestoredEntityHasAnImportStatement(t *testing.T) {
	statements := postgres.ImportableTables()

	for _, entity := range archive.RestoredEntities() {
		if !slices.Contains(statements, entity.Table) {
			t.Errorf("%s is restored and has no import statement", entity)
		}
	}
	for _, table := range statements {
		if _, known := archive.FindEntityByTable(table); !known {
			t.Errorf("there is an import statement for %s, which no archive carries", table)
		}
	}
}

// The one entity an archive carries and a restore does not write back. It is a list rather than a
// silence so that adding a second one is a decision somebody wrote down.
func TestTheAuditTrailIsReadAndNotWrittenBack(t *testing.T) {
	kept := archive.NotRestored()
	if _, present := kept["audit_log"]; !present {
		t.Fatal("the audit trail is no longer on the list of what a restore does not write back")
	}
	if len(kept) != 1 {
		t.Errorf("%d entities are read and not written back; each one is a decision", len(kept))
	}
	if slices.Contains(postgres.ImportableTables(), "audit_log") {
		t.Error("there is an import statement for the audit trail")
	}
}
