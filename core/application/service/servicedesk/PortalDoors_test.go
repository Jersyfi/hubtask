// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// portalEntries stands in for the entry use cases the portal writes through: it records each
// call with the actor it came as.
type portalEntries struct {
	names  []string
	inputs []usecase.Input
	actors []appshared.ActorContext
}

func (e *portalEntries) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	e.names, e.inputs, e.actors = append(e.names, name), append(e.inputs, in), append(e.actors, actor)
	if name == createEntryUseCase {
		return usecase.Output{"id": itemID.String()}, nil
	}
	return usecase.Output{"id": "comment"}, nil
}

func portalDoors(t *testing.T, portal Portal) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil, RaiseRequest{Portal: portal}.Descriptor(), CommentOnMyRequest{Portal: portal}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// UC-SVC-05 check 2 through the doors: a request of a type the catalogue offers is raised
// through CreateWorkItem on the PORTAL door in the requester's name and audited; a type it does
// not offer and somebody without REQUEST there create nothing.
func TestARequestIsRaisedOnThePortalDoorThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	entries, trail, auth := &portalEntries{}, &trailOf{}, &gate{allow: true}
	portal := Portal{
		Entries: entries, Containers: collections{}, EntryTypes: translatedTypes{},
		Items: portalItems{rows: map[shared.ID]work.WorkItem{itemID: {
			ID: itemID, TenantID: tenantID, CollectionID: collection, Title: "Printer jams", EntryType: "incident", CreatedBy: handlerID,
		}}},
		Desk: Desk{Authorizer: auth, UnitOfWork: inline{}, Audit: trail, Clock: at(monday), IDs: &seqIDs{}},
	}
	doors := portalDoors(t, portal)
	raise := func(entryType string) usecase.Input {
		return usecase.Input{
			"collection_id": collection.String(), "entry_type": entryType, "title": "Printer jams",
			"notes": "Third floor", "custom_fields": map[string]any{"room": "3.14"},
		}
	}

	if _, err := doors.Invoke(ctx, RaiseRequestName, reader(), raise("change")); detailOf(err) != "requests.type_not_offered" {
		t.Fatalf("a type the catalogue does not offer: %v", err)
	}
	auth.allow = false
	if _, err := doors.Invoke(ctx, RaiseRequestName, reader(), raise("incident")); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("raising without REQUEST: %v", err)
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, RaiseRequestName, reader(), usecase.Input{"entry_type": "incident", "title": "x"}); err == nil {
		t.Fatal("a request without its collection was accepted")
	}
	if len(entries.names) != 0 || len(trail.entries) != 0 {
		t.Fatalf("a refused request was written: %v, %d entries", entries.names, len(trail.entries))
	}

	out, err := doors.Invoke(ctx, RaiseRequestName, reader(), raise("incident"))
	if err != nil || out["title"] != "Printer jams" {
		t.Fatalf("raising: %v %v", out, err)
	}
	if entries.names[0] != createEntryUseCase || entries.actors[0].Channel != string(work.ChannelPortal) ||
		entries.actors[0].AccountID != handlerID {
		t.Fatalf("the entry was created as %s by %+v", entries.names[0], entries.actors[0])
	}
	in := entries.inputs[0]
	if in["notes"] != "Third floor" || in["custom_fields"] == nil || in["entry_type"] != "incident" {
		t.Fatalf("the entry's input: %v", in)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != RequestRaisedAction || trail.entries[0].TargetID != itemID {
		t.Fatalf("the trail: %+v", trail.entries)
	}
}

// UC-SVC-05 check 5 through the doors: the requester answers on their own request through
// AddComment on the PORTAL door; somebody else's request is not found and nothing is written.
func TestTheRequesterAnswersOnlyOnTheirOwnRequestThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005ff")
	entries, trail := &portalEntries{}, &trailOf{}
	portal := Portal{
		Entries: entries, Containers: collections{},
		Items: portalItems{rows: map[shared.ID]work.WorkItem{
			itemID: {ID: itemID, TenantID: tenantID, CollectionID: collection, CreatedBy: handlerID},
			other:  {ID: other, TenantID: tenantID, CollectionID: collection, CreatedBy: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005fe")},
		}},
		Desk: Desk{Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: trail, Clock: at(monday), IDs: &seqIDs{}},
	}
	doors := portalDoors(t, portal)

	if _, err := doors.Invoke(ctx, CommentOnMyRequestName, reader(), usecase.Input{"item_id": other.String(), "body": "Any news?"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("answering on somebody else's request: %v", err)
	}
	if _, err := doors.Invoke(ctx, CommentOnMyRequestName, reader(), usecase.Input{
		"item_id": itemID.String(), "body": "Any news?", "parent_comment_id": "no id",
	}); err == nil {
		t.Fatal("a parent that is no id was accepted")
	}
	if len(entries.names) != 0 || len(trail.entries) != 0 {
		t.Fatalf("a refused answer was written: %v", entries.names)
	}

	parent := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005fd")
	if _, err := doors.Invoke(ctx, CommentOnMyRequestName, reader(), usecase.Input{
		"item_id": itemID.String(), "body": "Any news?", "parent_comment_id": parent.String(),
	}); err != nil {
		t.Fatalf("answering: %v", err)
	}
	if entries.names[0] != addCommentUseCase || entries.actors[0].Channel != string(work.ChannelPortal) ||
		entries.inputs[0]["parent_comment_id"] != parent.String() {
		t.Fatalf("the comment went through %s as %+v with %v", entries.names[0], entries.actors[0], entries.inputs[0])
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != RequestCommentedAction {
		t.Fatalf("the trail: %+v", trail.entries)
	}
}
