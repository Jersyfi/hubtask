// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var recurringDue = time.Date(2026, 9, 7, 7, 0, 0, 0, time.UTC)

func seriesDue(t *testing.T) *work.DueDate {
	t.Helper()

	due, err := work.NewDueDate(&recurringDue, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the fixture due date does not build: %v", err)
	}
	return due
}

func weekly() work.RecurrenceSpec {
	return work.RecurrenceSpec{
		RRULE: "FREQ=WEEKLY;BYDAY=MO", TimeZone: "Europe/Berlin",
		Mode: string(work.RecurrenceOnSchedule),
	}
}

func draftSeries(t *testing.T, spec work.RecurrenceSpec) (work.RecurrenceRule, error) {
	t.Helper()

	return work.NewRecurrenceRule(work.NewRecurrenceRuleInput{
		ID: "r1", TenantID: "t1", ItemID: "i1", Spec: spec, Due: seriesDue(t), Now: remindedAt,
	})
}

// What a series may be, and every way of getting it wrong. The refusals are the substance: a rule
// that is stored and discovered broken by the scheduler is a series nobody can fix from the outside
// (security.md T-17, arc42 §11 R-07).
func TestASeriesIsCheckedBeforeItIsStored(t *testing.T) {
	future := recurringDue.Add(30 * 24 * time.Hour)
	past := recurringDue.Add(-time.Hour)

	for name, test := range map[string]struct {
		spec     func(work.RecurrenceSpec) work.RecurrenceSpec
		wantCode string
		wantPath string
	}{
		"a weekly series": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec { return s },
		},
		"an end by date": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec { s.EndsAt = &future; return s },
		},
		"an end by count": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec { s.MaxCount = 10; return s },
		},
		"no rule at all": {
			spec:     func(s work.RecurrenceSpec) work.RecurrenceSpec { s.RRULE = "  "; return s },
			wantCode: "recurrence.rrule_required", wantPath: "/rrule",
		},
		"a rule carrying its own start": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec {
				s.RRULE = "DTSTART:20260907T070000Z;FREQ=WEEKLY"
				return s
			},
			wantCode: "recurrence.rrule_carries_start", wantPath: "/rrule",
		},
		"a rule carrying its own end": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec {
				s.RRULE = "FREQ=WEEKLY;COUNT=10"
				return s
			},
			wantCode: "recurrence.rrule_carries_end", wantPath: "/rrule",
		},
		"a rule set rather than a rule": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec {
				s.RRULE = "FREQ=WEEKLY\nEXDATE:20260914T070000Z"
				return s
			},
			wantCode: "recurrence.rrule_not_single", wantPath: "/rrule",
		},
		"no zone": {
			spec:     func(s work.RecurrenceSpec) work.RecurrenceSpec { s.TimeZone = ""; return s },
			wantCode: "recurrence.time_zone_required", wantPath: "/time_zone",
		},
		"a zone that is not an IANA name": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec {
				s.TimeZone = "CEST"
				return s
			},
			wantCode: "recurrence.time_zone_invalid", wantPath: "/time_zone",
		},
		"a mode nobody defined": {
			spec:     func(s work.RecurrenceSpec) work.RecurrenceSpec { s.Mode = "WHENEVER"; return s },
			wantCode: "recurrence.mode_unknown", wantPath: "/mode",
		},
		"a horizon beyond the year": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec {
				s.HorizonDays = work.MaxHorizonDays + 1
				return s
			},
			wantCode: "recurrence.horizon_out_of_range", wantPath: "/horizon_days",
		},
		"both ends at once": {
			spec: func(s work.RecurrenceSpec) work.RecurrenceSpec {
				s.EndsAt, s.MaxCount = &future, 10
				return s
			},
			wantCode: "recurrence.end_spec_ambiguous", wantPath: "/ends_at",
		},
		"an end before the start": {
			spec:     func(s work.RecurrenceSpec) work.RecurrenceSpec { s.EndsAt = &past; return s },
			wantCode: "recurrence.end_before_start", wantPath: "/ends_at",
		},
	} {
		t.Run(name, func(t *testing.T) {
			rule, err := draftSeries(t, test.spec(weekly()))

			if test.wantCode != "" {
				refusal := shared.AsError(err)
				if refusal == nil || refusal.DetailCode != test.wantCode {
					t.Fatalf("refused as %v, want %s", err, test.wantCode)
				}
				if len(refusal.Fields) != 1 || refusal.Fields[0].Path != test.wantPath {
					t.Errorf("the refusal does not point at %s: %v", test.wantPath, refusal.Fields)
				}
				return
			}

			if err != nil {
				t.Fatalf("the series was refused: %v", err)
			}
			if rule.Version != 1 || rule.Mode != work.RecurrenceOnSchedule {
				t.Errorf("the series is %+v", rule)
			}
			// An omitted horizon is the column's own default rather than zero days, which would
			// be a window that owes nothing.
			if rule.HorizonDays != work.DefaultHorizonDays {
				t.Errorf("the horizon is %d days", rule.HorizonDays)
			}
		})
	}
}

// A series counts from the entry's due date, so an entry without one cannot carry a rule: there
// would be nothing to expand from, and a rule stored dormant is a series nobody can predict.
func TestASeriesNeedsTheEntryToHaveADueDate(t *testing.T) {
	_, err := work.NewRecurrenceRule(work.NewRecurrenceRuleInput{
		ID: "r1", TenantID: "t1", ItemID: "i1", Spec: weekly(), Now: remindedAt,
	})
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "recurrence.due_date_required" {
		t.Fatalf("refused as %v, want recurrence.due_date_required", err)
	}
}

// A change is a whole document, and what it reports is per field - which is what the merge needs.
func TestChangingASeriesReportsOneChangePerField(t *testing.T) {
	stored, err := draftSeries(t, weekly())
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}

	wanted := weekly()
	wanted.RRULE = "FREQ=WEEKLY;BYDAY=TU"
	wanted.HorizonDays = 30
	changed, changes, err := stored.Changed(wanted, seriesDue(t), remindedAt)
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}

	moved := map[string][2]string{}
	for _, change := range changes {
		moved[change.Field] = [2]string{change.From, change.To}
	}
	if len(moved) != 2 {
		t.Fatalf("the changes are %v", changes)
	}
	if got := moved["rrule"]; got != [2]string{"FREQ=WEEKLY;BYDAY=MO", "FREQ=WEEKLY;BYDAY=TU"} {
		t.Errorf("the rule change is %v", got)
	}
	if got := moved["horizon_days"]; got != [2]string{"90", "30"} {
		t.Errorf("the horizon change is %v", got)
	}
	if changed.UpdatedAt == nil || changed.Version != stored.Version {
		t.Errorf("the changed series is %+v", changed)
	}

	// A document that says what is already stored moves nothing, so the caller writes nothing.
	same, none, err := changed.Changed(wanted, seriesDue(t), remindedAt)
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	if len(none) != 0 || same.Version != changed.Version {
		t.Errorf("a change that changed nothing reported %v", none)
	}
}

// The capability matrix's note, enforced: a series applies to the whole subtree, so only a task
// carries one.
func TestOnlyATaskCarriesASeries(t *testing.T) {
	item := work.WorkItem{ID: "i1"}
	task := work.CapabilityProfile{
		Type: work.ItemTask, Capabilities: []work.Capability{work.CapabilityRecurrence},
	}
	if err := item.EnsureRecurring(task); err != nil {
		t.Fatalf("a task was refused a series: %v", err)
	}

	pack := work.CapabilityProfile{Type: work.ItemWorkPackage}
	if got := shared.AsError(item.EnsureRecurring(pack)).DetailCode; got !=
		"items.capability_not_supported" {
		t.Fatalf("refused as %q, want items.capability_not_supported", got)
	}

	trashed := work.WorkItem{ID: "i1", DeletedAt: &remindedAt}
	if err := trashed.EnsureRecurring(task); err == nil {
		t.Fatal("a trashed entry took a series")
	}
}

// The horizon is a window, and the window is what the materialisation will owe (D-05).
func TestTheHorizonIsAWindowFromAMoment(t *testing.T) {
	rule, err := draftSeries(t, weekly())
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}

	if got := rule.Horizon(recurringDue); !got.Equal(recurringDue.AddDate(0, 0, 90)) {
		t.Errorf("the horizon ends at %v", got)
	}
}
