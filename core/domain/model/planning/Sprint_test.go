// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func sprintID(n byte) shared.ID {
	return shared.MustParseID("00000000-0000-4000-8000-0000000000" + string([]byte{'0' + n/10, '0' + n%10}))
}

func detailOf(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

type monToFri struct{}

func (monToFri) WorksOn(y int, m time.Month, d int) bool {
	w := time.Date(y, m, d, 12, 0, 0, 0, time.UTC).Weekday()
	return w != time.Saturday && w != time.Sunday
}

func TestNewSprintChecksItsFields(t *testing.T) {
	start := DayFrom(2026, time.November, 2)
	cases := []struct {
		name   string
		fields SprintFields
		code   string
	}{
		{"two weeks", SprintFields{Name: "Sprint 1", StartOn: start, EndOn: start + 13}, ""},
		{"no name", SprintFields{Name: " ", StartOn: start, EndOn: start}, "sprints.name_required"},
		{"two lines", SprintFields{Name: "a\nb", StartOn: start, EndOn: start}, "sprints.name_invalid"},
		{"backwards", SprintFields{Name: "S", StartOn: start, EndOn: start - 1}, "sprints.ends_before_start"},
		{"a quarter and a day", SprintFields{Name: "S", StartOn: start, EndOn: start + MaxSprintDays}, "sprints.too_long"},
		{"sizes are points", SprintFields{Name: "S", StartOn: start, EndOn: start, Unit: Size}, "sprints.unit_unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := NewSprint(NewSprintInput{ID: sprintID(1), TenantID: sprintID(2), CollectionID: sprintID(3), Fields: c.fields})
			if c.code == "" {
				if err != nil || s.State != SprintPlanned || s.Unit != Points {
					t.Fatalf("got %+v, %v", s, err)
				}
				return
			}
			if code := detailOf(err); code != c.code {
				t.Fatalf("code %q, want %q (%v)", code, c.code, err)
			}
		})
	}
}

func TestNewEstimate(t *testing.T) {
	cases := []struct {
		unit  EstimateUnit
		value float64
		size  string
		want  float64
		code  string
	}{
		{Points, 5, "", 5, ""},
		{Hours, 1.25, "", 1.25, ""},
		{Size, 0, "L", 5, ""},
		{Size, 0, "XXXL", 0, "estimates.size_unknown"},
		{Points, 1, "M", 0, "estimates.size_without_unit"},
		{Points, -1, "", 0, "estimates.value_out_of_range"},
		{Points, 0.125, "", 0, "estimates.value_out_of_range"},
		{"DAYS", 1, "", 0, "estimates.unit_unknown"},
	}
	for _, c := range cases {
		e, err := NewEstimate(sprintID(9), c.unit, c.value, c.size)
		if c.code != "" {
			if detailOf(err) != c.code {
				t.Errorf("%v %v %q: %v, want %s", c.unit, c.value, c.size, err, c.code)
			}
			continue
		}
		if err != nil || e.Value != c.want {
			t.Errorf("%v %v %q: %+v %v", c.unit, c.value, c.size, e, err)
		}
	}
	size, _ := NewEstimate(sprintID(9), Size, 0, "M")
	if v, ok := size.CountsIn(Points); !ok || v != 3 {
		t.Errorf("a size counts in points: %v %v", v, ok)
	}
	if _, ok := size.CountsIn(Hours); ok {
		t.Error("a size counts in hours")
	}
}

// UC-PLN-06 checks 2 and 3: the commitment at start, the result at close, the unfinished entries
// carried or returned.
func TestASprintRecordsItsCommitmentAndWhatWasDone(t *testing.T) {
	start := DayFrom(2026, time.November, 2)
	s, err := NewSprint(NewSprintInput{
		ID: sprintID(1), TenantID: sprintID(2), CollectionID: sprintID(3),
		Fields: SprintFields{Name: "S", StartOn: start, EndOn: start + 13},
	})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := sprintID(10), sprintID(11), sprintID(12)
	estimates := map[shared.ID]Estimate{
		a: {EntryID: a, Unit: Points, Value: 3},
		b: {EntryID: b, Unit: Size, Value: 5, Size: "L"},
		c: {EntryID: c, Unit: Hours, Value: 8},
	}
	entries := []SprintEntry{{EntryID: a}, {EntryID: b}, {EntryID: c}}
	now := start.In(time.UTC).Add(9 * time.Hour)
	if _, err := s.Close(entries, estimates, false, now); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("closing a planned sprint: %v", err)
	}
	if err := s.Start(entries, estimates, now); err != nil {
		t.Fatal(err)
	}
	if s.CommittedValue != 8 || s.CommittedCount != 3 || s.State != SprintActive {
		t.Fatalf("commitment %v of %d, %s", s.CommittedValue, s.CommittedCount, s.State)
	}
	if err := s.Change(SprintFields{Name: "S", StartOn: start + 1, EndOn: start + 13}, now, nil); err == nil {
		t.Fatal("a running sprint moved its start")
	}
	entries[0].Done = true
	for _, carry := range []bool{true, false} {
		closing := s
		outcomes, err := closing.Close(entries, estimates, carry, now)
		if err != nil {
			t.Fatal(err)
		}
		want := OutcomeReturned
		if carry {
			want = OutcomeCarried
		}
		if outcomes[a] != OutcomeDone || outcomes[b] != want || outcomes[c] != want {
			t.Errorf("carry %v: %v", carry, outcomes)
		}
		if closing.CompletedValue != 3 || closing.CompletedCount != 1 || closing.State != SprintClosed {
			t.Errorf("result %v of %d", closing.CompletedValue, closing.CompletedCount)
		}
		if err := closing.Change(SprintFields{Name: "T", StartOn: start, EndOn: start}, now, nil); !errors.Is(err, shared.ErrConflict) {
			t.Errorf("a closed sprint changed: %v", err)
		}
	}
}

// UC-PLN-07 check 3: the burndown falls as entries finish, rises when one joins, and the ideal
// line falls over working days only.
func TestBurndown(t *testing.T) {
	start := DayFrom(2026, time.November, 2) // a Monday
	at := func(day Day, hour int) time.Time { return day.In(time.UTC).Add(time.Duration(hour) * time.Hour) }
	started := at(start, 9)
	s := Sprint{StartOn: start, EndOn: start + 4, State: SprintActive, Unit: Points, CommittedValue: 10, StartedAt: &started}
	a, b, c := sprintID(10), sprintID(11), sprintID(12)
	done := at(start+1, 15)
	estimates := map[shared.ID]Estimate{a: {Unit: Points, Value: 4}, b: {Unit: Points, Value: 6}, c: {Unit: Points, Value: 2}}
	entries := []SprintEntry{
		{EntryID: a, AddedAt: at(start-1, 10), CompletedAt: &done},
		{EntryID: b, AddedAt: at(start-1, 10)},
		{EntryID: c, AddedAt: at(start+2, 11)},
	}
	points := Burndown(s, entries, estimates, monToFri{}, time.UTC, start+3)
	if len(points) != 4 {
		t.Fatalf("%d points", len(points))
	}
	want := []struct{ remaining, scope, ideal float64 }{{10, 10, 8}, {6, 10, 6}, {8, 12, 4}, {8, 12, 2}}
	for i, w := range want {
		p := points[i]
		if p.Remaining != w.remaining || p.Scope != w.scope || p.Ideal != w.ideal {
			t.Errorf("day %d: %+v, want %+v", i, p, w)
		}
	}
	s.State = SprintPlanned
	if Burndown(s, entries, estimates, monToFri{}, time.UTC, start+3) != nil {
		t.Error("a planned sprint burns down")
	}
}

func TestVelocityAveragesTheLatestThreeOfOneUnit(t *testing.T) {
	v := VelocityOf([]Sprint{
		{Unit: Points, CompletedValue: 10},
		{Unit: Hours, CompletedValue: 80},
		{Unit: Points, CompletedValue: 14},
		{Unit: Points, CompletedValue: 6},
		{Unit: Points, CompletedValue: 100},
	})
	if v.Unit != Points || v.Average != 10 {
		t.Fatalf("%+v", v)
	}
	if VelocityOf(nil).Average != 0 {
		t.Fatal("no sprint, a velocity")
	}
}
