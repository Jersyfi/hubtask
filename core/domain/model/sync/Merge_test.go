// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Every field of every kind beyond the entry's, judged: the table is §4.2's rows for a reminder,
// a series, a container, a label and a bucket.
func TestEachKindJudgesItsFieldsByItsRow(t *testing.T) {
	const ok, notMergeable, unavailable, unknown = "", "sync.field_not_mergeable", "sync.field_unavailable", "sync.field_unknown"
	cases := []struct {
		kind  MutationKind
		field string
		want  string
	}{
		{ReminderPatch, "offset_spec", ok},
		{ReminderPatch, "channels", ok},
		{ReminderPatch, "recipients", ok},
		{ReminderPatch, "fire_at", notMergeable},
		{ReminderPatch, "state", notMergeable},
		{ReminderPatch, "item_id", notMergeable},
		{ReminderPatch, "colour", unknown},
		{RecurrencePatch, "rrule", ok},
		{RecurrencePatch, "time_zone", ok},
		{RecurrencePatch, "mode", ok},
		{RecurrencePatch, "horizon_days", ok},
		{RecurrencePatch, "ends_at", ok},
		{RecurrencePatch, "max_count", ok},
		{RecurrencePatch, "last_materialized_at", notMergeable},
		{RecurrencePatch, "version", notMergeable},
		{ContainerPatch, "name", ok},
		{ContainerPatch, "description", ok},
		{ContainerPatch, "icon", ok},
		{ContainerPatch, "color_token", ok},
		{ContainerPatch, "structure", notMergeable},
		{ContainerPatch, "private", notMergeable},
		{ContainerPatch, "type", notMergeable},
		{ContainerPatch, "parent_id", unavailable},
		{ContainerPatch, "completion_policy", unavailable},
		{LabelPatch, "name", ok},
		{LabelPatch, "color_token", ok},
		{LabelPatch, "description", ok},
		{LabelPatch, "collection_id", notMergeable},
		{LabelPatch, "deleted_at", notMergeable},
		{BucketPatch, "name", ok},
		{BucketPatch, "wip_limit", ok},
		{BucketPatch, "is_done_bucket", ok},
		{BucketPatch, "color_token", ok},
		{BucketPatch, "order_key", unavailable},
		{BucketPatch, "collection_id", notMergeable},
	}
	for _, c := range cases {
		t.Run(string(c.kind)+"/"+c.field, func(t *testing.T) {
			family, shape, found := FamilyOf(c.kind)
			if !found || shape != ShapeFields {
				t.Fatalf("%s has no field shape", c.kind)
			}
			got := ""
			if err := family.Judge(c.field); err != nil {
				got = shared.AsError(err).DetailCode
			}
			if got != c.want {
				t.Errorf("%s.%s judged %q, want %q", c.kind, c.field, got, c.want)
			}
		})
	}
}

// Each kind beyond the entry's travels by its row's shape: whole on creation and deletion, field
// by field between; the entry's kinds and an act have no family.
func TestEachKindHasTheShapeItsRowNames(t *testing.T) {
	cases := []struct {
		kind   MutationKind
		entity string
		shape  Shape
	}{
		{ReminderCreate, "reminder", ShapeCreation}, {ReminderPatch, "reminder", ShapeFields}, {ReminderDelete, "reminder", ShapeDeletion},
		{RecurrenceCreate, "recurrence_rule", ShapeCreation}, {RecurrencePatch, "recurrence_rule", ShapeFields}, {RecurrenceDelete, "recurrence_rule", ShapeDeletion},
		{ContainerCreate, "container", ShapeCreation}, {ContainerPatch, "container", ShapeFields}, {ContainerDelete, "container", ShapeDeletion},
		{LabelCreate, "label", ShapeCreation}, {LabelPatch, "label", ShapeFields}, {LabelDelete, "label", ShapeDeletion},
		{BucketCreate, "bucket", ShapeCreation}, {BucketPatch, "bucket", ShapeFields}, {BucketDelete, "bucket", ShapeDeletion},
	}
	for _, c := range cases {
		family, shape, found := FamilyOf(c.kind)
		if !found || family.Entity != c.entity || shape != c.shape {
			t.Errorf("%s: %q %q %v, want %q %q", c.kind, family.Entity, shape, found, c.entity, c.shape)
		}
		if !c.kind.Valid() {
			t.Errorf("%s is not a kind the contract names", c.kind)
		}
	}
	for _, kind := range []MutationKind{ItemCreate, ItemPatch, ItemDelete, SetAdd, SetRemove, Move, CommentAdd, TemplateApply} {
		if _, _, found := FamilyOf(kind); found {
			t.Errorf("%s has a family; it merges by its own rule", kind)
		}
	}
	if !TemplateApply.Valid() {
		t.Error("TEMPLATE_APPLY is not a kind the contract names")
	}
}

// Last writer wins per field: the later reading wins, a tie is broken by device, and a field the
// server never stamped loses to any reading.
func TestDecideLetsTheLaterReadingWinPerField(t *testing.T) {
	at := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	read := func(offset time.Duration, device string) shared.HLC {
		r, _ := shared.NewHLC(at.Add(offset), 1, device)
		return r
	}
	cases := []struct {
		name   string
		sent   shared.HLC
		server *shared.HLC
		wins   bool
	}{
		{"never stamped", read(-time.Hour, "dev-a"), nil, true},
		{"later than the server", read(time.Minute, "dev-a"), ptr(read(0, "dev-b")), true},
		{"earlier than the server", read(-time.Minute, "dev-a"), ptr(read(0, "dev-b")), false},
		{"the same reading", read(0, "dev-a"), ptr(read(0, "dev-a")), false},
		{"a tie the device breaks", read(0, "dev-b"), ptr(read(0, "dev-a")), true},
		{"a tie the server breaks", read(0, "dev-a"), ptr(read(0, "dev-b")), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := map[string]shared.HLC{}
			if c.server != nil {
				server["name"] = *c.server
			}
			winners, losers := Decide(map[string]shared.HLC{"name": c.sent}, server)
			_, won := winners["name"]
			_, lost := losers["name"]
			if won != c.wins || lost == c.wins {
				t.Errorf("won %v lost %v, want won %v", won, lost, c.wins)
			}
		})
	}
}

func ptr(r shared.HLC) *shared.HLC { return &r }
