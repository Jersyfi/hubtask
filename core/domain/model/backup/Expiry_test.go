// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// daily builds one archive per day, oldest first, at three in the morning.
func daily(count int) []domain.Archive {
	var archives []domain.Archive
	start := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	for day := range count {
		at := start.AddDate(0, 0, day)
		archives = append(archives, domain.Archive{
			ID: idOf(day), TakenAt: at, Mode: domain.ModeIncremental,
		})
	}
	return archives
}

func idOf(n int) shared.ID {
	return shared.MustParseID(fmt.Sprintf("0198f0a0-0000-7000-8000-%012d", n))
}

func kept(expiry domain.Expiry) map[shared.ID]bool {
	out := map[shared.ID]bool{}
	for _, archive := range expiry.Keep {
		out[archive.ID] = true
	}
	return out
}

func TestKeepLastKeepsTheNewest(t *testing.T) {
	plan := domain.Retention{KeepLast: 3, MinKeep: 1}

	expiry := plan.Apply(daily(10), time.UTC)

	if len(expiry.Keep) != 3 {
		t.Fatalf("kept %d, want 3", len(expiry.Keep))
	}
	for _, day := range []int{9, 8, 7} {
		if !kept(expiry)[idOf(day)] {
			t.Errorf("day %d was not kept", day)
		}
	}
	if len(expiry.Expire) != 7 {
		t.Fatalf("expired %d, want 7", len(expiry.Expire))
	}
	// Oldest first, which is the order to delete in.
	if expiry.Expire[0].ID != idOf(0) {
		t.Fatalf("the first to go is %s, want the oldest", expiry.Expire[0].ID)
	}
}

// The newest of each period rather than the oldest: a daily backup is kept because it is the state
// at the end of that day, which is what somebody restoring "yesterday" means.
func TestAGenerationKeepsTheNewestOfEachPeriod(t *testing.T) {
	var archives []domain.Archive
	day := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	for hour, at := range []time.Time{
		day.Add(3 * time.Hour), day.Add(9 * time.Hour), day.Add(21 * time.Hour),
	} {
		archives = append(archives, domain.Archive{ID: idOf(hour), TakenAt: at})
	}

	expiry := domain.Retention{KeepDaily: 1, MinKeep: 1}.Apply(archives, time.UTC)

	if len(expiry.Keep) != 1 || expiry.Keep[0].ID != idOf(2) {
		t.Fatalf("kept %+v, want the last run of the day", expiry.Keep)
	}
}

// The whole plan at once, on a year of daily runs: the answer is a handful of archives spread over
// days, weeks, months and years rather than a year of them.
func TestTheGenerationsSpreadWhatIsKept(t *testing.T) {
	plan := domain.Retention{KeepLast: 7, KeepDaily: 14, KeepWeekly: 8, KeepMonthly: 12, KeepYearly: 3, MinKeep: 3}

	expiry := plan.Apply(daily(400), time.UTC)

	if len(expiry.Keep)+len(expiry.Expire) != 400 {
		t.Fatalf("%d kept and %d expired out of 400", len(expiry.Keep), len(expiry.Expire))
	}
	// Every generation is at most its count, and they overlap: fourteen dailies include the seven
	// last, and the newest weekly is one of the dailies.
	if len(expiry.Keep) > 7+14+8+12+3 {
		t.Fatalf("kept %d, more than every generation together", len(expiry.Keep))
	}
	if len(expiry.Keep) < 14 {
		t.Fatalf("kept %d, fewer than the dailies alone", len(expiry.Keep))
	}
	// And what is kept reaches back: the monthlies alone put the oldest kept archive most of a
	// year behind the newest, which is the point of having generations at all.
	oldest, newest := expiry.Keep[len(expiry.Keep)-1], expiry.Keep[0]
	if newest.TakenAt.Sub(oldest.TakenAt) < 300*24*time.Hour {
		t.Fatalf("the oldest kept archive is only %v behind the newest - nothing survived from "+
			"the far end", newest.TakenAt.Sub(oldest.TakenAt))
	}
}

// The floor is a floor: whatever the counts work out to, that many archives stay.
func TestTheFloorIsNeverUndercut(t *testing.T) {
	// A plan that keeps nothing at all, which is exactly what min_keep exists for.
	plan := domain.Retention{MinKeep: 3}

	expiry := plan.Apply(daily(10), time.UTC)

	if len(expiry.Keep) != 3 {
		t.Fatalf("kept %d, want the floor of 3", len(expiry.Keep))
	}
	if !expiry.FloorHeld {
		t.Fatal("the floor held and the answer does not say so")
	}
	for _, day := range []int{9, 8, 7} {
		if !kept(expiry)[idOf(day)] {
			t.Errorf("the floor kept the wrong archives: day %d is gone", day)
		}
	}
}

func TestAFloorLargerThanTheHoldingKeepsEverything(t *testing.T) {
	expiry := domain.Retention{MinKeep: 20}.Apply(daily(4), time.UTC)

	if len(expiry.Expire) != 0 {
		t.Fatalf("%d archives expired with a floor above the holding", len(expiry.Expire))
	}
}

// An incremental restores only through its chain back to a full archive. Deleting a parent does
// not free one archive - it destroys every archive after it, silently.
func TestAnArchiveSomethingNewerNeedsIsKept(t *testing.T) {
	full := domain.Archive{ID: idOf(1), TakenAt: time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC), Mode: domain.ModeFull}
	middle := domain.Archive{
		ID: idOf(2), TakenAt: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC),
		Mode: domain.ModeIncremental, ParentID: full.ID,
	}
	newest := domain.Archive{
		ID: idOf(3), TakenAt: time.Date(2026, 1, 3, 3, 0, 0, 0, time.UTC),
		Mode: domain.ModeIncremental, ParentID: middle.ID,
	}

	// A plan that would otherwise keep only the newest.
	expiry := domain.Retention{KeepLast: 1, MinKeep: 1}.Apply([]domain.Archive{full, middle, newest}, time.UTC)

	if len(expiry.Expire) != 0 {
		t.Fatalf("%d archives of the chain were let go: %+v", len(expiry.Expire), expiry.Expire)
	}
	if len(expiry.ChainHeld) != 2 {
		t.Fatalf("the answer names %d archives held for the chain, want 2", len(expiry.ChainHeld))
	}
}

// A chain whose newest member is not kept goes whole, which is the case that makes the rule useful
// rather than an excuse to keep everything.
func TestAChainNothingNeedsGoesWhole(t *testing.T) {
	old := []domain.Archive{
		{ID: idOf(1), TakenAt: time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC), Mode: domain.ModeFull},
		{ID: idOf(2), TakenAt: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC),
			Mode: domain.ModeIncremental, ParentID: idOf(1)},
	}
	fresh := domain.Archive{
		ID: idOf(3), TakenAt: time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC), Mode: domain.ModeFull,
	}

	expiry := domain.Retention{KeepLast: 1, MinKeep: 1}.Apply(append(old, fresh), time.UTC)

	if len(expiry.Keep) != 1 || expiry.Keep[0].ID != fresh.ID {
		t.Fatalf("kept %+v, want only the fresh full archive", expiry.Keep)
	}
	if len(expiry.Expire) != 2 {
		t.Fatalf("expired %d, want the whole old chain", len(expiry.Expire))
	}
}

// A parent that points at itself is a shape a target somebody has been editing can produce. It
// must not take the process with it.
func TestAChainThatPointsAtItselfTerminates(t *testing.T) {
	looping := []domain.Archive{
		{ID: idOf(1), TakenAt: time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC), ParentID: idOf(2)},
		{ID: idOf(2), TakenAt: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC), ParentID: idOf(1)},
	}

	expiry := domain.Retention{KeepLast: 1, MinKeep: 1}.Apply(looping, time.UTC)

	if len(expiry.Keep) != 2 {
		t.Fatalf("kept %+v", expiry.Keep)
	}
}

// A day is a day where the operator is.
func TestTheGenerationsAreCountedInTheSchedulesZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone: %v", err)
	}
	// Two runs on one Berlin day that fall on two different UTC days: 00:30 and 23:30 Berlin.
	archives := []domain.Archive{
		{ID: idOf(1), TakenAt: time.Date(2026, 6, 10, 0, 30, 0, 0, berlin)},
		{ID: idOf(2), TakenAt: time.Date(2026, 6, 10, 23, 30, 0, 0, berlin)},
	}
	// Two days of dailies: in Berlin the two runs share one, so one archive is kept and the other
	// is let go; in UTC they are two, so both are.
	plan := domain.Retention{KeepDaily: 2, MinKeep: 1}

	inBerlin := plan.Apply(archives, berlin)
	if len(inBerlin.Keep) != 1 {
		t.Fatalf("in Berlin the two runs are one day and kept %d", len(inBerlin.Keep))
	}
	inUTC := plan.Apply(archives, time.UTC)
	if len(inUTC.Keep) != 2 {
		t.Fatalf("in UTC the two runs are two days and kept %d", len(inUTC.Keep))
	}
}

// The same input decides the same way twice, whatever order it arrives in.
func TestThePlanIsStable(t *testing.T) {
	plan := domain.Retention{KeepLast: 3, KeepDaily: 5, KeepWeekly: 2, MinKeep: 2}
	archives := daily(40)

	first := plan.Apply(archives, time.UTC)
	shuffled := slices.Clone(archives)
	slices.Reverse(shuffled)
	second := plan.Apply(shuffled, time.UTC)

	if len(first.Keep) != len(second.Keep) {
		t.Fatalf("%d against %d", len(first.Keep), len(second.Keep))
	}
	for i := range first.Keep {
		if first.Keep[i].ID != second.Keep[i].ID {
			t.Fatalf("the answer depends on the order it was asked in: %s against %s",
				first.Keep[i].ID, second.Keep[i].ID)
		}
	}
}

func TestNothingIsDecidedAboutAnEmptyTarget(t *testing.T) {
	expiry := domain.DefaultRetention().Apply(nil, time.UTC)

	if len(expiry.Keep) != 0 || len(expiry.Expire) != 0 {
		t.Fatalf("an empty target produced %+v", expiry)
	}
}
