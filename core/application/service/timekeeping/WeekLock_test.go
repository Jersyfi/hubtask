// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// UC-TIM-02 check 4: a record written while its week is handed in on another device never lands in
// the handed-in week - the write and the hand-in each hold the person's week before reading it, so
// one waits for the other instead of both reading it open.
func TestAWeekIsHeldBeforeItIsReadForAWriteOrAHandIn(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	heldFirst := func(what string) {
		t.Helper()
		if len(f.store.steps) == 0 || !strings.HasPrefix(f.store.steps[0], "lock ") {
			t.Fatalf("%s read the week before holding it: %v", what, f.store.steps)
		}
		read := slices.IndexFunc(f.store.steps, func(s string) bool { return strings.HasPrefix(s, "read ") })
		if read < 0 || strings.TrimPrefix(f.store.steps[read], "read ") != strings.TrimPrefix(f.store.steps[0], "lock ") {
			t.Fatalf("%s held another week than it read: %v", what, f.store.steps)
		}
		f.store.steps = nil
	}
	r := f.log(t, ana, checkout, "2026-11-10", 60)
	heldFirst("a record's write")
	if _, err := (UpdateTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), r.ID, r.Version, RecordFields{
		EntryID: checkout, WorkType: "development", Day: day(t, "2026-11-03"), Minutes: 30,
	}, time.UTC); err != nil {
		t.Fatal(err)
	}
	if len(f.store.steps) < 4 || f.store.steps[0] >= f.store.steps[2] {
		t.Fatalf("a record moved between weeks holds them out of order: %v", f.store.steps)
	}
	f.store.steps = nil
	if _, err := (SubmitTimesheet{Keeper: f.keeper}).Execute(ctx, as(ana), day(t, "2026-11-04")); err != nil {
		t.Fatal(err)
	}
	heldFirst("a hand-in")
	if got := f.store.steps; len(got) != 0 {
		t.Fatalf("steps left: %v", got)
	}
}
