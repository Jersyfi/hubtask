// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
)

var noon = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

// The period a tenant starts with, as data-retention.md §3 documents it. Written down here rather
// than only in the document, because the sweep reads it and a drift between the two would be a trash
// emptied on a schedule nobody wrote down.
func TestTheDefaultTrashPeriodIsThirtyDaysWithASevenDayFloor(t *testing.T) {
	trash := defaultFor(t, lifecycle.KindTrash)

	if trash.RetainDays != 30 || trash.MinDays != 7 {
		t.Errorf("the default is %d days with a floor of %d, want 30 and 7",
			trash.RetainDays, trash.MinDays)
	}
	if trash.MaxDays != nil {
		t.Errorf("the default sets an upper bound of %d, want none", *trash.MaxDays)
	}
}

// The notification history, as data-retention.md §3 and data-protection.md §5 both give it. No
// lower bound: §4.3 names one for the trash and none for this, and a floor invented here would be
// a retention rule decided in code rather than in the document (ADR-0020).
func TestTheDefaultNotificationPeriodIsNinetyDaysWithNoFloor(t *testing.T) {
	history := defaultFor(t, lifecycle.KindNotification)

	if history.RetainDays != 90 {
		t.Errorf("the default is %d days, want 90", history.RetainDays)
	}
	if history.MinDays != 0 {
		t.Errorf("the default has a floor of %d days, and no document sets one", history.MinDays)
	}
	if lifecycle.FloorFor(lifecycle.KindNotification) != 0 {
		t.Error("a floor reached the kind through FloorFor after all")
	}
}

// Every kind this installation sweeps has exactly one default, and every default is for a kind
// something sweeps. A period configured for a kind nothing removes is a promise nothing keeps.
func TestTheDefaultsAreOnePerKindTheEngineSweeps(t *testing.T) {
	seen := map[lifecycle.DataKind]bool{}
	for _, policy := range lifecycle.DefaultPolicies() {
		if seen[policy.DataKind] {
			t.Errorf("%s has two defaults, and FloorFor would read whichever comes first",
				policy.DataKind)
		}
		seen[policy.DataKind] = true
	}
	for _, swept := range []lifecycle.DataKind{
		lifecycle.KindTrash, lifecycle.KindNotification,
	} {
		if !seen[swept] {
			t.Errorf("%s is swept and has no default period", swept)
		}
	}
	if len(seen) != 2 {
		t.Errorf("%d kinds have defaults, want the two the engine sweeps", len(seen))
	}
}

func defaultFor(t *testing.T, kind lifecycle.DataKind) lifecycle.Policy {
	t.Helper()
	for _, policy := range lifecycle.DefaultPolicies() {
		if policy.DataKind == kind {
			return policy
		}
	}
	t.Fatalf("no default period for %s", kind)
	return lifecycle.Policy{}
}

// The cutoff is the instant a row has to be older than, counted back in days rather than in hours:
// a period is a number of days in every calendar this system reports in, and subtracting a duration
// would drift by an hour twice a year.
func TestTheCutoffCountsBackTheConfiguredPeriod(t *testing.T) {
	policy := lifecycle.Policy{DataKind: lifecycle.KindTrash, RetainDays: 30, MinDays: 7}

	if got, want := policy.Cutoff(noon), noon.AddDate(0, 0, -30); !got.Equal(want) {
		t.Errorf("the cutoff is %v, want %v", got, want)
	}
}

// The lower bound of data-retention.md §4.3, applied where the period is used and taken from the
// kind rather than from the row. A rule that read the row's own copy of the bound would be undercut
// by whoever wrote that copy, which is the one thing a lower bound exists to prevent. A period below
// it becomes the bound rather than a refusal - the tenant asked for their trash to be emptied
// sooner, and the soonest allowed is closer to that than not sweeping at all.
func TestAPeriodBelowTheFloorIsRaisedToIt(t *testing.T) {
	for _, c := range []struct {
		name       string
		retainDays int
		wantDays   int
	}{
		{"a period nobody set", 0, 7},
		{"below the floor", 3, 7},
		{"exactly the floor", 7, 7},
		{"above it", 30, 30},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The row's own bound is deliberately wrong here: it is a copy, and the bound the
			// sweep obeys is the kind's.
			policy := lifecycle.Policy{
				DataKind: lifecycle.KindTrash, RetainDays: c.retainDays, MinDays: 1,
			}

			if got, want := policy.Cutoff(noon), noon.AddDate(0, 0, -c.wantDays); !got.Equal(want) {
				t.Errorf("a %d-day period cut off at %v, want %v (a %d-day period)",
					c.retainDays, got, want, c.wantDays)
			}
		})
	}
}

// A kind with no documented default has no bound either, which is the honest answer for one nothing
// sweeps yet rather than a floor invented in the code.
func TestAKindWithNoDefaultHasNoFloor(t *testing.T) {
	if floor := lifecycle.FloorFor("COMMENT"); floor != 0 {
		t.Errorf("an unswept kind has a floor of %d days, want none", floor)
	}
	if floor := lifecycle.FloorFor(lifecycle.KindTrash); floor != 7 {
		t.Errorf("the trash floor is %d days, want 7", floor)
	}
}
