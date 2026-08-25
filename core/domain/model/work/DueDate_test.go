// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// dueProfile is a task profile that carries the capability, which the default taskProfile()
// fixture deliberately does not - the matrix gives every type DUE_DATE by default, and the
// narrowed fixture is what the negative test needs (domain-model.md §2).
func dueProfile() CapabilityProfile {
	profile := taskProfile()
	profile.Capabilities = append(profile.Capabilities, CapabilityDueDate)
	return profile
}

func parseInstant(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &parsed
}

// The three refusals the backlog names, each with a stable code and a field error, and the
// combinations that are due dates. The rule is about the resulting trio rather than the request:
// a zone or a flag qualifying an instant that is not there is a row whose meaning depends on a
// field it does not have (i18n-l10n.md §4).
func TestADueDateIsTheTrioOrNothing(t *testing.T) {
	for name, test := range map[string]struct {
		at       *time.Time
		dateOnly bool
		zone     string

		want     *DueDate
		wantCode string
		wantPath string
	}{
		"a plain instant": {
			at:   parseInstant("2026-09-01T17:00:00Z"),
			want: &DueDate{At: time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC)},
		},
		"an instant with its zone": {
			at:   parseInstant("2026-09-01T17:00:00+02:00"),
			zone: "Europe/Berlin",
			want: &DueDate{
				At:       time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC),
				TimeZone: "Europe/Berlin",
			},
		},
		"an all-day date in its zone": {
			at:       parseInstant("2026-09-01T00:00:00+02:00"),
			dateOnly: true,
			zone:     "Europe/Berlin",
			want: &DueDate{
				At:       time.Date(2026, 8, 31, 22, 0, 0, 0, time.UTC),
				DateOnly: true, TimeZone: "Europe/Berlin",
			},
		},
		"no due date at all": {},
		"a zone that is not an IANA name": {
			at:       parseInstant("2026-09-01T17:00:00Z"),
			zone:     "CEST",
			wantCode: "items.due_time_zone_invalid",
			wantPath: "/due_time_zone",
		},
		"a zone without a date": {
			zone:     "Europe/Berlin",
			wantCode: "items.due_time_zone_without_date",
			wantPath: "/due_time_zone",
		},
		"a flag without a date": {
			dateOnly: true,
			wantCode: "items.due_date_only_without_date",
			wantPath: "/due_date_only",
		},
	} {
		t.Run(name, func(t *testing.T) {
			due, err := NewDueDate(test.at, test.dateOnly, test.zone)

			if test.wantCode != "" {
				if !errors.Is(err, shared.ErrValidation) {
					t.Fatalf("the refusal is %v", err)
				}
				var domainErr *shared.Error
				if !errors.As(err, &domainErr) {
					t.Fatalf("the refusal is not a domain error: %v", err)
				}
				if domainErr.DetailCode != test.wantCode {
					t.Errorf("the detail code is %q rather than %q", domainErr.DetailCode, test.wantCode)
				}
				if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != test.wantPath {
					t.Errorf("the refusal does not point at %s: %v", test.wantPath, domainErr.Fields)
				}
				return
			}

			if err != nil {
				t.Fatalf("the due date was refused: %v", err)
			}
			if !due.Equal(test.want) {
				t.Errorf("the due date is %+v rather than %+v", due, test.want)
			}
			if due != nil && due.At.Location() != time.UTC {
				t.Errorf("the instant is stored in %v rather than UTC", due.At.Location())
			}
		})
	}
}

// The acceptance sentence, at the domain's level: the stored trio is one value for every reader.
// An all-day date set from Berlin and read from São Paulo is the same instant, the same flag and
// the same zone - nothing about the reader enters the answer, because nothing the server stores
// depends on who asks.
func TestAnAllDayDueDateAnswersIdenticallyToEveryReader(t *testing.T) {
	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("the zone does not load: %v", err)
	}

	setInBerlin := parseInstant("2026-09-01T00:00:00+02:00")
	viewedElsewhere := setInBerlin.In(saoPaulo)

	fromBerlin, err := NewDueDate(setInBerlin, true, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	fromElsewhere, err := NewDueDate(&viewedElsewhere, true, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}

	if !fromBerlin.Equal(fromElsewhere) {
		t.Errorf("the same due date answers differently: %+v and %+v", fromBerlin, fromElsewhere)
	}
}

// The capability matrix: a type whose profile does not carry DUE_DATE cannot be given one, by
// name rather than by silence - and because every system profile carries it, the case is a
// tenant-narrowed profile rather than a type (domain-model.md §2).
func TestADueDateOnANarrowedProfileIsRefused(t *testing.T) {
	item := updatable(t)

	due, err := NewDueDate(parseInstant("2026-09-01T17:00:00Z"), false, "")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}

	_, _, err = item.WithDueDate(due, taskProfile(), laterOn)
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("a due date on a narrowed profile answered %v", err)
	}

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("the refusal is not a domain error: %v", err)
	}
	if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/due_at" {
		t.Errorf("the refusal does not point at /due_at: %v", domainErr.Fields)
	}
}

// Clearing what the profile does not carry asks for the state the entry is already in - the same
// view Updated takes of empty notes on an activity, and NewWorkItem of empty fields at creation.
func TestClearingADueDateOnANarrowedProfileIsAllowed(t *testing.T) {
	item := updatable(t)

	after, changes, err := item.WithDueDate(nil, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("clearing was refused: %v", err)
	}
	if len(changes) != 0 || after.Due != nil {
		t.Errorf("clearing nothing reported %v", changes)
	}
}

// I-W4: a trashed or archived entry is read-only, and the state comes back as a conflict rather
// than a validation failure - the request is well formed and restoring the item is what helps.
func TestADueDateOnAReadOnlyItemIsRefused(t *testing.T) {
	trashed := updatable(t)
	trashed.DeletedAt = &laterOn

	due, err := NewDueDate(parseInstant("2026-09-01T17:00:00Z"), false, "")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}

	if _, _, err := trashed.WithDueDate(due, dueProfile(), laterOn); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a due date on a trashed entry answered %v", err)
	}
}

// One change log entry per field that moved, and none for one that did not: two devices moving
// the date and the zone independently converge to both, which is the scalar row of
// offline-sync.md §4.2 and the reason the trio does not travel as one entry.
func TestEachFieldOfTheTrioMovesAlone(t *testing.T) {
	withDue := updatable(t)
	due, err := NewDueDate(parseInstant("2026-09-01T17:00:00Z"), false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the fixture due date was refused: %v", err)
	}
	withDue.Due = due

	for name, test := range map[string]struct {
		target      *DueDate
		wantChanges map[string][2]string
	}{
		"moving the date alone": {
			target: &DueDate{
				At: time.Date(2026, 9, 4, 17, 0, 0, 0, time.UTC), TimeZone: "Europe/Berlin",
			},
			wantChanges: map[string][2]string{
				FieldDueAt: {"2026-09-01T17:00:00Z", "2026-09-04T17:00:00Z"},
			},
		},
		"moving the zone alone": {
			target: &DueDate{
				At: time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC), TimeZone: "America/Sao_Paulo",
			},
			wantChanges: map[string][2]string{
				FieldDueTimeZone: {"Europe/Berlin", "America/Sao_Paulo"},
			},
		},
		"raising the flag alone": {
			target: &DueDate{
				At: time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC), DateOnly: true,
				TimeZone: "Europe/Berlin",
			},
			wantChanges: map[string][2]string{
				FieldDueDateOnly: {"false", "true"},
			},
		},
		"clearing": {
			target: nil,
			wantChanges: map[string][2]string{
				FieldDueAt:       {"2026-09-01T17:00:00Z", ""},
				FieldDueTimeZone: {"Europe/Berlin", ""},
			},
		},
		"the same due date again": {
			target:      &DueDate{At: time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC), TimeZone: "Europe/Berlin"},
			wantChanges: map[string][2]string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			after, changes, err := withDue.WithDueDate(test.target, dueProfile(), laterOn)
			if err != nil {
				t.Fatalf("the due date was refused: %v", err)
			}

			if len(changes) != len(test.wantChanges) {
				t.Fatalf("%d changes were reported rather than %d: %v",
					len(changes), len(test.wantChanges), changes)
			}
			for _, change := range changes {
				want, known := test.wantChanges[change.Field]
				if !known {
					t.Errorf("%s was reported although it did not move", change.Field)
					continue
				}
				if change.From != want[0] || change.To != want[1] {
					t.Errorf("%s moved %q → %q rather than %q → %q",
						change.Field, change.From, change.To, want[0], want[1])
				}
			}
			if len(test.wantChanges) == 0 {
				if !after.UpdatedAt.Equal(withDue.UpdatedAt) {
					t.Errorf("the timestamp moved although nothing changed")
				}
				if !after.Due.Equal(withDue.Due) {
					t.Errorf("the due date moved although nothing changed")
				}
			}
		})
	}
}

// Setting from nothing reports what is now there and stays quiet about what still is not: the
// flag of a plain-instant due date was false before and is false now, so no entry names it.
func TestSettingFromNothingReportsOnlyWhatIsNowThere(t *testing.T) {
	item := updatable(t)

	due, err := NewDueDate(parseInstant("2026-09-01T17:00:00Z"), false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}

	after, changes, err := item.WithDueDate(due, dueProfile(), laterOn)
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}

	want := map[string][2]string{
		FieldDueAt:       {"", "2026-09-01T17:00:00Z"},
		FieldDueTimeZone: {"", "Europe/Berlin"},
	}
	if len(changes) != len(want) {
		t.Fatalf("%d changes were reported rather than %d: %v", len(changes), len(want), changes)
	}
	for _, change := range changes {
		expected, known := want[change.Field]
		if !known {
			t.Errorf("%s was reported although it did not move", change.Field)
			continue
		}
		if change.From != expected[0] || change.To != expected[1] {
			t.Errorf("%s moved %q → %q rather than %q → %q",
				change.Field, change.From, change.To, expected[0], expected[1])
		}
	}
	if !after.UpdatedAt.Equal(laterOn) {
		t.Errorf("the timestamp is %s rather than %s", after.UpdatedAt, laterOn)
	}
}

// The start is a plain scalar attribute: it moves through the same diff as the title, clears
// through the zero time the way the board clears through the zero identifier, and the same
// moment written from another zone is not a change.
func TestTheStartMovesThroughTheAttributeDiff(t *testing.T) {
	item := updatable(t)

	after, changes, err := item.Updated(ItemAttributes{StartAt: parseInstant("2026-08-30T08:00:00Z")},
		taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("the start was refused: %v", err)
	}
	if len(changes) != 1 || changes[0].Field != FieldStartAt ||
		changes[0].From != "" || changes[0].To != "2026-08-30T08:00:00Z" {
		t.Fatalf("setting the start reported %v", changes)
	}
	if after.StartAt == nil || !after.StartAt.Equal(time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("the start is %v", after.StartAt)
	}

	// The same moment, written from another zone: not a change.
	_, changes, err = after.Updated(ItemAttributes{StartAt: parseInstant("2026-08-30T10:00:00+02:00")},
		taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("the repeated start was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("the same moment from another zone reported %v", changes)
	}

	// The zero time clears, the way merge patch's null reaches the domain.
	cleared, changes, err := after.Updated(ItemAttributes{StartAt: &time.Time{}}, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("clearing the start was refused: %v", err)
	}
	if len(changes) != 1 || changes[0].Field != FieldStartAt ||
		changes[0].From != "2026-08-30T08:00:00Z" || changes[0].To != "" {
		t.Fatalf("clearing the start reported %v", changes)
	}
	if cleared.StartAt != nil {
		t.Fatalf("the start is still %v", cleared.StartAt)
	}
}
