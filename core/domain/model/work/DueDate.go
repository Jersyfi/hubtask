// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// DueDate is when an entry is due: the instant, the all-day flag, and the IANA zone the date is
// local to (domain-model.md §3.4, i18n-l10n.md §4).
//
// One value rather than three fields, like Completion and for the same reason: the three are only
// ever meaningful together. The instant is stored in UTC and the zone beside it, never folded in -
// an all-day due date is a date in that zone, not a midnight that shifts with the viewer, and the
// flag is what tells every reader to render a date rather than a moment.
type DueDate struct {
	At time.Time
	// DateOnly marks an all-day due date: At is read as a date in TimeZone, never as an instant.
	DateOnly bool
	// TimeZone is the IANA name the date is local to, and empty for a due date that is a plain
	// instant. Stored separately because local time matters to the business logic here - a
	// reminder offset and a recurrence both count from it (i18n-l10n.md §4).
	TimeZone string
}

// NewDueDate builds the trio or explains why it cannot be one.
//
// The two absences the backlog names are refused here: a zone without a date and a flag without a
// date each qualify something that is not there, and storing either would be a row whose meaning
// depends on a field it does not have. The nil answer with no error is a due date deliberately
// absent - which is what lets a merge patch express "clear it" through the same door that sets it.
func NewDueDate(at *time.Time, dateOnly bool, zone string) (*DueDate, error) {
	zone = strings.TrimSpace(zone)
	if at == nil || at.IsZero() {
		switch {
		case zone != "":
			return nil, shared.ErrValidation.
				WithDetail("items.due_time_zone_without_date").
				WithFields(shared.FieldError{
					Path: "/due_time_zone", Code: "items.due_time_zone_without_date",
				})
		case dateOnly:
			return nil, shared.ErrValidation.
				WithDetail("items.due_date_only_without_date").
				WithFields(shared.FieldError{
					Path: "/due_date_only", Code: "items.due_date_only_without_date",
				})
		}
		return nil, nil
	}
	if zone != "" {
		// An IANA name is checked by loading it, which is the only check worth making: a name the
		// time package cannot load is a name no calculation can use (identity spells account
		// preferences the same way). Never a fixed offset - an offset cannot represent daylight
		// saving, and this product schedules across it.
		if _, err := time.LoadLocation(zone); err != nil {
			return nil, shared.ErrValidation.
				WithDetail("items.due_time_zone_invalid").
				WithParams(map[string]string{"value": zone}).
				WithFields(shared.FieldError{
					Path: "/due_time_zone", Code: "items.due_time_zone_invalid",
					Params: map[string]string{"value": zone},
				})
		}
	}
	return &DueDate{At: at.UTC(), DateOnly: dateOnly, TimeZone: zone}, nil
}

// Equal reports whether two due dates say the same thing. Instants compare as instants, so the
// same moment written from two zones is one due date.
func (d *DueDate) Equal(other *DueDate) bool {
	if d == nil || other == nil {
		return d == other
	}
	return d.At.Equal(other.At) && d.DateOnly == other.DateOnly && d.TimeZone == other.TimeZone
}

// WithDueDate returns the item carrying the given due date - or none, for nil - and reports which
// of the three fields moved, each as its own change (offline-sync.md §4.2: one change log entry
// per field, so two devices moving the date and the zone independently converge to both).
//
// Only setting asks for the capability, which is the rule Updated applies to the notes and the
// board: clearing a due date from a type that cannot carry one asks for the state it is already
// in. Idempotent like Completed and for the same reason: an unchanged item reports no changes, so
// the caller writes nothing, spends no version and announces nothing.
func (i WorkItem) WithDueDate(
	due *DueDate, profile CapabilityProfile, at time.Time,
) (WorkItem, []FieldChange, error) {
	if due != nil {
		if err := profile.Require(CapabilityDueDate, "/due_at"); err != nil {
			return WorkItem{}, nil, err
		}
	}
	if err := i.EnsureEditable(); err != nil {
		return WorkItem{}, nil, err
	}

	changes := dueDateChanges(i.Due, due)
	if len(changes) == 0 {
		return i, nil, nil
	}
	i.Due = due
	i.UpdatedAt = at
	return i, changes, nil
}

// dueDateChanges diffs the trio field by field. Absent spells as the empty string on either side,
// exactly as clearing an assignee does: a client matches these names and values against what it
// sent, and an omitted entry would read as "not touched" (offline-sync.md §4.2).
func dueDateChanges(from, to *DueDate) []FieldChange {
	var changes []FieldChange
	appendChange := func(field, before, after string) {
		if before != after {
			changes = append(changes, FieldChange{Field: field, From: before, To: after})
		}
	}
	appendChange(FieldDueAt, dueInstant(from), dueInstant(to))
	appendChange(FieldDueDateOnly, dueFlag(from), dueFlag(to))
	appendChange(FieldDueTimeZone, dueZone(from), dueZone(to))
	return changes
}

func dueInstant(d *DueDate) string {
	if d == nil {
		return ""
	}
	return instant(d.At)
}

func dueFlag(d *DueDate) string {
	if d != nil && d.DateOnly {
		return "true"
	}
	return "false"
}

func dueZone(d *DueDate) string {
	if d == nil {
		return ""
	}
	return d.TimeZone
}

// equalInstant compares two optional instants as instants, so the same moment written from two
// zones is one moment.
func equalInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// instantValue spells an optional instant the way a change carries it: `instant`'s spelling, and
// the empty string for none - an omitted entry would read as "not touched" (offline-sync.md §4.2).
func instantValue(t *time.Time) string {
	if t == nil {
		return ""
	}
	return instant(*t)
}
