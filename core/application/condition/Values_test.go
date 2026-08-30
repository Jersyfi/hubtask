// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	valuesTenant = shared.ID("01936f2a-7c1e-7000-8000-000000000e01")
	valuesItem   = shared.ID("01936f2a-7c1e-7000-8000-000000000e02")
	valuesParent = shared.ID("01936f2a-7c1e-7000-8000-000000000e03")
	collection   = shared.ID("01936f2a-7c1e-7000-8000-000000000e04")
	hub          = shared.ID("01936f2a-7c1e-7000-8000-000000000e05")
	actorAccount = shared.ID("01936f2a-7c1e-7000-8000-000000000e06")
	readAt       = time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
)

type entryStore struct{ rows map[shared.ID]work.WorkItem }

func (s entryStore) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, found := s.rows[id]
	if !found {
		return work.WorkItem{}, shared.ErrNotFound.WithDetail("items.item_not_found")
	}
	return item, nil
}

type containerStore struct{ rows map[shared.ID]work.Container }

func (s containerStore) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, found := s.rows[id]
	if !found {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.container_not_found")
	}
	return container, nil
}

func workspace() (condition.Entries, condition.Containers) {
	return entryStore{rows: map[shared.ID]work.WorkItem{
			valuesItem: {
				ID: valuesItem, TenantID: valuesTenant, CollectionID: collection,
				ParentID: valuesParent, Title: "Buy milk", Type: work.ItemTask,
			},
			valuesParent: {
				ID: valuesParent, TenantID: valuesTenant, CollectionID: collection,
				Title: "Groceries", Type: work.ItemTask,
			},
		}}, containerStore{rows: map[shared.ID]work.Container{
			collection: {ID: collection, Type: work.ContainerCollection, Name: "Home", ParentID: hub},
			hub:        {ID: hub, Type: work.ContainerHub, Name: "Life"},
		}}
}

func itemEnvelope() event.Envelope {
	return event.Envelope{
		ID: shared.ID("01936f2a-7c1e-7000-8000-000000000e07"), Type: event.ItemUpdated,
		TenantID: valuesTenant, Subject: "item/" + valuesItem.String(),
		Actor:      event.Actor{Kind: shared.ActorUser, ID: actorAccount},
		OccurredAt: readAt,
		Payload:    map[string]any{"collection_id": collection.String()},
	}
}

func resolved(t *testing.T, values condition.Values, name string) any {
	t.Helper()
	value, present, err := values.Resolve(context.Background(), name)
	if err != nil {
		t.Fatalf("resolving %s: %v", name, err)
	}
	if !present {
		t.Fatalf("%s is absent", name)
	}
	return value
}

// The whole activation, name by name: the same names a condition and an outbound body template
// read, answered from one event.
func TestTheActivationAnswersEveryDocumentedName(t *testing.T) {
	entries, containers := workspace()
	values := condition.Values{
		Envelope: itemEnvelope(), Now: readAt,
		Payload: map[string]any{"order_id": "42"},
		Entries: entries, Containers: containers,
	}

	if got := resolved(t, values, "now"); got != readAt {
		t.Errorf("now = %v", got)
	}
	eventDoc, _ := resolved(t, values, "event").(map[string]any)
	if eventDoc["type"] != event.ItemUpdated.String() || eventDoc["subject"] != "item/"+valuesItem.String() {
		t.Errorf("event = %v", eventDoc)
	}
	actorDoc, _ := resolved(t, values, "actor").(map[string]any)
	if actorDoc["kind"] != string(shared.ActorUser) || actorDoc["id"] != actorAccount.String() {
		t.Errorf("actor = %v", actorDoc)
	}
	payload, _ := resolved(t, values, "payload").(map[string]any)
	if payload["order_id"] != "42" {
		t.Errorf("payload = %v", payload)
	}
	tenantDoc, _ := resolved(t, values, "tenant").(map[string]any)
	if _, present := tenantDoc["settings"]; !present {
		t.Errorf("tenant = %v", tenantDoc)
	}

	itemDoc, _ := resolved(t, values, "item").(map[string]any)
	if itemDoc["title"] != "Buy milk" {
		t.Errorf("item = %v", itemDoc)
	}
	parentDoc, _ := resolved(t, values, "parent").(map[string]any)
	if parentDoc["title"] != "Groceries" {
		t.Errorf("parent = %v", parentDoc)
	}
	collectionDoc, _ := resolved(t, values, "collection").(map[string]any)
	if collectionDoc["name"] != "Home" {
		t.Errorf("collection = %v", collectionDoc)
	}
	hubDoc, _ := resolved(t, values, "hub").(map[string]any)
	if hubDoc["name"] != "Life" {
		t.Errorf("hub = %v", hubDoc)
	}
}

// A name the environment declared and the activation cannot produce is absent, not an error and
// never false: a caller with no lookups - the dedupe key inside the dispatcher's transaction -
// asks about `item` and hears "not there".
func TestAMissingValueIsAbsentRatherThanAnError(t *testing.T) {
	values := condition.Values{Envelope: itemEnvelope(), Now: readAt}

	for _, name := range []string{"item", "parent", "collection", "hub"} {
		_, present, err := values.Resolve(context.Background(), name)
		if err != nil || present {
			t.Errorf("%s with no lookups: present=%v err=%v", name, present, err)
		}
	}
	if _, present, _ := values.Resolve(context.Background(), "no_such_name"); present {
		t.Error("an undeclared name resolved")
	}
	// An absent payload is an empty document rather than a failure: a condition written for one
	// trigger and used on another asks about something that is not there.
	payload, _ := resolved(t, values, "payload").(map[string]any)
	if len(payload) != 0 {
		t.Errorf("payload = %v", payload)
	}
}

// An entry deleted between the event and the run reads as absent: a condition asking about a
// deleted entry gets "not there", which is true.
func TestADeletedEntryReadsAsAbsent(t *testing.T) {
	_, containers := workspace()
	values := condition.Values{
		Envelope: itemEnvelope(), Now: readAt,
		Entries: entryStore{rows: map[shared.ID]work.WorkItem{}}, Containers: containers,
	}
	if _, present, err := values.Resolve(context.Background(), "item"); err != nil || present {
		t.Errorf("a deleted entry: present=%v err=%v", present, err)
	}
}

// A run no event started - a RELATIVE_DATE run measured from one entry - answers `item` from the
// command's subject, and `collection` and `hub` by reading it.
func TestASubjectAnswersWhereThereIsNoEvent(t *testing.T) {
	entries, containers := workspace()
	values := condition.Values{
		Envelope: event.Envelope{}, Now: readAt, Subject: valuesItem,
		Entries: entries, Containers: containers,
	}

	itemDoc, _ := resolved(t, values, "item").(map[string]any)
	if itemDoc["title"] != "Buy milk" {
		t.Errorf("item = %v", itemDoc)
	}
	collectionDoc, _ := resolved(t, values, "collection").(map[string]any)
	if collectionDoc["name"] != "Home" {
		t.Errorf("collection = %v", collectionDoc)
	}
	hubDoc, _ := resolved(t, values, "hub").(map[string]any)
	if hubDoc["name"] != "Life" {
		t.Errorf("hub = %v", hubDoc)
	}
}

// The envelope readers: what an event is about, read from its subject and payload.
func TestTheEnvelopeReadersReadTheirShapes(t *testing.T) {
	if got := condition.ItemOf(itemEnvelope()); got != valuesItem {
		t.Errorf("ItemOf = %s", got)
	}
	if got := condition.CollectionOf(itemEnvelope()); got != collection {
		t.Errorf("CollectionOf = %s", got)
	}

	collectionEvent := event.Envelope{Payload: map[string]any{
		"type": string(work.ContainerCollection), "id": collection.String(),
	}}
	if got := condition.CollectionOf(collectionEvent); got != collection {
		t.Errorf("a collection's own event names %s", got)
	}
	hubEvent := event.Envelope{Payload: map[string]any{
		"type": string(work.ContainerHub), "id": hub.String(),
	}}
	if got := condition.HubOf(hubEvent); got != hub {
		t.Errorf("a hub's own event names %s", got)
	}
	if got := condition.HubOf(itemEnvelope()); !got.IsZero() {
		t.Errorf("an item event names hub %s", got)
	}
}

// A read that fails for a real reason - not absence - propagates rather than reading as false.
type failingEntries struct{}

func (failingEntries) Find(context.Context, shared.ID) (work.WorkItem, error) {
	return work.WorkItem{}, shared.ErrUnavailable.WithDetail("postgres.query_failed")
}

func TestARealReadFailurePropagates(t *testing.T) {
	values := condition.Values{
		Envelope: itemEnvelope(), Now: readAt, Entries: failingEntries{},
	}
	_, _, err := values.Resolve(context.Background(), "item")
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v, want the read's own failure", err)
	}
}
