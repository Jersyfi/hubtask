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
	eventHub      = shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	eventOtherHub = shared.MustParseID("0192f000-0000-7000-8000-00000000001c")
	archivedAt    = occurred.Add(-time.Hour)
)

func eventCollectionIn(hub shared.ID) work.Container {
	return work.Container{
		ID: eventCollection, TenantID: eventTenant, Type: work.ContainerCollection, ParentID: hub,
		Name: "Shopping", OrderKey: "a0", CompletionPolicy: work.CompletionRollup,
		CreatedBy: eventAuthor, CreatedAt: occurred, UpdatedAt: occurred, Version: 4,
	}
}

func by() Actor { return Actor{Kind: shared.ActorUser, ID: eventAuthor} }

// Every container event carries the same snapshot, in the API's words, so that a webhook payload
// and a REST response describe the same object alike.
func TestEveryContainerEventCarriesTheSameSnapshot(t *testing.T) {
	container := eventCollectionIn(eventHub)
	renamed := []work.FieldChange{{Field: work.FieldName, From: "Shop", To: "Shopping"}}

	events := map[Type]func() (Envelope, error){
		ContainerCreated: func() (Envelope, error) {
			return NewContainerCreated(eventID, container, by(), occurred, Cause{})
		},
		ContainerRenamed: func() (Envelope, error) {
			return NewContainerRenamed(eventID, container, renamed, by(), occurred, Cause{})
		},
		ContainerPoliciesUpdated: func() (Envelope, error) {
			return NewContainerPoliciesUpdated(eventID, container,
				[]work.FieldChange{{Field: work.FieldCompletionPolicy, From: "MANUAL", To: "ROLLUP"}},
				by(), occurred, Cause{})
		},
		ContainerMoved: func() (Envelope, error) {
			return NewContainerMoved(eventID, container, eventOtherHub, by(), occurred, Cause{})
		},
		ContainerArchived: func() (Envelope, error) {
			return NewContainerArchived(eventID, container, by(), occurred, Cause{})
		},
		ContainerUnarchived: func() (Envelope, error) {
			return NewContainerUnarchived(eventID, container, by(), occurred, Cause{})
		},
	}

	for want, build := range events {
		t.Run(string(want), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}
			if envelope.Type != want {
				t.Errorf("type = %s, want %s", envelope.Type, want)
			}
			if envelope.Subject != "container/"+eventCollection.String() {
				t.Errorf("subject = %s", envelope.Subject)
			}
			if envelope.TenantID != eventTenant {
				t.Errorf("tenant = %s", envelope.TenantID)
			}

			for field, expected := range map[string]any{
				"id":                 eventCollection.String(),
				"type":               "COLLECTION",
				"parent_id":          eventHub.String(),
				"name":               "Shopping",
				"order_key":          "a0",
				"completion_policy":  "ROLLUP",
				"archived_at":        nil,
				"effective_archived": false,
				"version":            4,
			} {
				if envelope.Payload[field] != expected {
					t.Errorf("%s = %v, want %v", field, envelope.Payload[field], expected)
				}
			}
		})
	}
}

// A hub has no parent, and saying so as an explicit null is what tells a consumer which level it is
// looking at without interpreting the type.
func TestTheEventOfAHubSaysItsParentIsNull(t *testing.T) {
	hub := work.Container{
		ID: eventHub, TenantID: eventTenant, Type: work.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: eventAuthor, CreatedAt: occurred, UpdatedAt: occurred, Version: 1,
	}

	envelope, err := NewContainerArchived(eventID, hub, by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	parent, present := envelope.Payload["parent_id"]
	if !present || parent != nil {
		t.Errorf("parent_id = %v, want an explicit null", parent)
	}
	// A hub has never been configured and reads as the default, rather than as the empty string the
	// column holds.
	if envelope.Payload["completion_policy"] != "MANUAL" {
		t.Errorf("completion_policy = %v, want the default", envelope.Payload["completion_policy"])
	}
}

// The two archive facts stay separate in the payload: `archived_at` is this container's own stamp,
// `effective_archived` is whether it may be written to at all.
func TestTheArchiveFieldsSayWhichArchivingItIs(t *testing.T) {
	t.Run("archived in its own right", func(t *testing.T) {
		container := eventCollectionIn(eventHub)
		container.ArchivedAt = &archivedAt

		envelope, err := NewContainerArchived(eventID, container, by(), occurred, Cause{})
		if err != nil {
			t.Fatalf("building the event: %v", err)
		}
		if envelope.Payload["archived_at"] != archivedAt.UTC() {
			t.Errorf("archived_at = %v", envelope.Payload["archived_at"])
		}
		if envelope.Payload["effective_archived"] != true {
			t.Error("an archived container does not report itself as read-only")
		}
	})

	t.Run("read-only because its hub is archived", func(t *testing.T) {
		container := eventCollectionIn(eventHub)
		container.ParentArchivedAt = &archivedAt

		envelope, err := NewContainerUnarchived(eventID, container, by(), occurred, Cause{})
		if err != nil {
			t.Fatalf("building the event: %v", err)
		}
		if envelope.Payload["archived_at"] != nil {
			t.Errorf("archived_at = %v, want null - it carries no stamp of its own",
				envelope.Payload["archived_at"])
		}
		if envelope.Payload["effective_archived"] != true {
			t.Error("a collection in an archived hub does not report itself as read-only")
		}
	})
}

// The change set is what a field change trigger is written against: a rule fires on "the name
// became X" or on "it stopped being Y", and only the second needs the value that went.
func TestTheChangeSetCarriesBothSidesOfEveryFieldThatMoved(t *testing.T) {
	envelope, err := NewContainerRenamed(eventID, eventCollectionIn(eventHub),
		[]work.FieldChange{
			{Field: work.FieldName, From: "Shop", To: "Shopping"},
			{Field: work.FieldIcon, From: "basket", To: ""},
		}, by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	changeSet, ok := envelope.Payload["change_set"].(map[string]any)
	if !ok || len(changeSet) != 2 {
		t.Fatalf("change set = %v", envelope.Payload["change_set"])
	}
	name, ok := changeSet["name"].(map[string]any)
	if !ok || name["from"] != "Shop" || name["to"] != "Shopping" {
		t.Errorf("the name change does not carry both sides: %v", changeSet["name"])
	}
	// A field that did not move is not in the set, which is what keeps a rename from looking like a
	// change to everything.
	if _, present := changeSet["order_key"]; present {
		t.Errorf("an untouched field is in the change set: %v", changeSet)
	}
}

// An event that announces nothing changed is a defect rather than something a client sent: the
// writer does not write when nothing moved, so the two disagreeing is a bug.
func TestAContainerChangeEventRefusesAnEmptyChangeSet(t *testing.T) {
	for name, build := range map[string]func() (Envelope, error){
		"renamed": func() (Envelope, error) {
			return NewContainerRenamed(eventID, eventCollectionIn(eventHub), nil, by(), occurred, Cause{})
		},
		"policies updated": func() (Envelope, error) {
			return NewContainerPoliciesUpdated(eventID, eventCollectionIn(eventHub), nil, by(), occurred, Cause{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := build()
			if !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("error %v, want an internal one", err)
			}
		})
	}
}

// A consumer that cares only about reparenting compares the hub the collection came from with the
// one it is in; equal identifiers mean a reorder.
func TestTheMoveEventSaysWhichHubItCameFrom(t *testing.T) {
	envelope, err := NewContainerMoved(eventID, eventCollectionIn(eventHub), eventOtherHub,
		by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	if envelope.Payload["from_parent_id"] != eventOtherHub.String() {
		t.Errorf("from_parent_id = %v", envelope.Payload["from_parent_id"])
	}
	if envelope.Payload["parent_id"] != eventHub.String() {
		t.Errorf("parent_id = %v", envelope.Payload["parent_id"])
	}
}

// The optional text fields are left out rather than sent as null: a client tolerates unknown
// fields, and an absent one says "not set" just as clearly with fewer bytes.
func TestTheOptionalFieldsAreAbsentRatherThanNull(t *testing.T) {
	container := eventCollectionIn(eventHub)
	container.Description, container.Icon, container.ColorToken = "Weekly", "", "blue"

	envelope, err := NewContainerArchived(eventID, container, by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	if envelope.Payload["description"] != "Weekly" || envelope.Payload["color_token"] != "blue" {
		t.Errorf("the set fields did not travel: %v", envelope.Payload)
	}
	if _, present := envelope.Payload["icon"]; present {
		t.Errorf("an unset field was sent anyway: %v", envelope.Payload["icon"])
	}
}
