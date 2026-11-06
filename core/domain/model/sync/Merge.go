// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// How the kinds beyond the entry's merge (offline-sync.md §4.2): a reminder, a recurrence rule, a
// container, a label and a bucket travel whole on creation and deletion and field by field
// between, each field last writer wins under its own clock; applying a template is an act, not a
// state, and merges nothing. The tables say which fields a device may send and which the server
// decides; everything else is a field this build does not know.

// Shape is how one kind travels.
type Shape string

const (
	// ShapeCreation carries the whole object; the server's use case judges it whole.
	ShapeCreation Shape = "CREATION"
	// ShapeFields carries changed fields, each decided on its own against the server's clock.
	ShapeFields Shape = "FIELDS"
	// ShapeDeletion carries nothing but the identity.
	ShapeDeletion Shape = "DELETION"
	// ShapeAct carries an instruction the server performs; its results arrive as creations.
	ShapeAct Shape = "ACT"
)

// Family is what one kind of object beyond the entry merges by: the entity its change log
// entries and clocks are filed under, the fields a device may write, the fields only the server
// writes, and the fields a later build may take.
type Family struct {
	Entity      string
	Writable    map[string]bool
	ServerOwned map[string]bool
	Later       map[string]bool
}

var (
	// The fields every object carries and no device writes.
	stamps = []string{"id", "version", "created_at", "updated_at", "created_by", "tenant_id", "deleted_at"}

	reminderFamily = family("reminder",
		[]string{"offset_spec", "channels", "recipients"},
		[]string{"item_id", "fire_at", "state"}, nil)
	recurrenceFamily = family("recurrence_rule",
		[]string{"rrule", "time_zone", "mode", "horizon_days", "ends_at", "max_count"},
		[]string{"item_id", "last_materialized_at"}, nil)
	// A container's structure is resolved by the server and never pushed, and its privacy is a
	// permission (§1's right column); its policies and its place in the tree wait for a later
	// build and are kept by the device until then.
	containerFamily = family("container",
		[]string{"name", "description", "icon", "color_token"},
		[]string{"type", "path", "depth", "structure", "private", "archived_at"},
		[]string{"parent_id", "order_key", "completion_policy", "auto_assign"})
	labelFamily = family("label",
		[]string{"name", "color_token", "description"},
		[]string{"collection_id"}, nil)
	bucketFamily = family("bucket",
		[]string{"name", "color_token", "wip_limit", "is_done_bucket"},
		[]string{"collection_id"},
		[]string{"order_key"})
)

func family(entity string, writable, owned, later []string) Family {
	f := Family{Entity: entity, Writable: map[string]bool{}, ServerOwned: map[string]bool{}, Later: map[string]bool{}}
	for _, name := range writable {
		f.Writable[name] = true
	}
	for _, name := range append(append([]string{}, stamps...), owned...) {
		f.ServerOwned[name] = true
	}
	for _, name := range later {
		f.Later[name] = true
	}
	return f
}

// FamilyOf answers the family and the shape of a kind beyond the entry's. False for the entry's
// own kinds, which merge by their own rules (ITEM_PATCH, the OR-set, MOVE), and for an act.
func FamilyOf(kind MutationKind) (Family, Shape, bool) {
	switch kind {
	case ReminderCreate:
		return reminderFamily, ShapeCreation, true
	case ReminderPatch:
		return reminderFamily, ShapeFields, true
	case ReminderDelete:
		return reminderFamily, ShapeDeletion, true
	case RecurrenceCreate:
		return recurrenceFamily, ShapeCreation, true
	case RecurrencePatch:
		return recurrenceFamily, ShapeFields, true
	case RecurrenceDelete:
		return recurrenceFamily, ShapeDeletion, true
	case ContainerCreate:
		return containerFamily, ShapeCreation, true
	case ContainerPatch:
		return containerFamily, ShapeFields, true
	case ContainerDelete:
		return containerFamily, ShapeDeletion, true
	case LabelCreate:
		return labelFamily, ShapeCreation, true
	case LabelPatch:
		return labelFamily, ShapeFields, true
	case LabelDelete:
		return labelFamily, ShapeDeletion, true
	case BucketCreate:
		return bucketFamily, ShapeCreation, true
	case BucketPatch:
		return bucketFamily, ShapeFields, true
	case BucketDelete:
		return bucketFamily, ShapeDeletion, true
	}
	return Family{}, "", false
}

// Judge answers whether a device may send the field: nil, `sync.field_not_mergeable` for one the
// server decides, `sync.field_unavailable` for one a later build takes (unavailable, so the device
// keeps it), and `sync.field_unknown` otherwise.
func (f Family) Judge(field string) error {
	switch {
	case f.Writable[field]:
		return nil
	case f.Later[field]:
		return shared.ErrUnavailable.
			WithDetail("sync.field_unavailable").
			WithParams(map[string]string{"field": field})
	case f.ServerOwned[field]:
		return shared.ErrValidation.
			WithDetail("sync.field_not_mergeable").
			WithParams(map[string]string{"field": field}).
			WithFields(shared.FieldError{Path: "/fields/" + field, Code: "sync.field_not_mergeable"})
	}
	return shared.ErrValidation.
		WithDetail("sync.field_unknown").
		WithParams(map[string]string{"field": field}).
		WithFields(shared.FieldError{Path: "/fields/" + field, Code: "sync.field_unknown"})
}

// Decide splits the readings a device sent per field into the fields it wins and the ones it
// loses against the server's clock per field: the later reading wins, a tie broken by device as
// HLC.After breaks it, and a field the server never stamped loses to any reading.
func Decide(sent, server map[string]shared.HLC) (winners, losers map[string]shared.HLC) {
	winners = map[string]shared.HLC{}
	losers = map[string]shared.HLC{}
	for field, reading := range sent {
		if stored, stamped := server[field]; stamped && !reading.After(stored) {
			losers[field] = reading
			continue
		}
		winners[field] = reading
	}
	return winners, losers
}

// RecurrenceFamily is the series' row, for a creation that merges with a series already there.
func RecurrenceFamily() Family { return recurrenceFamily }
