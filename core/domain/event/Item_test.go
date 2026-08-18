// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	eventTenant     = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	eventCollection = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	eventAuthor     = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	eventTask       = shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	eventPackage    = shared.MustParseID("0192f000-0000-7000-8000-000000000012")
	eventID         = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	occurred        = time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
)

func task() work.WorkItem {
	return work.WorkItem{
		ID: eventTask, TenantID: eventTenant, CollectionID: eventCollection,
		Type: work.ItemTask, Path: work.RootPath(eventTask), Depth: 1, Title: "Buy milk",
		OrderKey: "a0", CreatedBy: eventAuthor, CreatedAt: occurred, UpdatedAt: occurred, Version: 1,
	}
}

func announce(t *testing.T, item work.WorkItem) Envelope {
	t.Helper()

	envelope, err := NewItemCreated(eventID, item,
		Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

// The payload is a snapshot in the API's words, so that a webhook payload and a REST response
// describe the same object alike.
func TestTheItemEventCarriesASnapshotInTheApiFieldNames(t *testing.T) {
	envelope := announce(t, task())

	if envelope.Type != ItemCreated {
		t.Errorf("type = %s", envelope.Type)
	}
	if envelope.Subject != "item/"+eventTask.String() {
		t.Errorf("subject = %s", envelope.Subject)
	}
	if envelope.TenantID != eventTenant {
		t.Errorf("tenant = %s", envelope.TenantID)
	}

	payload := envelope.Payload
	for field, want := range map[string]any{
		"id":            eventTask.String(),
		"type":          string(work.ItemTask),
		"collection_id": eventCollection.String(),
		"path":          work.RootPath(eventTask),
		"depth":         1,
		"title":         "Buy milk",
		"order_key":     "a0",
		"created_by":    eventAuthor.String(),
		"version":       1,
	} {
		if payload[field] != want {
			t.Errorf("%s = %v, want %v", field, payload[field], want)
		}
	}
	if payload["created_at"] != occurred.UTC() {
		t.Errorf("created_at = %v", payload["created_at"])
	}
}

// A task sits directly under the collection, and the parent is sent as null rather than left out:
// a consumer places the item by reading the field, and inferring it from the type is what breaks
// the day a level is configured above the task.
func TestATasksParentIsSentAsNullRatherThanOmitted(t *testing.T) {
	payload := announce(t, task()).Payload

	parent, present := payload["parent_id"]
	if !present {
		t.Fatal("parent_id is missing rather than null")
	}
	if parent != nil {
		t.Errorf("parent_id = %v, want null", parent)
	}
}

func TestAChildCarriesItsParent(t *testing.T) {
	item := task()
	item.ID, item.Type, item.ParentID, item.Depth = eventPackage, work.ItemWorkPackage, eventTask, 2
	item.Path = task().ChildPath(eventPackage)

	payload := announce(t, item).Payload
	if payload["parent_id"] != eventTask.String() {
		t.Errorf("parent_id = %v", payload["parent_id"])
	}
	if payload["depth"] != 2 {
		t.Errorf("depth = %v", payload["depth"])
	}
}

// An unset note is left out; completion is always there. The asymmetry is deliberate - an absent
// note is unambiguous, whereas a rule reading completion.is_completed should need no special case
// for the one event where the field would be missing.
func TestAnUnsetNoteIsOmittedAndCompletionNeverIs(t *testing.T) {
	payload := announce(t, task()).Payload

	if _, present := payload["notes"]; present {
		t.Error("an unset note was sent anyway")
	}
	completion, present := payload["completion"].(map[string]any)
	if !present {
		t.Fatalf("completion = %v, want it always present", payload["completion"])
	}
	if completion["is_completed"] != false {
		t.Errorf("is_completed = %v, want false", completion["is_completed"])
	}
	// Present as null rather than absent, for the same reason parent_id is: a consumer reads the
	// field rather than inferring it.
	for _, field := range []string{"completed_at", "completed_by"} {
		value, present := completion[field]
		if !present || value != nil {
			t.Errorf("%s = %v, want null", field, value)
		}
	}

	withNotes := task()
	withNotes.Notes = "Semi-skimmed"
	if announce(t, withNotes).Payload["notes"] != "Semi-skimmed" {
		t.Error("a note that was set did not travel")
	}
}

// A completed item carries who closed it and when. Nothing creates one, but the payload is built
// from the item rather than from the occasion, so the shape has to be right wherever it is used
// next (B-07).
func TestACompletedItemCarriesWhoClosedItAndWhen(t *testing.T) {
	closed := occurred.Add(time.Hour)
	item := task()
	item.Completion = work.Completion{
		IsCompleted: true, CompletedAt: &closed, CompletedBy: eventAuthor,
	}

	completion, _ := announce(t, item).Payload["completion"].(map[string]any)
	if completion["is_completed"] != true {
		t.Errorf("is_completed = %v", completion["is_completed"])
	}
	if completion["completed_at"] != closed.UTC() {
		t.Errorf("completed_at = %v, want %v", completion["completed_at"], closed.UTC())
	}
	if completion["completed_by"] != eventAuthor.String() {
		t.Errorf("completed_by = %v", completion["completed_by"])
	}
}

// The subject is what a consumer filters on without parsing the payload, so it is kept next to
// the event that writes it.
func TestTheSubjectNamesTheItem(t *testing.T) {
	if got := ItemSubject(eventTask); got != "item/"+eventTask.String() {
		t.Errorf("subject = %q", got)
	}
}

func TestTheTypeRendersAsItsName(t *testing.T) {
	if ItemCreated.String() != "de.hubtask.work.item.created.v1" {
		t.Errorf("type = %q", ItemCreated.String())
	}
	if !ItemCreated.Valid() {
		t.Error("the item event is not in the closed set")
	}
}
