// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A kept count is shared by exactly the readers whose basis is the same (migration 0292): what the
// kind reads of the reader's day enters the key, nothing else does.
func TestAFiguresBasisHoldsWhatItsKindReads(t *testing.T) {
	berlin, auckland := zone(t, "Europe/Berlin"), zone(t, "Pacific/Auckland")
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	inBerlin := NewFigureWindow(now, berlin, "MONDAY")
	inAuckland := NewFigureWindow(now, auckland, "MONDAY")
	tomorrow := NewFigureWindow(now.Add(24*time.Hour), berlin, "MONDAY")
	request := func(kind Figure) FigureRequest {
		return FigureRequest{CollectionID: shared.ID("c"), Kind: kind,
			Offered: []ItemType{ItemTask, ItemWorkPackage}, Dated: []ItemType{ItemTask}}
	}
	cases := []struct {
		name string
		kind Figure
		a, b FigureWindow
		same bool
	}{
		{"progress is nobody's day", FigureProgress, inBerlin, inAuckland, true},
		{"open entries are nobody's day", FigureOpen, inBerlin, tomorrow, true},
		{"due is the reader's day", FigureDue, inBerlin, inAuckland, false},
		{"due moves with the day", FigureDue, inBerlin, tomorrow, false},
		{"due in one day is one count", FigureDue, inBerlin, NewFigureWindow(now.Add(time.Hour), berlin, "MONDAY"), true},
		{"the rhythm is the reader's zone", FigureRhythm, inBerlin, inAuckland, false},
		{"the rhythm moves with the week start", FigureRhythm, inBerlin, NewFigureWindow(now, berlin, "SUNDAY"), false},
		{"flow is nobody's day", FigureFlow, inBerlin, inAuckland, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := request(tc.kind).Basis(tc.a).Key(), request(tc.kind).Basis(tc.b).Key()
			if (a == b) != tc.same {
				t.Fatalf("keys %q and %q: same = %v, want %v", a, b, a == b, tc.same)
			}
		})
	}

	narrowed := request(FigureProgress)
	narrowed.Offered = []ItemType{ItemTask}
	if narrowed.Basis(inBerlin).Key() == request(FigureProgress).Basis(inBerlin).Key() {
		t.Fatal("a narrowing must be a basis of its own")
	}
	summed := request(FigureFieldSum)
	summed.FieldKey = "price"
	if summed.Basis(inBerlin).Key() == request(FigureFieldSum).Basis(inBerlin).Key() {
		t.Fatal("another field must be a basis of its own")
	}
}

func TestOnlyTheKindsTheClockMovesAreKeptForAMinute(t *testing.T) {
	for _, kind := range []Figure{FigureOpen, FigureFieldSum, FigureDue, FigureProgress, FigureFlow, FigureRhythm} {
		clocked := kind == FigureFlow || kind == FigureRhythm
		if got := kind.KeptFor(); (got == FigureClockBound) != clocked || (got == 0) == clocked {
			t.Errorf("%s kept for %v", kind, got)
		}
		if kind.Recountable() == clocked {
			t.Errorf("%s recountable = %v", kind, kind.Recountable())
		}
	}
}

// A basis taken from a request gives the same request back, and the window the recount needs.
func TestABasisGivesItsRequestBack(t *testing.T) {
	window := NewFigureWindow(time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC), zone(t, "Europe/Berlin"), "MONDAY")
	request := FigureRequest{CollectionID: shared.ID("c"), Kind: FigureDue,
		Offered: []ItemType{ItemTask}, Dated: []ItemType{ItemTask}, FieldKey: ""}
	basis := request.Basis(window)
	again := basis.Request(request.CollectionID, request.Kind)
	if again.Basis(window).Key() != basis.Key() {
		t.Fatalf("request back %+v", again)
	}
	recount := basis.Window(window.Now)
	if recount.Today != window.Today || !recount.DayEnd.Equal(window.DayEnd) {
		t.Fatalf("window back %+v, want today %s and day end %v", recount, window.Today, window.DayEnd)
	}
}

func TestSameCountsComparesTheFigure(t *testing.T) {
	sum, other, average := "12.50", "12.5", 3.0
	cases := []struct {
		name string
		a, b FigureCounts
		same bool
	}{
		{"equal", FigureCounts{Open: 2, Sum: &sum}, FigureCounts{Open: 2, Sum: &sum}, true},
		{"open differs", FigureCounts{Open: 2}, FigureCounts{Open: 3}, false},
		{"a sum and none", FigureCounts{Sum: &sum}, FigureCounts{}, false},
		{"another sum text", FigureCounts{Sum: &sum}, FigureCounts{Sum: &other}, false},
		{"an average and none", FigureCounts{AverageSeconds: &average}, FigureCounts{}, false},
		{"days in any order", FigureCounts{Days: []string{"2026-10-20", "2026-10-19"}},
			FigureCounts{Days: []string{"2026-10-19", "2026-10-20"}}, true},
		{"progress", FigureCounts{Done: 1, Total: 4}, FigureCounts{Done: 2, Total: 4}, false},
	}
	for _, tc := range cases {
		if got := SameCounts(tc.a, tc.b); got != tc.same {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}
