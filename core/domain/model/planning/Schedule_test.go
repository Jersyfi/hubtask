// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// weekdays works Monday to Friday, with the holidays given.
type weekdays map[Day]bool

func (h weekdays) WorksOn(year int, month time.Month, day int) bool {
	d := DayFrom(year, month, day)
	weekday := d.In(time.UTC).Weekday()
	return weekday != time.Saturday && weekday != time.Sunday && !h[d]
}

// 2026-11-02 is a Monday.
func day(t *testing.T, text string) Day {
	t.Helper()
	at, err := time.Parse(time.DateOnly, text)
	if err != nil {
		t.Fatal(err)
	}
	return DayFrom(at.Year(), at.Month(), at.Day())
}

func ptr(d Day) *Day { return &d }

func entry(t *testing.T, id string, mode Mode, start, finish string) Entry {
	e := Entry{ID: shared.ID(id), Mode: mode}
	if start != "" {
		e.Start = ptr(day(t, start))
	}
	if finish != "" {
		e.Finish = ptr(day(t, finish))
	}
	return e
}

func placed(t *testing.T, r Result, id string) Placement {
	t.Helper()
	for _, p := range r.Placements {
		if p.ID == shared.ID(id) {
			return p
		}
	}
	t.Fatalf("%s not placed", id)
	return Placement{}
}

func TestScheduleMovesAutomaticSuccessorsByKindAndLag(t *testing.T) {
	cases := []struct {
		name          string
		timing        Timing
		start, finish string
	}{
		// A: Mon 2 - Wed 4 (3 days); B lasts 2 days.
		{"finish to start", Timing{FinishToStart, 0}, "2026-11-05", "2026-11-06"},
		{"finish to start with lag over a weekend", Timing{FinishToStart, 2}, "2026-11-09", "2026-11-10"},
		{"finish to start with a lead", Timing{FinishToStart, -1}, "2026-11-04", "2026-11-05"},
		{"start to start", Timing{StartToStart, 1}, "2026-11-03", "2026-11-04"},
		{"finish to finish", Timing{FinishToFinish, 0}, "2026-11-03", "2026-11-04"},
		{"start to finish", Timing{StartToFinish, 0}, "2026-10-29", "2026-10-30"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := Schedule(Plan{
				Calendar: weekdays{},
				Entries: []Entry{
					entry(t, "a", Manual, "2026-11-02", "2026-11-04"),
					entry(t, "b", Automatic, "2026-11-20", "2026-11-23"),
				},
				Dependencies: []Dependency{{Predecessor: "a", Successor: "b", Timing: c.timing}},
			})
			if err != nil {
				t.Fatal(err)
			}
			b := placed(t, r, "b")
			if b.Start != day(t, c.start) || b.Finish != day(t, c.finish) || !b.Moved || b.Duration != 2 {
				t.Errorf("b = %s..%s moved=%v duration=%d, want %s..%s", b.Start, b.Finish, b.Moved, b.Duration, c.start, c.finish)
			}
		})
	}
}

func TestScheduleSkipsHolidays(t *testing.T) {
	r, err := Schedule(Plan{
		Calendar: weekdays{day(t, "2026-11-05"): true},
		Entries: []Entry{
			entry(t, "a", Manual, "2026-11-02", "2026-11-04"),
			entry(t, "b", Automatic, "2026-11-02", "2026-11-03"),
		},
		Dependencies: []Dependency{{Predecessor: "a", Successor: "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b := placed(t, r, "b"); b.Start != day(t, "2026-11-06") || b.Finish != day(t, "2026-11-09") {
		t.Errorf("b = %s..%s, want the Friday after the holiday and the Monday", b.Start, b.Finish)
	}
}

func TestScheduleKeepsManualEntriesAndNamesTheConflict(t *testing.T) {
	r, err := Schedule(Plan{
		Calendar: weekdays{},
		Entries: []Entry{
			entry(t, "a", Manual, "2026-11-02", "2026-11-04"),
			entry(t, "b", Manual, "2026-11-03", "2026-11-03"),
		},
		Dependencies: []Dependency{{Predecessor: "a", Successor: "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b := placed(t, r, "b"); b.Moved || b.Start != day(t, "2026-11-03") {
		t.Errorf("a manual entry moved: %+v", b)
	}
	if len(r.Conflicts) != 1 || r.Conflicts[0].Successor != "b" || r.Conflicts[0].Earliest != day(t, "2026-11-05") {
		t.Errorf("conflicts = %+v", r.Conflicts)
	}
	if len(r.Moves()) != 0 {
		t.Errorf("moves = %+v", r.Moves())
	}
}

func TestScheduleNeverMovesDoneEntries(t *testing.T) {
	done := entry(t, "b", Automatic, "2026-11-03", "2026-11-03")
	done.Done = true
	r, err := Schedule(Plan{
		Calendar:     weekdays{},
		Entries:      []Entry{entry(t, "a", Manual, "2026-11-02", "2026-11-04"), done},
		Dependencies: []Dependency{{Predecessor: "a", Successor: "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if placed(t, r, "b").Moved {
		t.Error("finished work moved")
	}
}

func TestScheduleFindsTheCriticalPathAndFloat(t *testing.T) {
	// a (3d) -> b (2d) -> d (1d); a -> c (1d) -> d. c has one day of float... two: b is 2, c is 1.
	r, err := Schedule(Plan{
		Calendar: weekdays{},
		Entries: []Entry{
			entry(t, "a", Manual, "2026-11-02", "2026-11-04"),
			entry(t, "b", Automatic, "2026-11-05", "2026-11-06"),
			entry(t, "c", Automatic, "2026-11-05", "2026-11-05"),
			entry(t, "d", Automatic, "2026-11-09", "2026-11-09"),
		},
		Dependencies: []Dependency{
			{Predecessor: "a", Successor: "b"}, {Predecessor: "a", Successor: "c"},
			{Predecessor: "b", Successor: "d"}, {Predecessor: "c", Successor: "d"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.CriticalPath, []shared.ID{"a", "b", "d"}) {
		t.Errorf("critical path = %v", r.CriticalPath)
	}
	if c := placed(t, r, "c"); c.TotalFloat != 1 || c.FreeFloat != 1 || c.Critical {
		t.Errorf("c = %+v, want one day of float", c)
	}
	if r.Finish != day(t, "2026-11-09") {
		t.Errorf("finish = %s", r.Finish)
	}
	if len(r.Moves()) != 0 {
		t.Errorf("a plan already in place proposed moves: %+v", r.Moves())
	}
}

func TestScheduleRefusesALoop(t *testing.T) {
	_, err := Schedule(Plan{
		Calendar: weekdays{},
		Entries: []Entry{
			entry(t, "a", Automatic, "2026-11-02", "2026-11-02"),
			entry(t, "b", Automatic, "2026-11-03", "2026-11-03"),
		},
		Dependencies: []Dependency{{Predecessor: "a", Successor: "b"}, {Predecessor: "b", Successor: "a"}},
	})
	var e *shared.Error
	if !errors.As(err, &e) || e.DetailCode != "planning.cycle" {
		t.Fatalf("err = %v, want planning.cycle", err)
	}
}

func TestScheduleRollsUpAnAutomaticParent(t *testing.T) {
	child1 := entry(t, "c1", Manual, "2026-11-03", "2026-11-04")
	child1.ParentID = "p"
	child2 := entry(t, "c2", Manual, "2026-11-09", "2026-11-10")
	child2.ParentID = "p"
	r, err := Schedule(Plan{
		Calendar: weekdays{},
		Entries:  []Entry{entry(t, "p", Automatic, "2026-11-02", "2026-11-02"), child1, child2},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := placed(t, r, "p")
	if !p.Summary || !p.Moved || p.Start != day(t, "2026-11-03") || p.Finish != day(t, "2026-11-10") {
		t.Errorf("parent = %+v", p)
	}
}

func TestScheduleMilestoneMeetsItsPredecessorsFinish(t *testing.T) {
	m := entry(t, "m", Automatic, "", "2026-11-20")
	m.Milestone = true
	r, err := Schedule(Plan{
		Calendar:     weekdays{},
		Entries:      []Entry{entry(t, "a", Manual, "2026-11-02", "2026-11-06"), m},
		Dependencies: []Dependency{{Predecessor: "a", Successor: "m"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := placed(t, r, "m"); got.Start != day(t, "2026-11-06") || got.Finish != got.Start || got.Duration != 0 {
		t.Errorf("milestone = %+v, want the Friday a finishes", got)
	}
}

func TestScheduleLeavesUndatedEntriesOut(t *testing.T) {
	r, err := Schedule(Plan{Calendar: weekdays{}, Entries: []Entry{{ID: "x", Mode: Automatic}}})
	if err != nil || !slices.Equal(r.Unscheduled, []shared.ID{"x"}) || len(r.Placements) != 0 {
		t.Errorf("r = %+v, err = %v", r, err)
	}
}

func TestScheduleIsDeterministic(t *testing.T) {
	plan := Plan{
		Calendar: weekdays{},
		Entries: []Entry{
			entry(t, "a", Manual, "2026-11-02", "2026-11-04"),
			entry(t, "b", Automatic, "2026-12-01", "2026-12-02"),
			entry(t, "c", Automatic, "2026-12-01", "2026-12-03"),
		},
		Dependencies: []Dependency{{Predecessor: "a", Successor: "b"}, {Predecessor: "b", Successor: "c", Timing: Timing{StartToStart, 1}}},
	}
	first, _ := Schedule(plan)
	for range 5 {
		again, _ := Schedule(plan)
		if !slices.Equal(first.Placements, again.Placements) {
			t.Fatal("the same plan answered different dates")
		}
	}
}

func TestNewTimingBounds(t *testing.T) {
	cases := []struct {
		kind DependencyKind
		lag  int
		code string
	}{
		{"", 0, ""}, {StartToFinish, -999, ""}, {"XX", 0, "planning.dependency_kind_unknown"},
		{FinishToStart, 1000, "planning.lag_out_of_range"},
	}
	for _, c := range cases {
		timing, err := NewTiming(c.kind, c.lag)
		var e *shared.Error
		switch {
		case c.code == "" && (err != nil || timing.Kind == ""):
			t.Errorf("%q %d: %v", c.kind, c.lag, err)
		case c.code != "" && (!errors.As(err, &e) || e.DetailCode != c.code):
			t.Errorf("%q %d: err = %v, want %s", c.kind, c.lag, err, c.code)
		}
	}
}

func TestAddWorkdaysAndBetween(t *testing.T) {
	if got, _ := AddWorkdays(weekdays{}, day(t, "2026-11-06"), 1); got != day(t, "2026-11-09") {
		t.Errorf("Friday + 1 = %s", got)
	}
	if got, _ := AddWorkdays(weekdays{}, day(t, "2026-11-09"), -1); got != day(t, "2026-11-06") {
		t.Errorf("Monday - 1 = %s", got)
	}
	if n, _ := WorkdaysBetween(weekdays{}, day(t, "2026-11-02"), day(t, "2026-11-15")); n != 10 {
		t.Errorf("two weeks = %d working days", n)
	}
}

func TestCompareBaselineWithToday(t *testing.T) {
	at := func(s string) *time.Time { v, _ := time.Parse(time.DateOnly, s); return &v }
	baseline := []BaselineEntry{
		{EntryID: "a", StartAt: at("2026-11-02"), DueAt: at("2026-11-04")},
		{EntryID: "gone", DueAt: at("2026-11-04")},
	}
	current := []BaselineEntry{
		{EntryID: "a", StartAt: at("2026-11-03"), DueAt: at("2026-11-09")},
		{EntryID: "new", DueAt: at("2026-11-10")},
	}
	got := Compare(baseline, current, time.UTC)
	if len(got) != 3 || *got[0].StartVarianceDays != 1 || *got[0].FinishVarianceDays != 5 {
		t.Fatalf("variance = %+v", got)
	}
	if got[1].InPlan || got[1].FinishVarianceDays != nil || got[2].InBaseline || !got[2].InPlan {
		t.Errorf("gone and new = %+v %+v", got[1], got[2])
	}
}

func TestNewBaselineName(t *testing.T) {
	in := NewBaselineInput{ID: "b", TenantID: "t", PlanID: "p", Name: "  Agreed in board  "}
	if b, err := NewBaseline(in); err != nil || b.Name != "Agreed in board" {
		t.Errorf("b = %+v, err = %v", b, err)
	}
	in.Name = " "
	if _, err := NewBaseline(in); err == nil {
		t.Error("an empty name was accepted")
	}
}
