// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func text(value string) *string { return &value }

// updatable is a task as it comes out of NewWorkItem, which is the only state an update ever sees.
func updatable(t *testing.T) WorkItem {
	t.Helper()

	item, err := NewWorkItem(taskInput())
	if err != nil {
		t.Fatalf("the fixture does not build: %v", err)
	}
	return item
}

var laterOn = time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)

// The heart of it: what was sent moves, what was not sent stays, and only what moved is reported.
// The last part is what the change log and the event both depend on - a field reported as changed
// when it did not move would be merged over a concurrent edit on another device.
func TestUpdatedReportsOnlyWhatMoved(t *testing.T) {
	before := updatable(t)
	before.Notes = "Semi-skimmed"

	for _, test := range []struct {
		name       string
		attributes ItemAttributes
		wantFields []string
		wantTitle  string
		wantNotes  string
	}{
		{
			name:       "the title alone",
			attributes: ItemAttributes{Title: text("Buy oat milk")},
			wantFields: []string{FieldTitle},
			wantTitle:  "Buy oat milk", wantNotes: "Semi-skimmed",
		},
		{
			name:       "the notes alone",
			attributes: ItemAttributes{Notes: text("Whole")},
			wantFields: []string{FieldNotes},
			wantTitle:  "Buy milk", wantNotes: "Whole",
		},
		{
			name:       "both at once",
			attributes: ItemAttributes{Title: text("Buy oat milk"), Notes: text("Whole")},
			wantFields: []string{FieldTitle, FieldNotes},
			wantTitle:  "Buy oat milk", wantNotes: "Whole",
		},
		{
			// Merge patch spells "clear it" as null, which reaches the domain as the empty string.
			name:       "clearing the notes",
			attributes: ItemAttributes{Notes: text("")},
			wantFields: []string{FieldNotes},
			wantTitle:  "Buy milk", wantNotes: "",
		},
		{
			// Sent, but identical to what is stored. Nothing moved, so nothing is reported - a
			// client that echoes the whole object back does not produce a change log entry per field
			// for every device to merge.
			name:       "a title equal to the one stored",
			attributes: ItemAttributes{Title: text("Buy milk")},
			wantFields: nil,
			wantTitle:  "Buy milk", wantNotes: "Semi-skimmed",
		},
		{
			name:       "surrounding whitespace is not a change",
			attributes: ItemAttributes{Title: text("  Buy milk  ")},
			wantFields: nil,
			wantTitle:  "Buy milk", wantNotes: "Semi-skimmed",
		},
		{
			name:       "an empty update",
			attributes: ItemAttributes{},
			wantFields: nil,
			wantTitle:  "Buy milk", wantNotes: "Semi-skimmed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			after, changes, err := before.Updated(test.attributes, taskProfile(), laterOn)
			if err != nil {
				t.Fatalf("the update was refused: %v", err)
			}

			if after.Title != test.wantTitle {
				t.Errorf("the title is %q rather than %q", after.Title, test.wantTitle)
			}
			if after.Notes != test.wantNotes {
				t.Errorf("the notes are %q rather than %q", after.Notes, test.wantNotes)
			}

			var fields []string
			for _, change := range changes {
				fields = append(fields, change.Field)
			}
			if len(fields) != len(test.wantFields) {
				t.Fatalf("the fields reported as changed are %v rather than %v", fields, test.wantFields)
			}
			for index, field := range test.wantFields {
				if fields[index] != field {
					t.Errorf("field %d is %q rather than %q", index, fields[index], field)
				}
			}
		})
	}
}

// A change carries both sides. The event's change set is old/new per field (domain-model.md §4),
// and the audit entry hashes both - neither can be built from the new value alone.
func TestAChangeCarriesBothSides(t *testing.T) {
	before := updatable(t)

	_, changes, err := before.Updated(ItemAttributes{Title: text("Buy oat milk")}, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("the update was refused: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("%d changes were reported rather than one", len(changes))
	}
	if changes[0].From != "Buy milk" || changes[0].To != "Buy oat milk" {
		t.Errorf("the change is %q → %q", changes[0].From, changes[0].To)
	}
}

// An update that changes nothing does not move the timestamp either. Whoever writes it reads the
// empty change set and writes nothing at all; a moved `updated_at` would be a change no field
// accounts for, and every offline device would fetch the item to find out what it was.
func TestAnUpdateThatChangesNothingLeavesTheTimestampAlone(t *testing.T) {
	before := updatable(t)

	after, changes, err := before.Updated(ItemAttributes{Title: text("Buy milk")}, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("the update was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("%d changes were reported rather than none", len(changes))
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("the timestamp moved to %s although nothing changed", after.UpdatedAt)
	}
}

func TestAnUpdateThatChangesSomethingMovesTheTimestamp(t *testing.T) {
	before := updatable(t)

	after, _, err := before.Updated(ItemAttributes{Title: text("Buy oat milk")}, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("the update was refused: %v", err)
	}
	if !after.UpdatedAt.Equal(laterOn) {
		t.Errorf("the timestamp is %s rather than %s", after.UpdatedAt, laterOn)
	}
	// The version is the writer's business: it is the database that increments it, against the
	// version the caller matched on, and a domain that guessed would be a second answer.
	if after.Version != before.Version {
		t.Errorf("the domain moved the version to %d", after.Version)
	}
}

// The capability matrix, which is the whole point of B-05: an activity carries no NOTES, so
// writing notes to one is refused by name rather than silently ignored (domain-model.md §2).
func TestNotesOnATypeWithoutTheCapabilityAreRefused(t *testing.T) {
	activity := updatable(t)
	activity.Type = ItemActivity

	_, _, err := activity.Updated(ItemAttributes{Notes: text("Whole")}, activityProfile(), laterOn)
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("notes on an activity answered %v", err)
	}

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("the refusal is not a domain error: %v", err)
	}
	if domainErr.DetailCode != "items.capability_not_supported" {
		t.Errorf("the detail code is %q", domainErr.DetailCode)
	}
	if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/notes" {
		t.Errorf("the refusal does not point at /notes: %v", domainErr.Fields)
	}
}

// Clearing what a type never had is not the same request as setting it. An activity has no notes,
// so asking for none asks for the state it is already in - and NewWorkItem takes the same view of
// an empty `notes` at creation. Two answers to one question is what this prevents.
func TestClearingNotesOnATypeWithoutTheCapabilityIsAllowed(t *testing.T) {
	activity := updatable(t)
	activity.Type = ItemActivity

	after, changes, err := activity.Updated(ItemAttributes{Notes: text("")}, activityProfile(), laterOn)
	if err != nil {
		t.Fatalf("clearing the notes of an activity was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("%d changes were reported for a no-op", len(changes))
	}
	if after.Notes != "" {
		t.Errorf("the notes are %q", after.Notes)
	}
}

// The type's answer comes before the state's. An archived activity told to unarchive itself first
// would still have its notes refused afterwards, which is a round trip for nothing.
func TestTheCapabilityIsAnsweredBeforeTheLifecycle(t *testing.T) {
	archived := updatable(t)
	archived.Type = ItemActivity
	at := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	archived.ArchivedAt = &at

	_, _, err := archived.Updated(ItemAttributes{Notes: text("Whole")}, activityProfile(), laterOn)
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("an archived activity answered %v rather than naming the capability", err)
	}
}

// I-W4: a trashed or archived item is not editable except through restore or unarchive.
func TestALifecycleStateThatMakesTheItemReadOnly(t *testing.T) {
	at := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

	for _, test := range []struct {
		name   string
		mark   func(*WorkItem)
		detail string
	}{
		{name: "archived", mark: func(i *WorkItem) { i.ArchivedAt = &at }, detail: "items.archived"},
		{name: "trashed", mark: func(i *WorkItem) { i.DeletedAt = &at }, detail: "items.trashed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := updatable(t)
			test.mark(&item)

			_, _, err := item.Updated(ItemAttributes{Title: text("Buy oat milk")}, taskProfile(), laterOn)
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("a %s item answered %v", test.name, err)
			}

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != test.detail {
				t.Errorf("the detail code is not %s: %v", test.detail, err)
			}
		})
	}
}

// The title rules are the ones creation applies, because they are the item's rules and not the
// use case's. A second implementation of them here would be a second place to be wrong.
func TestTheTitleIsHeldToTheSameRulesAsAtCreation(t *testing.T) {
	before := updatable(t)

	for _, test := range []struct {
		name   string
		title  string
		detail string
	}{
		{name: "empty", title: "   ", detail: "items.title_empty"},
		{name: "too long", title: strings.Repeat("a", MaxItemTitleLength+1), detail: "items.title_too_long"},
		{name: "more than one line", title: "Buy\nmilk", detail: "items.title_malformed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := before.Updated(ItemAttributes{Title: text(test.title)}, taskProfile(), laterOn)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("a %s title answered %v", test.name, err)
			}

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != test.detail {
				t.Errorf("the detail code is not %s: %v", test.detail, err)
			}
		})
	}
}

func TestAnEmptyUpdateIsRecognisableAsOne(t *testing.T) {
	if !(ItemAttributes{}).IsEmpty() {
		t.Error("an update with no fields does not report itself as empty")
	}
	if (ItemAttributes{Notes: text("")}).IsEmpty() {
		t.Error("clearing the notes reports itself as an empty update")
	}
}
