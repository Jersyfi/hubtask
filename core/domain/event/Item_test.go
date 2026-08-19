// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"errors"
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

// The move event, in the package that builds it. The contract test judges its shape against the schema; this
// says what the builder decides, which the schema cannot: which half of a move is read off the item and which
// half has to be told.
func TestTheMoveEventCarriesBothEndsOfTheMove(t *testing.T) {
	oldTask := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	newTask := shared.MustParseID("0192f000-0000-7000-8000-000000000014")
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000012")
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	fromPath := "/" + oldTask.String() + "/" + id.String() + "/"

	item := work.WorkItem{
		ID:           id,
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemWorkPackage,
		ParentID:     newTask,
		Path:         "/" + newTask.String() + "/" + id.String() + "/",
		Depth:        2,
		Title:        "Order the cable",
		OrderKey:     "a1",
		CreatedBy:    shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:    at,
		UpdatedAt:    at.Add(time.Hour),
		Version:      2,
	}

	envelope, err := NewItemMoved(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), item,
		Movement{FromParentID: oldTask, FromPath: fromPath, FromCollectionID: item.CollectionID},
		Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.UpdatedAt, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	if envelope.Type != ItemMoved {
		t.Errorf("the type is %s", envelope.Type)
	}
	if envelope.Payload["from_parent_id"] != oldTask.String() {
		t.Errorf("from_parent_id is %v", envelope.Payload["from_parent_id"])
	}
	if envelope.Payload["to_parent_id"] != newTask.String() {
		t.Errorf("to_parent_id is %v", envelope.Payload["to_parent_id"])
	}
	// The prefix swap: together these two are what a client rewrites its own subtree with, which is why a moved
	// subtree needs no event per descendant.
	if envelope.Payload["from_path"] != fromPath {
		t.Errorf("from_path is %v, want %q", envelope.Payload["from_path"], fromPath)
	}
	if envelope.Payload["path"] != item.Path {
		t.Errorf("path is %v, want %q", envelope.Payload["path"], item.Path)
	}
	// The collection did not change, so the field says so rather than repeating the current one.
	if envelope.Payload["from_collection_id"] != nil {
		t.Errorf("from_collection_id is %v on a move within one collection", envelope.Payload["from_collection_id"])
	}
	// Reserved and null until buckets exist, and present so a kanban consumer can be written now.
	for _, field := range []string{"from_bucket_id", "to_bucket_id"} {
		if value, present := envelope.Payload[field]; !present || value != nil {
			t.Errorf("%s is %v (present=%v), want a null that is there", field, value, present)
		}
	}
}

// A move out of a collection sets the field a device subscribed to one hub reads to know the item has left its
// scope.
func TestAMoveBetweenCollectionsSaysWhereItCameFrom(t *testing.T) {
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000016")
	source := shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	destination := shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	item := work.WorkItem{
		ID: id, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: destination, Type: work.ItemTask, Path: work.RootPath(id), Depth: 1,
		Title: "Weekly shop", OrderKey: "a0",
		CreatedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt: at, UpdatedAt: at, Version: 2,
	}

	envelope, err := NewItemMoved(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000f2"), item,
		Movement{FromPath: work.RootPath(id), FromCollectionID: source},
		Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, at, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	if envelope.Payload["from_collection_id"] != source.String() {
		t.Errorf("from_collection_id is %v, want %s", envelope.Payload["from_collection_id"], source)
	}
	if envelope.Payload["collection_id"] != destination.String() {
		t.Errorf("collection_id is %v, want %s", envelope.Payload["collection_id"], destination)
	}
	// A task sits directly in a collection, so both parent fields are null and the schema has to accept it.
	for _, field := range []string{"from_parent_id", "to_parent_id"} {
		if value, present := envelope.Payload[field]; !present || value != nil {
			t.Errorf("%s is %v (present=%v), want a null that is there", field, value, present)
		}
	}
}

// The update event is the one that is not a snapshot: it names the fields that moved and carries the
// content of no other one. A rename must not put somebody's notes into every subscriber's log.
func TestTheUpdateEventCarriesAChangeSetRatherThanASnapshot(t *testing.T) {
	item := task()
	item.Title = "Buy oat milk"
	item.Notes = "Semi-skimmed, two litres"
	item.Version = 2

	envelope, err := NewItemUpdated(eventID, item,
		[]work.FieldChange{{Field: work.FieldTitle, From: "Buy milk", To: item.Title}},
		Actor{Kind: shared.ActorUser, ID: eventAuthor}, occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	if envelope.Type != ItemUpdated {
		t.Errorf("type = %s", envelope.Type)
	}
	if envelope.Subject != ItemSubject(item.ID) {
		t.Errorf("subject = %s", envelope.Subject)
	}

	changeSet, present := envelope.Payload["change_set"].(map[string]any)
	if !present {
		t.Fatalf("there is no change set: %v", envelope.Payload)
	}
	title, _ := changeSet[work.FieldTitle].(map[string]any)
	if title["from"] != "Buy milk" || title["to"] != "Buy oat milk" {
		t.Errorf("the change set says %v", changeSet[work.FieldTitle])
	}

	// The notes did not move, so neither they nor the title appear outside the change set.
	if _, present := changeSet[work.FieldNotes]; present {
		t.Error("the change set names a field that did not move")
	}
	for _, field := range []string{"title", "notes", "path", "order_key", "completion"} {
		if _, present := envelope.Payload[field]; present {
			t.Errorf("the event carries %q outside the change set", field)
		}
	}

	// Enough to place it, and no more: which collection, and which version the change produced.
	if envelope.Payload["collection_id"] != eventCollection.String() {
		t.Errorf("collection_id = %v", envelope.Payload["collection_id"])
	}
	if envelope.Payload["version"] != 2 {
		t.Errorf("version = %v", envelope.Payload["version"])
	}
}

// An event announcing that nothing changed would be a lie the writer cannot have meant: it does not
// write when nothing moved, so the two disagreeing is a defect rather than something a client sent.
func TestAnUpdateEventWithNothingInItIsRefused(t *testing.T) {
	_, err := NewItemUpdated(eventID, task(), nil,
		Actor{Kind: shared.ActorUser, ID: eventAuthor}, occurred, Cause{})
	if !errors.Is(err, shared.ErrInternal) {
		t.Errorf("an empty change set answered %v", err)
	}
}
