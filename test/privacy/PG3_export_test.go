// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
)

// PG-3: the access export contains every field classified as personal - the catalogue reconciled
// against the export's schema (data-protection.md §10).
//
// The export is the archive of E-04 filtered to one person (E-10), so "the export's schema" is two
// things: which entities the archive carries, and which column of each makes a row somebody's. This
// gate holds the second against the first. A table the archive carries and the export has made no
// decision about is the failure it exists for - silence means "not exported", and the person's copy
// is quietly short.
func TestPG3EveryArchivedTableIsDecidedForTheExport(t *testing.T) {
	byPerson, byAddress, excluded := privacyservice.SubjectTables()

	for _, entity := range archive.Entities() {
		_, personal := byPerson[entity.Table]
		_, byEmail := byAddress[entity.Table]
		_, left := excluded[entity.Table]

		switch {
		case personal || byEmail:
			if left {
				t.Errorf("%s is both exported and excluded (PG-3) - one of the two decisions is stale",
					entity.Table)
			}
		case left:
			if reason := excluded[entity.Table]; len(reason) < 20 {
				t.Errorf("%s is left out of the export with the reason %q (PG-3) - a reason "+
					"somebody can weigh, or the table belongs in the export", entity.Table, reason)
			}
		default:
			t.Errorf("%s is in the archive and the export has decided nothing about it (PG-3). "+
				"Name the column that makes a row the person's, or say in notThePersons why a copy "+
				"of somebody's data leaves it out", entity.Table)
		}
	}
}

// And the other direction: a decision about a table the archive does not carry is a decision about
// nothing, which is how a map like this rots.
func TestPG3TheExportDecidesAboutNothingItCannotRead(t *testing.T) {
	carried := map[string]bool{}
	for _, entity := range archive.Entities() {
		carried[entity.Table] = true
	}

	byPerson, byAddress, excluded := privacyservice.SubjectTables()
	for _, decisions := range []map[string]bool{
		tablesOf(byPerson), tablesOf(byAddress), reasonsOf(excluded),
	} {
		for table := range decisions {
			if !carried[table] {
				t.Errorf("the export decides about %s, which no archive carries (PG-3)", table)
			}
		}
	}
}

func tablesOf(columns map[string][]string) map[string]bool {
	out := map[string]bool{}
	for table := range columns {
		out[table] = true
	}
	return out
}

func reasonsOf(reasons map[string]string) map[string]bool {
	out := map[string]bool{}
	for table := range reasons {
		out[table] = true
	}
	return out
}
