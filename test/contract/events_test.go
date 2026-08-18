// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Jersyfi/hubtask/core/domain/event"
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
