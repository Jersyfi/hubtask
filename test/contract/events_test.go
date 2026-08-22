// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
)

const eventSchemaDir = "../../api/events"

// loadEventSchema reads one event schema. JSON is valid YAML, so the validator that judges REST
// responses judges events too - one implementation of the subset, not two.
func loadEventSchema(t *testing.T, eventType event.Type) *specification {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(eventSchemaDir, string(eventType)+".json"))
	if err != nil {
		t.Fatalf("no schema for %s: %v", eventType, err)
	}

	var root schema
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("the schema of %s is not readable: %v", eventType, err)
	}

	spec := &specification{}
	spec.Components.Schemas = map[string]*schema{"root": &root}
	return spec
}

func containerCreated(t *testing.T) event.Envelope {
	t.Helper()

	container := work.Container{
		ID:        shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		Type:      work.ContainerCollection,
		ParentID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000c"),
		Name:      "Shopping",
		OrderKey:  "a0",
		CreatedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt: time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		Version:   1,
	}

	envelope, err := event.NewContainerCreated(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"), container,
		event.Actor{Kind: shared.ActorUser, ID: container.CreatedBy},
		container.CreatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

// The acceptance criterion of A-07: what the system publishes matches the schema it publishes it
// under. The schema is the contract a subscriber writes against (ADR-0007), so it is judged
// against a real event rather than against an example somebody kept up to date.
func TestTheContainerCreatedEventMatchesItsSchema(t *testing.T) {
	spec := loadEventSchema(t, event.ContainerCreated)

	body, err := json.Marshal(eventbus.ToCloudEvent(containerCreated(t), "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}

	problems, err := spec.validateAgainst("root", body)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// A hub has no parent, and the schema has to accept that as null rather than as an omission - a
// subscriber that reads `parent_id` to decide which level it is looking at needs the field there.
func TestTheEventOfAHubCarriesANullParent(t *testing.T) {
	spec := loadEventSchema(t, event.ContainerCreated)

	container := work.Container{
		ID:        shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		Type:      work.ContainerHub,
		Name:      "Private",
		OrderKey:  "a0",
		CreatedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt: time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		Version:   1,
	}
	envelope, err := event.NewContainerCreated(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e2"), container,
		event.Actor{Kind: shared.ActorUser, ID: container.CreatedBy}, container.CreatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}
	if problems, _ := spec.validateAgainst("root", body); len(problems) > 0 {
		t.Errorf("the event of a hub does not match its schema: %v", problems)
	}

	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatalf("re-reading the event: %v", err)
	}
	data, _ := rendered["data"].(map[string]any)
	if parent, present := data["parent_id"]; !present || parent != nil {
		t.Errorf("parent_id is %v rather than null", data["parent_id"])
	}
}

// itemCreated builds a work package: the middle level, so the event carries both a parent item and
// a collection, which is the case the two identifiers exist for.
func itemCreated(t *testing.T) event.Envelope {
	t.Helper()

	task := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	item := work.WorkItem{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-000000000012"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemWorkPackage,
		ParentID:     task,
		Path:         "/" + task.String() + "/0192f000-0000-7000-8000-000000000012/",
		Depth:        2,
		Title:        "Order the cable",
		Notes:        "Three metres, not two.",
		OrderKey:     "a0",
		CreatedBy:    shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:    time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Version:      1,
	}

	envelope, err := event.NewItemCreated(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e3"), item,
		event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.CreatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

// The same criterion for the second aggregate: what the system publishes matches the schema it
// publishes it under.
func TestTheItemCreatedEventMatchesItsSchema(t *testing.T) {
	spec := loadEventSchema(t, event.ItemCreated)

	body, err := json.Marshal(eventbus.ToCloudEvent(itemCreated(t), "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}

	problems, err := spec.validateAgainst("root", body)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// A task sits directly under the collection, and the schema has to accept that as null - a
// consumer that places items in a tree reads the field rather than inferring it from the type,
// because inferring it breaks the day a level is configured above the task.
func TestTheEventOfATaskCarriesANullParentAndNoNotes(t *testing.T) {
	spec := loadEventSchema(t, event.ItemCreated)

	id := shared.MustParseID("0192f000-0000-7000-8000-000000000013")
	item := work.WorkItem{
		ID:           id,
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemTask,
		Path:         work.RootPath(id),
		Depth:        1,
		Title:        "Buy milk",
		OrderKey:     "a0",
		CreatedBy:    shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:    time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Version:      1,
	}
	envelope, err := event.NewItemCreated(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e4"), item,
		event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.CreatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}
	if problems, _ := spec.validateAgainst("root", body); len(problems) > 0 {
		t.Errorf("the event of a task does not match its schema: %v", problems)
	}

	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatalf("re-reading the event: %v", err)
	}
	data, _ := rendered["data"].(map[string]any)
	if parent, present := data["parent_id"]; !present || parent != nil {
		t.Errorf("parent_id is %v rather than null", data["parent_id"])
	}
	// An unset note is left out rather than sent as null, the way an unset description is: a
	// consumer tolerates the absence, and the bytes are not spent saying nothing.
	if _, present := data["notes"]; present {
		t.Error("an unset note was sent anyway")
	}
	// Completion is the opposite case, and deliberately so: always present, so that a rule
	// reading it needs no special case for the one event where it would be missing.
	completion, present := data["completion"].(map[string]any)
	if !present || completion["is_completed"] != false {
		t.Errorf("completion = %v, want an open one", data["completion"])
	}
}

// The same criterion for the one item event that carries a change set rather than a snapshot.
func TestTheItemUpdatedEventMatchesItsSchema(t *testing.T) {
	spec := loadEventSchema(t, event.ItemUpdated)

	body, err := json.Marshal(eventbus.ToCloudEvent(itemUpdated(t), "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}

	problems, err := spec.validateAgainst("root", body)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// The acceptance criterion of B-05: the event names the fields that changed and carries the content
// of no other field. A rename must not put somebody's notes into every subscriber's log.
func TestTheItemUpdatedEventCarriesOnlyWhatChanged(t *testing.T) {
	body, err := json.Marshal(eventbus.ToCloudEvent(itemUpdated(t), "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}

	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatalf("re-reading the event: %v", err)
	}
	data, _ := rendered["data"].(map[string]any)

	changeSet, present := data["change_set"].(map[string]any)
	if !present {
		t.Fatalf("there is no change set: %v", data)
	}
	if len(changeSet) != 1 {
		t.Fatalf("the change set names %d fields rather than the one that changed: %v", len(changeSet), changeSet)
	}
	title, _ := changeSet["title"].(map[string]any)
	if title["from"] != "Order the cable" || title["to"] != "Order the longer cable" {
		t.Errorf("the change set says %v", changeSet["title"])
	}

	// The notes did not change, so neither the field nor its content is anywhere in the event.
	if _, present := changeSet["notes"]; present {
		t.Error("the change set names a field that did not change")
	}
	for field := range data {
		if field == "notes" || field == "title" {
			t.Errorf("the event carries %q outside the change set", field)
		}
	}
}

// itemUpdated renames a work package whose notes stay as they are: the case the change set exists
// for, because the item has a second content field that must not travel.
func itemUpdated(t *testing.T) event.Envelope {
	t.Helper()

	task := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	item := work.WorkItem{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-000000000012"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemWorkPackage,
		ParentID:     task,
		Path:         "/" + task.String() + "/0192f000-0000-7000-8000-000000000012/",
		Depth:        2,
		Title:        "Order the longer cable",
		Notes:        "Three metres, not two.",
		OrderKey:     "a0",
		CreatedBy:    shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:    time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 8, 18, 11, 0, 0, 0, time.UTC),
		Version:      2,
	}

	envelope, err := event.NewItemUpdated(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e5"), item,
		[]work.FieldChange{{Field: work.FieldTitle, From: "Order the cable", To: item.Title}},
		event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.UpdatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

// Every event the domain can emit has a schema, and every schema belongs to an event. A schema
// without an event is a promise to a subscriber that nothing keeps; an event without a schema is
// a contract nobody can write against.
func TestEveryEventTypeHasASchemaAndTheOtherWayRound(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(eventSchemaDir, "*.json"))
	if err != nil {
		t.Fatalf("reading %s: %v", eventSchemaDir, err)
	}

	declared := map[string]bool{}
	for _, file := range files {
		name := filepath.Base(file)
		declared[name[:len(name)-len(".json")]] = true
	}

	for _, eventType := range event.Types() {
		if !declared[string(eventType)] {
			t.Errorf("%s has no schema under api/events/", eventType)
		}
		delete(declared, string(eventType))
	}
	for orphan := range declared {
		t.Errorf("api/events/%s.json describes an event the domain cannot emit", orphan)
	}
}

// The extension attributes are what a broker and a webhook subscription filter on, so they are
// part of the contract as much as the payload is.
func TestTheExtensionAttributesCarryTheChain(t *testing.T) {
	envelope := containerCreated(t)

	rendered := eventbus.ToCloudEvent(envelope, "urn:hubtask:test")

	if rendered["tenantid"] != envelope.TenantID.String() {
		t.Errorf("the tenant is missing: %v", rendered["tenantid"])
	}
	if rendered["correlationid"] != envelope.ID.String() {
		t.Errorf("a root event is not its own correlation: %v", rendered["correlationid"])
	}
	// Absent rather than empty: "no causing event" and "an event with an empty identifier" are
	// different statements, and only one of them can happen.
	if _, present := rendered["causationid"]; present {
		t.Errorf("a root event names a causing event: %v", rendered["causationid"])
	}
	if _, present := rendered["onbehalfof"]; present {
		t.Errorf("a person acting for themselves has a principal: %v", rendered["onbehalfof"])
	}
}

// completedItem is a work package that has been completed: the middle level, so both identifiers the
// payload carries are exercised, and a completion that is answered in full.
func completedItem(t *testing.T) work.WorkItem {
	t.Helper()

	task := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000012")
	completedAt := time.Date(2026, 8, 18, 11, 0, 0, 0, time.UTC)

	return work.WorkItem{
		ID:           id,
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemWorkPackage,
		ParentID:     task,
		Path:         "/" + task.String() + "/" + id.String() + "/",
		Depth:        2,
		Title:        "Order the cable",
		Completion: work.Completion{
			IsCompleted: true,
			CompletedAt: &completedAt,
			CompletedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		},
		OrderKey:  "a0",
		CreatedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		UpdatedAt: completedAt,
		Version:   2,
	}
}

// What the system publishes matches the schema it publishes it under - for the two directions of
// completion as much as for the create (B-07).
func TestTheCompletionEventsMatchTheirSchemas(t *testing.T) {
	done := completedItem(t)
	reopened := done
	reopened.Completion = work.Completion{}
	reopened.Version = 3

	cases := map[string]struct {
		eventType event.Type
		build     func() (event.Envelope, error)
	}{
		"completed": {event.ItemCompleted, func() (event.Envelope, error) {
			return event.NewItemCompleted(
				shared.MustParseID("0192f000-0000-7000-8000-0000000000e4"), done,
				event.Actor{Kind: shared.ActorUser, ID: done.Completion.CompletedBy},
				done.UpdatedAt, event.Cause{})
		}},
		"reopened": {event.ItemReopened, func() (event.Envelope, error) {
			return event.NewItemReopened(
				shared.MustParseID("0192f000-0000-7000-8000-0000000000e5"), reopened,
				event.Actor{Kind: shared.ActorUser, ID: reopened.CreatedBy},
				reopened.UpdatedAt, event.Cause{})
		}},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			envelope, err := c.build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := loadEventSchema(t, c.eventType).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// The schemas pin `is_completed` to one value each, which is what stops the two events being told apart
// by a payload field rather than by their type. An event carrying the wrong state has to fail its schema.
func TestACompletionEventCannotCarryTheOppositeState(t *testing.T) {
	// Rendered by hand: the constructors refuse this, which is the point - the schema is the second
	// line of defence, for anything that ever builds an event without going through them.
	done := completedItem(t)
	envelope, err := event.NewItemCompleted(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e6"), done,
		event.Actor{Kind: shared.ActorUser, ID: done.Completion.CompletedBy}, done.UpdatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	rendered := eventbus.ToCloudEvent(envelope, "urn:hubtask:test")
	body, err := json.Marshal(rendered)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	data, _ := raw["data"].(map[string]any)
	completion, _ := data["completion"].(map[string]any)
	completion["is_completed"] = false

	tampered, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-rendering: %v", err)
	}
	problems, err := loadEventSchema(t, event.ItemCompleted).validateAgainst("root", tampered)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	if len(problems) == 0 {
		t.Error("a completed event claiming the item is open passed its own schema")
	}
}

// The constructors refuse an event that would say the opposite of its own name. A defect in whatever
// built it rather than something a client did, so it is an internal error.
func TestACompletionEventRefusesAnInconsistentItem(t *testing.T) {
	done := completedItem(t)
	stillOpen := done
	stillOpen.Completion = work.Completion{}

	actor := event.Actor{Kind: shared.ActorUser, ID: done.CreatedBy}
	id := shared.MustParseID("0192f000-0000-7000-8000-0000000000e7")

	if _, err := event.NewItemCompleted(id, stillOpen, actor, done.UpdatedAt, event.Cause{}); err == nil {
		t.Error("a completed event was built for an open item")
	}
	if _, err := event.NewItemReopened(id, done, actor, done.UpdatedAt, event.Cause{}); err == nil {
		t.Error("a reopened event was built for a completed item")
	}
}

// The move event, judged by the schema it is published under (B-08).
func TestTheItemMovedEventMatchesItsSchema(t *testing.T) {
	spec := loadEventSchema(t, event.ItemMoved)

	oldTask := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	newTask := shared.MustParseID("0192f000-0000-7000-8000-000000000014")
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000012")
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

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
	from := event.Movement{
		FromParentID: oldTask,
		FromPath:     "/" + oldTask.String() + "/" + id.String() + "/",
	}

	envelope, err := event.NewItemMoved(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e8"), item, from,
		event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.UpdatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering the event: %v", err)
	}
	problems, err := spec.validateAgainst("root", body)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}

	// The prefix swap is the whole reason a moved subtree needs no event per descendant: from_path and path
	// together are what a client rewrites its own copy with.
	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatalf("re-reading the event: %v", err)
	}
	data, _ := rendered["data"].(map[string]any)
	if data["from_path"] != from.FromPath {
		t.Errorf("from_path is %v, want %q", data["from_path"], from.FromPath)
	}
	if data["path"] != item.Path {
		t.Errorf("path is %v, want %q", data["path"], item.Path)
	}
	// Not a collection change, so the field says so rather than repeating the collection.
	if data["from_collection_id"] != nil {
		t.Errorf("from_collection_id is %v on a move within one collection", data["from_collection_id"])
	}
}

// A reorder is the same event with the same parent on both sides, which is what lets a rule that only cares
// about reparenting tell the two apart without a second event type.
func TestAReorderIsAMoveWithTheSameParent(t *testing.T) {
	spec := loadEventSchema(t, event.ItemMoved)

	task := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000012")
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	path := "/" + task.String() + "/" + id.String() + "/"

	item := work.WorkItem{
		ID:           id,
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemWorkPackage,
		ParentID:     task,
		Path:         path,
		Depth:        2,
		Title:        "Order the cable",
		OrderKey:     "a0V",
		CreatedBy:    shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:    at,
		UpdatedAt:    at.Add(time.Hour),
		Version:      3,
	}

	envelope, err := event.NewItemMoved(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e9"), item,
		event.Movement{FromParentID: task, FromPath: path},
		event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.UpdatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	problems, err := spec.validateAgainst("root", body)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}

	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	data, _ := rendered["data"].(map[string]any)
	if data["from_parent_id"] != data["to_parent_id"] {
		t.Errorf("a reorder announced a reparenting: %v -> %v", data["from_parent_id"], data["to_parent_id"])
	}
	if data["order_key"] != item.OrderKey {
		t.Errorf("order_key is %v, want %q", data["order_key"], item.OrderKey)
	}
}

// A move to the top level of a collection carries a null parent, and the schema has to accept it - a consumer
// placing items in a tree reads the field rather than inferring it from the type.
func TestAMoveToTheTopLevelCarriesANullParent(t *testing.T) {
	spec := loadEventSchema(t, event.ItemMoved)

	oldTask := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000015")
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	item := work.WorkItem{
		ID:           id,
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemTask,
		Path:         work.RootPath(id),
		Depth:        1,
		Title:        "Weekly shop",
		OrderKey:     "a0",
		CreatedBy:    shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:    at,
		UpdatedAt:    at.Add(time.Hour),
		Version:      2,
	}

	envelope, err := event.NewItemMoved(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000ea"), item,
		event.Movement{
			FromParentID: oldTask,
			FromPath:     "/" + oldTask.String() + "/" + id.String() + "/",
		},
		event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}, item.UpdatedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	problems, err := spec.validateAgainst("root", body)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}

	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	data, _ := rendered["data"].(map[string]any)
	if parent, present := data["to_parent_id"]; !present || parent != nil {
		t.Errorf("to_parent_id is %v rather than null", data["to_parent_id"])
	}
}

// The container lifecycle's five events, each judged against its own schema. A table rather than
// five tests: they share a subject and an envelope, and what differs between them is the payload -
// which is exactly what the schemas describe and this loop checks.
func TestTheContainerLifecycleEventsMatchTheirSchemas(t *testing.T) {
	hub := shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-00000000001c")
	actor := shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	at := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	archivedAt := at.Add(-time.Hour)

	collection := work.Container{
		ID:               shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		TenantID:         shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		Type:             work.ContainerCollection,
		ParentID:         hub,
		Name:             "Shopping",
		OrderKey:         "a0",
		CompletionPolicy: work.CompletionRollup,
		CreatedBy:        actor,
		CreatedAt:        at.Add(-24 * time.Hour),
		UpdatedAt:        at,
		Version:          4,
	}
	archived := collection
	archived.ArchivedAt = &archivedAt

	renamed := []work.FieldChange{{Field: work.FieldName, From: "Shopping", To: "Groceries"}}
	reconfigured := []work.FieldChange{{Field: work.FieldCompletionPolicy, From: "MANUAL", To: "ROLLUP"}}

	cases := []struct {
		name  string
		typed event.Type
		build func(shared.ID) (event.Envelope, error)
	}{
		{
			name: "renamed", typed: event.ContainerRenamed,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerRenamed(id, collection, renamed,
					event.Actor{Kind: shared.ActorUser, ID: actor}, at, event.Cause{})
			},
		},
		{
			name: "policies updated", typed: event.ContainerPoliciesUpdated,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerPoliciesUpdated(id, collection, reconfigured,
					event.Actor{Kind: shared.ActorUser, ID: actor}, at, event.Cause{})
			},
		},
		{
			name: "moved into another hub", typed: event.ContainerMoved,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerMoved(id, collection, elsewhere,
					event.Actor{Kind: shared.ActorUser, ID: actor}, at, event.Cause{})
			},
		},
		{
			name: "archived", typed: event.ContainerArchived,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerArchived(id, archived,
					event.Actor{Kind: shared.ActorUser, ID: actor}, at, event.Cause{})
			},
		},
		{
			name: "unarchived", typed: event.ContainerUnarchived,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerUnarchived(id, collection,
					event.Actor{Kind: shared.ActorUser, ID: actor}, at, event.Cause{})
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := c.build(shared.MustParseID("0192f000-0000-7000-8000-0000000000e7"))
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}
			if envelope.Type != c.typed {
				t.Fatalf("event type %s, want %s", envelope.Type, c.typed)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}
			problems, err := loadEventSchema(t, c.typed).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// An archived hub makes its collections read-only without archiving them, so `archived_at` alone
// cannot tell a subscriber whether a collection may be written to. `effective_archived` is what
// answers that, and it has to be in the payload rather than derivable from it.
func TestAContainerEventReportsTheInheritedArchive(t *testing.T) {
	archivedAt := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	collection := work.Container{
		ID:               shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		TenantID:         shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		Type:             work.ContainerCollection,
		ParentID:         shared.MustParseID("0192f000-0000-7000-8000-00000000000c"),
		Name:             "Shopping",
		OrderKey:         "a0",
		CompletionPolicy: work.CompletionManual,
		ParentArchivedAt: &archivedAt,
		CreatedBy:        shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		CreatedAt:        archivedAt,
		UpdatedAt:        archivedAt,
		Version:          2,
	}

	envelope, err := event.NewContainerRenamed(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e8"), collection,
		[]work.FieldChange{{Field: work.FieldName, From: "Shop", To: "Shopping"}},
		event.Actor{Kind: shared.ActorUser, ID: collection.CreatedBy}, archivedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	if envelope.Payload["effective_archived"] != true {
		t.Errorf("effective_archived is %v, want true for a collection in an archived hub", envelope.Payload["effective_archived"])
	}
	if envelope.Payload["archived_at"] != nil {
		t.Errorf("archived_at is %v, want null - the collection carries no stamp of its own", envelope.Payload["archived_at"])
	}
}

// An event that announces nothing changed is a defect rather than something a client sent: the
// writer does not write when nothing moved.
func TestAChangeEventRefusesAnEmptyChangeSet(t *testing.T) {
	_, err := event.NewContainerRenamed(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e9"),
		work.Container{TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a")},
		nil, event.Actor{Kind: shared.ActorSystem}, time.Now(), event.Cause{})
	if err == nil {
		t.Fatal("an event with no changes was built")
	}
}

// The bucket a board is made of (B-09). `wip_limit` and `color_token` are explicit nulls in the
// payload rather than omissions, for the reason the response carries them that way: a subscriber
// that had to tell "no limit" from "this producer does not know about limits" would have to fetch
// the column, which is what a snapshot exists to avoid.
func TestTheBucketCreatedEventMatchesItsSchema(t *testing.T) {
	spec := loadEventSchema(t, event.BucketCreated)
	limit := 4

	for _, c := range []struct {
		name   string
		bucket work.Bucket
	}{
		{name: "a plain column", bucket: work.Bucket{
			ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
			TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
			CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
			Name:         "Doing", OrderKey: "a1", Version: 1,
		}},
		{name: "one that carries everything", bucket: work.Bucket{
			ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000b2"),
			TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
			CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
			Name:         "Done", OrderKey: "a2", WipLimit: &limit, IsDoneBucket: true,
			ColorToken: "surface.green", Version: 1,
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := event.NewBucketCreated(
				shared.MustParseID("0192f000-0000-7000-8000-0000000000e3"), c.bucket,
				event.Actor{
					Kind: shared.ActorUser,
					ID:   shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
				},
				time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC), event.Cause{})
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := spec.validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// The two change events of a board carry a snapshot and a change set beside it, as every change
// event in this system does: the snapshot is what the column now is, and the change set is what a
// field change trigger is written against.
func TestTheBucketChangeEventsMatchTheirSchemas(t *testing.T) {
	bucket := work.Bucket{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Name:         "In progress", OrderKey: "a1", Version: 2,
	}
	by := event.Actor{
		Kind: shared.ActorUser, ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
	at := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	eventID := shared.MustParseID("0192f000-0000-7000-8000-0000000000e4")

	for eventType, build := range map[event.Type]func() (event.Envelope, error){
		event.BucketUpdated: func() (event.Envelope, error) {
			return event.NewBucketUpdated(eventID, bucket,
				[]work.FieldChange{{Field: work.FieldName, From: "Doing", To: "In progress"}},
				by, at, event.Cause{})
		},
		event.BucketReordered: func() (event.Envelope, error) {
			return event.NewBucketReordered(eventID, bucket,
				[]work.FieldChange{{Field: work.FieldOrderKey, From: "a0", To: "a1"}},
				by, at, event.Cause{})
		},
	} {
		t.Run(string(eventType), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := loadEventSchema(t, eventType).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// An event announcing that nothing changed is a defect: the writer does not write when nothing
// moved, so reaching that point means the two disagree.
func TestABucketChangeEventRefusesAnEmptyChangeSet(t *testing.T) {
	_, err := event.NewBucketUpdated(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e4"),
		work.Bucket{
			ID:       shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
			TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		},
		nil, event.Actor{Kind: shared.ActorSystem},
		time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC), event.Cause{})
	if err == nil {
		t.Fatal("an event with no change set was built")
	}
}

// A deletion says where the entries went, so that a consumer does not have to reload the board.
func TestTheBucketDeletedEventMatchesItsSchema(t *testing.T) {
	deleted := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	bucket := work.Bucket{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Name:         "Doing", OrderKey: "a1", DeletedAt: &deleted, Version: 2,
	}
	by := event.Actor{
		Kind: shared.ActorUser, ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}

	for _, c := range []struct {
		name   string
		target shared.ID
		moved  int
	}{
		{name: "onto the leftmost remaining column",
			target: shared.MustParseID("0192f000-0000-7000-8000-0000000000b2"), moved: 3},
		{name: "off the last column of the board"},
	} {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := event.NewBucketDeleted(
				shared.MustParseID("0192f000-0000-7000-8000-0000000000e5"), bucket,
				c.target, c.moved, by, deleted, event.Cause{})
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := loadEventSchema(t, event.BucketDeleted).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// A label as a collection defines it (B-09).
func TestTheLabelCreatedEventMatchesItsSchema(t *testing.T) {
	spec := loadEventSchema(t, event.LabelCreated)

	for _, c := range []struct {
		name  string
		label work.Label
	}{
		{name: "a plain label", label: work.Label{
			ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"),
			TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
			CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
			Name:         "Urgent", ColorToken: "accent.red", Version: 1,
		}},
		{name: "one that carries a description", label: work.Label{
			ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000c2"),
			TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
			CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
			Name:         "Blocked", ColorToken: "accent.amber",
			Description: "Waiting on somebody else", Version: 1,
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := event.NewLabelCreated(
				shared.MustParseID("0192f000-0000-7000-8000-0000000000e6"), c.label,
				event.Actor{
					Kind: shared.ActorUser,
					ID:   shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
				},
				time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC), event.Cause{})
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := spec.validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// The two remaining label events. A deletion carries a snapshot and no list of the entries that
// carried the label: a collection's vocabulary is small and its entries are not, so listing them
// would make the payload unbounded.
func TestTheLabelChangeEventsMatchTheirSchemas(t *testing.T) {
	deleted := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	label := work.Label{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Name:         "Blocked", ColorToken: "accent.amber", Version: 2,
	}
	by := event.Actor{
		Kind: shared.ActorUser, ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
	eventID := shared.MustParseID("0192f000-0000-7000-8000-0000000000e7")

	for eventType, build := range map[event.Type]func() (event.Envelope, error){
		event.LabelUpdated: func() (event.Envelope, error) {
			return event.NewLabelUpdated(eventID, label,
				[]work.FieldChange{{Field: work.FieldName, From: "Urgent", To: "Blocked"}},
				by, deleted, event.Cause{})
		},
		event.LabelDeleted: func() (event.Envelope, error) {
			gone := label
			gone.DeletedAt = &deleted
			return event.NewLabelDeleted(eventID, gone, by, deleted, event.Cause{})
		},
	} {
		t.Run(string(eventType), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := loadEventSchema(t, eventType).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// The two events domain-model.md §4 names by hand. Their payload is a reference rather than a
// snapshot, which is the one place this system does not send one: a set merges separately, so a
// snapshot of the entry would carry a value another device may already have merged differently.
func TestTheItemLabelEventsMatchTheirSchemas(t *testing.T) {
	item := work.WorkItem{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-00000000000e"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemTask,
		Path:         work.RootPath(shared.MustParseID("0192f000-0000-7000-8000-00000000000e")),
		Depth:        1, Title: "Buy oat milk", OrderKey: "a0",
		CreatedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Version:   1,
	}
	labelID := shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	by := event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}
	at := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	eventID := shared.MustParseID("0192f000-0000-7000-8000-0000000000e8")

	for eventType, build := range map[event.Type]func() (event.Envelope, error){
		event.ItemLabelAdded: func() (event.Envelope, error) {
			return event.NewItemLabelAdded(eventID, item, labelID, by, at, event.Cause{})
		},
		event.ItemLabelRemoved: func() (event.Envelope, error) {
			return event.NewItemLabelRemoved(eventID, item, labelID, by, at, event.Cause{})
		},
	} {
		t.Run(string(eventType), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := loadEventSchema(t, eventType).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// An event about no label at all means the writer and the event disagree, which is a defect rather
// than something a client sent.
func TestAnItemLabelEventNeedsALabel(t *testing.T) {
	_, err := event.NewItemLabelAdded(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e8"),
		work.WorkItem{
			ID:       shared.MustParseID("0192f000-0000-7000-8000-00000000000e"),
			TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		},
		"", event.Actor{Kind: shared.ActorSystem},
		time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC), event.Cause{})
	if err == nil {
		t.Fatal("an event naming no label was built")
	}
}

// The four events of C-01, against the schemas they are published under. All four carry a reference
// and no snapshot of the entry, which is what the schemas declare and what a subscriber writes
// against.
func TestTheAssignmentAndMemberEventsMatchTheirSchemas(t *testing.T) {
	item := work.WorkItem{
		ID:           shared.MustParseID("0192f000-0000-7000-8000-00000000000e"),
		TenantID:     shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemTask,
		Path:         work.RootPath(shared.MustParseID("0192f000-0000-7000-8000-00000000000e")),
		Depth:        1, Title: "Buy oat milk", OrderKey: "a0",
		CreatedBy: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Version:   1,
	}
	accountID := shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	by := event.Actor{Kind: shared.ActorUser, ID: item.CreatedBy}
	at := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	eventID := shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")

	for eventType, build := range map[event.Type]func() (event.Envelope, error){
		event.ItemAssigned: func() (event.Envelope, error) {
			return event.NewItemAssigned(eventID, item, accountID, by, at, event.Cause{})
		},
		event.ItemUnassigned: func() (event.Envelope, error) {
			return event.NewItemUnassigned(eventID, item, accountID, by, at, event.Cause{})
		},
		event.ItemMemberAdded: func() (event.Envelope, error) {
			return event.NewItemMemberAdded(eventID, item, accountID, by, at, event.Cause{})
		},
		event.ItemMemberRemoved: func() (event.Envelope, error) {
			return event.NewItemMemberRemoved(eventID, item, accountID, by, at, event.Cause{})
		},
	} {
		t.Run(string(eventType), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}

			problems, err := loadEventSchema(t, eventType).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// The lifecycle events of both aggregates, against the schemas they are published under (B-10).
//
// Built from real transitions rather than from hand-written fixtures: the stamp a payload carries is
// the one the domain wrote, so a builder that reported the wrong state would fail here rather than
// in a subscriber's log.
func TestTheTrashAndArchiveEventsMatchTheirSchemas(t *testing.T) {
	tenant := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	collectionID := shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	hubID := shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	actor := shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	batch := shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	at := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)

	task := work.WorkItem{
		ID: shared.MustParseID("0192f000-0000-7000-8000-000000000001"), TenantID: tenant,
		CollectionID: collectionID, Type: work.ItemTask, Path: "/0192f000-0000-7000-8000-000000000001/",
		Depth: 1, Title: "Weekly shop", OrderKey: "a0", CreatedBy: actor,
		CreatedAt: at.Add(-24 * time.Hour), UpdatedAt: at, Version: 3,
	}
	archived, _, err := task.Archived(at)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	trashed, _, err := task.Trashed(at, batch)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}

	hub := work.Container{
		ID: hubID, TenantID: tenant, Type: work.ContainerHub, Name: "Private", OrderKey: "a0",
		CompletionPolicy: work.CompletionManual, CreatedBy: actor,
		CreatedAt: at.Add(-24 * time.Hour), UpdatedAt: at, Version: 2,
	}
	deleted, _, err := hub.Trashed(at, batch)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	cascade := event.Cascade{Collections: 2, Items: 17}
	who := event.Actor{Kind: shared.ActorUser, ID: actor}

	cases := []struct {
		name  string
		typed event.Type
		build func(shared.ID) (event.Envelope, error)
	}{
		{
			name: "an entry archived", typed: event.ItemArchived,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewItemArchived(id, archived, who, at, event.Cause{})
			},
		},
		{
			name: "an entry unarchived", typed: event.ItemUnarchived,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewItemUnarchived(id, task, who, at, event.Cause{})
			},
		},
		{
			name: "an entry trashed with its subtree", typed: event.ItemTrashed,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewItemTrashed(id, trashed, 4, who, at, event.Cause{})
			},
		},
		{
			name: "an entry restored", typed: event.ItemRestored,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewItemRestored(id, task, 4, who, at, event.Cause{})
			},
		},
		{
			name: "an entry purged", typed: event.ItemPurged,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewItemPurged(id, event.Purge{
					TenantID: tenant, ItemID: task.ID, Type: task.Type,
					CollectionID: collectionID, Path: task.Path, Reason: lifecycle.DeletedByRetention,
				}, who, at, event.Cause{})
			},
		},
		{
			name: "a hub deleted with its subtree", typed: event.ContainerDeleted,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerDeleted(id, deleted, cascade, who, at, event.Cause{})
			},
		},
		{
			name: "a hub restored", typed: event.ContainerRestored,
			build: func(id shared.ID) (event.Envelope, error) {
				return event.NewContainerRestored(id, hub, cascade, who, at, event.Cause{})
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := c.build(shared.MustParseID("0192f000-0000-7000-8000-0000000000e8"))
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}
			if envelope.Type != c.typed {
				t.Fatalf("event type %s, want %s", envelope.Type, c.typed)
			}

			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering the event: %v", err)
			}
			problems, err := loadEventSchema(t, c.typed).validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			for _, problem := range problems {
				t.Error(problem)
			}
		})
	}
}

// A lifecycle event that said the opposite of its own name would be a subscriber acting on a state
// that never existed. Each builder refuses the item whose stamp contradicts it - a defect in this
// process rather than something a client sent, so an internal error and never advice to a caller.
func TestALifecycleEventCannotContradictItsOwnName(t *testing.T) {
	tenant := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	actor := shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	at := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	who := event.Actor{Kind: shared.ActorUser, ID: actor}

	task := work.WorkItem{
		ID: shared.MustParseID("0192f000-0000-7000-8000-000000000001"), TenantID: tenant,
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
		Type:         work.ItemTask, Path: "/0192f000-0000-7000-8000-000000000001/", Depth: 1,
		Title: "Weekly shop", OrderKey: "a0", CreatedBy: actor, CreatedAt: at, UpdatedAt: at,
		Version: 1,
	}
	archived, _, err := task.Archived(at)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	trashed, _, err := task.Trashed(at, shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"))
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	hub := work.Container{
		ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000c"), TenantID: tenant,
		Type: work.ContainerHub, Name: "Private", OrderKey: "a0", CreatedBy: actor,
		CreatedAt: at, UpdatedAt: at, Version: 1,
	}

	for _, c := range []struct {
		name  string
		build func() (event.Envelope, error)
	}{
		{"archived, on an entry that is not", func() (event.Envelope, error) {
			return event.NewItemArchived("0192f000-0000-7000-8000-0000000000e9", task, who, at, event.Cause{})
		}},
		{"unarchived, on an entry that is archived", func() (event.Envelope, error) {
			return event.NewItemUnarchived("0192f000-0000-7000-8000-0000000000e9", archived, who, at, event.Cause{})
		}},
		{"trashed, on an entry that is not", func() (event.Envelope, error) {
			return event.NewItemTrashed("0192f000-0000-7000-8000-0000000000e9", task, 1, who, at, event.Cause{})
		}},
		{"restored, on an entry still in the trash", func() (event.Envelope, error) {
			return event.NewItemRestored("0192f000-0000-7000-8000-0000000000e9", trashed, 1, who, at, event.Cause{})
		}},
		{"deleted, on a container that is not", func() (event.Envelope, error) {
			return event.NewContainerDeleted("0192f000-0000-7000-8000-0000000000e9", hub,
				event.Cascade{}, who, at, event.Cause{})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.build(); !errors.Is(err, shared.ErrInternal) {
				t.Errorf("the builder reported %v, want an internal error", err)
			}
		})
	}
}
