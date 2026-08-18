// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var collectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")

// The item fakes. Same shape as the container ones: each records what it was given, because what
// the use case owes is not only a return value.

type items struct {
	inserted  []domain.WorkItem
	stored    map[shared.ID]domain.WorkItem
	lastKey   string
	insertErr error
}

func (i *items) Find(_ context.Context, id shared.ID) (domain.WorkItem, error) {
	item, found := i.stored[id]
	if !found {
		return domain.WorkItem{}, shared.ErrNotFound
	}
	return item, nil
}

func (i *items) LastOrderKey(context.Context, shared.ID, shared.ID) (string, error) {
	return i.lastKey, nil
}

func (i *items) Insert(_ context.Context, item domain.WorkItem) error {
	if i.insertErr != nil {
		return i.insertErr
	}
	i.inserted = append(i.inserted, item)
	i.stored[item.ID] = item
	return nil
}

// profiles is the capability matrix as db/migrations/0002 seeds it. A fake rather than a constant
// in the use case, which is the point of the profiles being data: these tests narrow them to
// prove the use case reads them rather than knowing them.
type profiles struct {
	rows []domain.CapabilityProfile
	// system is the unnarrowed topology. Separate from rows, because the two differ exactly when
	// a tenant has narrowed something - which is the case worth testing.
	system []domain.CapabilityProfile
	err    error
}

func (p *profiles) List(context.Context) ([]domain.CapabilityProfile, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.rows, nil
}

func (p *profiles) ListSystem(context.Context) ([]domain.CapabilityProfile, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.system != nil {
		return p.system, nil
	}
	return p.rows, nil
}

func systemProfiles() []domain.CapabilityProfile {
	return []domain.CapabilityProfile{
		{
			Type: domain.ItemTask,
			Capabilities: []domain.Capability{
				domain.CapabilityCompletion, domain.CapabilityNotes, domain.CapabilityBucket,
			},
			AllowedChildTypes: []domain.ItemType{domain.ItemWorkPackage},
			MaxDepth:          3,
		},
		{
			Type:              domain.ItemWorkPackage,
			Capabilities:      []domain.Capability{domain.CapabilityCompletion, domain.CapabilityNotes},
			AllowedChildTypes: []domain.ItemType{domain.ItemActivity},
			MaxDepth:          2,
		},
		{
			Type:         domain.ItemActivity,
			Capabilities: []domain.Capability{domain.CapabilityCompletion, domain.CapabilityDueDate},
			MaxDepth:     1,
		},
	}
}

type itemHarness struct {
	handler    CreateWorkItem
	items      *items
	containers *containers
	profiles   *profiles
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newItemHarness() *itemHarness {
	store := &items{stored: map[shared.ID]domain.WorkItem{}}
	containerStore := &containers{stored: map[shared.ID]domain.Container{}}
	h := &itemHarness{
		items:      store,
		containers: containerStore,
		profiles:   &profiles{rows: systemProfiles()},
		events:     &events{},
		changes:    &changes{},
		audit:      &sink{},
		authorizer: &authorizer{},
		uow:        &unitOfWork{},
	}
	h.handler = CreateWorkItem{
		Items: store, Containers: containerStore, Profiles: h.profiles,
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}

	// A collection inside a hub: the shape every one of these tests writes into.
	containerStore.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	containerStore.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

// withTask stores a task in the collection, so that a work package has something to sit in.
func (h *itemHarness) withTask() domain.WorkItem {
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000201")
	task := domain.WorkItem{
		ID: id, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
		Path: domain.RootPath(id), Depth: 1, Title: "Weekly shop", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[id] = task
	return task
}

func (h *itemHarness) withWorkPackage(parent domain.WorkItem) domain.WorkItem {
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000202")
	pkg := domain.WorkItem{
		ID: id, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemWorkPackage,
		ParentID: parent.ID, Path: parent.ChildPath(id), Depth: 2, Title: "Dairy aisle",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[id] = pkg
	return pkg
}

func itemActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
		AccountName: "Anna Beispiel", Scopes: []string{"items:write"},
	}
}

func taskCommand() CreateWorkItemCommand {
	return CreateWorkItemCommand{
		Type: domain.ItemTask, CollectionID: collectionID, Title: "  Buy milk  ",
	}
}

// The acceptance criterion of B-03, read forwards: all three levels can be created, each landing
// where the level implies.
func TestTheThreeLevelsCanAllBeCreated(t *testing.T) {
	h := newItemHarness()
	ctx := context.Background()

	task, err := h.handler.Execute(ctx, itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("the task: %v", err)
	}
	if task.Type != domain.ItemTask || task.Depth != 1 || !task.ParentID.IsZero() {
		t.Errorf("unexpected task: %+v", task)
	}
	if task.Title != "Buy milk" {
		t.Errorf("title = %q, want it trimmed", task.Title)
	}

	pkg, err := h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: collectionID, ParentID: task.ID,
		Title: "Dairy aisle",
	})
	if err != nil {
		t.Fatalf("the work package: %v", err)
	}
	if pkg.Depth != 2 || pkg.ParentID != task.ID {
		t.Errorf("unexpected work package: %+v", pkg)
	}

	activity, err := h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemActivity, CollectionID: collectionID, ParentID: pkg.ID,
		Title: "Semi-skimmed, two litres",
	})
	if err != nil {
		t.Fatalf("the activity: %v", err)
	}
	if activity.Depth != 3 || activity.ParentID != pkg.ID {
		t.Errorf("unexpected activity: %+v", activity)
	}

	// The subtree is what the paths say it is, and that is what every later query rests on.
	if !hasPrefix(pkg.Path, task.Path) || !hasPrefix(activity.Path, pkg.Path) {
		t.Errorf("the paths do not nest: %q, %q, %q", task.Path, pkg.Path, activity.Path)
	}
	if task.CollectionID != collectionID || activity.CollectionID != collectionID {
		t.Error("the collection is not carried down the subtree")
	}
}

func hasPrefix(path, prefix string) bool {
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}

// One write owes four things, and this is the test that says so.
func TestCreatingAnItemWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newItemHarness()
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	item, err := h.handler.Execute(ctx, itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("creating the task failed: %v", err)
	}

	if len(h.items.inserted) != 1 || h.items.inserted[0].ID != item.ID {
		t.Fatalf("inserted = %+v", h.items.inserted)
	}
	if item.TenantID != tenantID {
		t.Errorf("the tenant came from somewhere other than the actor: %s", item.TenantID)
	}

	if len(h.events.appended) != 1 {
		t.Fatalf("events = %d, want 1", len(h.events.appended))
	}
	announcement := h.events.appended[0]
	if announcement.Type != event.ItemCreated {
		t.Errorf("event type = %s", announcement.Type)
	}
	if announcement.Subject != event.ItemSubject(item.ID) {
		t.Errorf("subject = %s", announcement.Subject)
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("changes = %d, want 1", len(h.changes.recorded))
	}
	change := h.changes.recorded[0]
	if change.Entity != "item" || change.EntityID != item.ID {
		t.Errorf("change = %+v", change)
	}
	// The visibility filter a pull applies: a device subscribed to the collection sees the item
	// appear, at every level of the subtree.
	if change.ContainerID != collectionID {
		t.Errorf("the change is filed under %s rather than the collection", change.ContainerID)
	}
	// The event and the change describe the same state. Building the snapshot twice is how the
	// two come to disagree.
	if change.Payload["id"] != announcement.Payload["id"] ||
		change.Payload["path"] != announcement.Payload["path"] {
		t.Error("the change and the event describe different states")
	}

	if len(h.audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != ItemCreatedAction || entry.TargetID != item.ID ||
		entry.Outcome != audit.OutcomeSuccess {
		t.Errorf("entry = %+v", entry)
	}
	if entry.Context.RequestID != "01J9REQUEST" {
		t.Errorf("request id = %q", entry.Context.RequestID)
	}
	if !h.uow.committed {
		t.Error("the transaction did not commit")
	}
}

// Rule 10: no user content in the trail. The title is evidence that something was named, not a
// place to read the name; the notes are not recorded at all.
func TestTheAuditEntryCarriesNoReadableUserContent(t *testing.T) {
	h := newItemHarness()

	cmd := taskCommand()
	cmd.Title = "Buy a birthday present for Jonas"
	cmd.Notes = "He likes the blue one"

	if _, err := h.handler.Execute(context.Background(), itemActor(), cmd); err != nil {
		t.Fatalf("creating the task failed: %v", err)
	}

	entry := h.audit.entries[0]
	if _, present := entry.Changes["notes"]; present {
		t.Error("the notes reached the audit trail - a fingerprint of free text answers nothing")
	}
	// The title is there as evidence that something was named, masked rather than readable.
	title, present := entry.Changes["title"].(map[string]any)
	if !present {
		t.Fatalf("changes = %v, want the title recorded", entry.Changes)
	}
	if title["changed"] != true || title["to_hash"] == nil {
		t.Errorf("title = %v, want it fingerprinted", title)
	}

	for _, secret := range []string{cmd.Title, cmd.Notes} {
		if carries(entry.Changes, secret) {
			t.Errorf("the trail carries %q verbatim", secret)
		}
	}
	if entry.TargetLabel != "" {
		t.Errorf("target label = %q, want none: it would be the title", entry.TargetLabel)
	}
}

// carries reports whether a value appears anywhere in the masked changes, however deeply. The
// recursion is the point: a nested map is exactly where a raw value would hide.
func carries(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return typed == needle
	case map[string]any:
		for _, nested := range typed {
			if carries(nested, needle) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if carries(nested, needle) {
				return true
			}
		}
	}
	return false
}

// The permission is asked for before the transaction, at the path that decides it: a membership
// held at the hub applies downwards, so the hub has to be on the path or somebody with the right
// is refused (domain-model.md §3.2).
func TestThePermissionIsAskedForAtTheWholePathBeforeTheTransaction(t *testing.T) {
	h := newItemHarness()

	if _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); err != nil {
		t.Fatalf("creating the task failed: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("authorisation requests = %d, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionWriteItems {
		t.Errorf("permission = %s", request.Permission)
	}
	if request.TokenScope != itemsWrite {
		t.Errorf("token scope = %q", request.TokenScope)
	}
	if request.Action != ItemCreatedAction || request.TargetType != itemTarget {
		t.Errorf("the refusal would be recorded as %s on %s", request.Action, request.TargetType)
	}
	// The item does not exist yet, so the refusal names the collection it would have gone into.
	if request.TargetID != collectionID {
		t.Errorf("target = %s, want the collection", request.TargetID)
	}

	want := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}
	if len(request.Path) != len(want) {
		t.Fatalf("path = %v, want %v", request.Path, want)
	}
	for i, scope := range want {
		if request.Path[i] != scope {
			t.Errorf("path[%d] = %v, want %v", i, request.Path[i], scope)
		}
	}
}

// A refusal writes nothing. The audit entry for it is the authoriser's business, in a transaction
// of its own, which is why it would survive this rollback.
func TestARefusedRequestWritesNothing(t *testing.T) {
	h := newItemHarness()
	h.authorizer.err = shared.ErrForbidden

	_, err := h.handler.Execute(context.Background(), itemActor(), taskCommand())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}
	if len(h.items.inserted) != 0 || len(h.events.appended) != 0 ||
		len(h.changes.recorded) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused request wrote something")
	}
}

// The acceptance criterion read backwards: every forbidden combination is refused with its own
// reason, and none of them is silently ignored.
func TestEveryForbiddenCombinationIsRefusedWithItsReason(t *testing.T) {
	cases := []struct {
		name       string
		command    func(h *itemHarness) CreateWorkItemCommand
		wantDetail string
		wantIs     error
	}{
		{
			name: "an activity directly in a task, skipping the work package",
			command: func(h *itemHarness) CreateWorkItemCommand {
				task := h.withTask()
				return CreateWorkItemCommand{
					Type: domain.ItemActivity, ParentID: task.ID, Title: "Milk",
				}
			},
			wantDetail: "items.parent_type_invalid", wantIs: shared.ErrValidation,
		},
		{
			name: "a work package with no parent at all",
			command: func(*itemHarness) CreateWorkItemCommand {
				return CreateWorkItemCommand{
					Type: domain.ItemWorkPackage, CollectionID: collectionID, Title: "Dairy",
				}
			},
			wantDetail: "items.parent_item_required", wantIs: shared.ErrValidation,
		},
		{
			name: "a note on an activity, whose profile has no notes",
			command: func(h *itemHarness) CreateWorkItemCommand {
				pkg := h.withWorkPackage(h.withTask())
				return CreateWorkItemCommand{
					Type: domain.ItemActivity, ParentID: pkg.ID, Title: "Milk",
					Notes: "Semi-skimmed",
				}
			},
			wantDetail: "items.capability_not_supported", wantIs: shared.ErrCapabilityNotSupported,
		},
		{
			name: "an item in a hub rather than a collection",
			command: func(*itemHarness) CreateWorkItemCommand {
				return CreateWorkItemCommand{
					Type: domain.ItemTask, CollectionID: hubID, Title: "Milk",
				}
			},
			wantDetail: "items.collection_required", wantIs: shared.ErrValidation,
		},
		{
			name: "a collection that does not exist",
			command: func(*itemHarness) CreateWorkItemCommand {
				return CreateWorkItemCommand{
					Type:         domain.ItemTask,
					CollectionID: shared.MustParseID("0192f000-0000-7000-8000-0000000009ff"),
					Title:        "Milk",
				}
			},
			wantDetail: "items.collection_not_found", wantIs: shared.ErrNotFound,
		},
		{
			name: "a parent that does not exist",
			command: func(*itemHarness) CreateWorkItemCommand {
				return CreateWorkItemCommand{
					Type:     domain.ItemWorkPackage,
					ParentID: shared.MustParseID("0192f000-0000-7000-8000-0000000009fe"),
					Title:    "Dairy",
				}
			},
			wantDetail: "items.parent_not_found", wantIs: shared.ErrNotFound,
		},
		{
			name: "neither a collection nor a parent",
			command: func(*itemHarness) CreateWorkItemCommand {
				return CreateWorkItemCommand{Type: domain.ItemTask, Title: "Milk"}
			},
			wantDetail: "items.collection_or_parent_required", wantIs: shared.ErrValidation,
		},
		{
			name: "an empty title",
			command: func(*itemHarness) CreateWorkItemCommand {
				return CreateWorkItemCommand{
					Type: domain.ItemTask, CollectionID: collectionID, Title: "   ",
				}
			},
			wantDetail: "items.title_empty", wantIs: shared.ErrValidation,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newItemHarness()
			cmd := c.command(h)

			item, err := h.handler.Execute(context.Background(), itemActor(), cmd)
			if err == nil {
				t.Fatalf("no error, and the item was created: %+v", item)
			}
			if !errors.Is(err, c.wantIs) {
				t.Errorf("error = %v, want %v", err, c.wantIs)
			}
			if got := shared.AsError(err).DetailCode; got != c.wantDetail {
				t.Errorf("detail code = %s, want %s", got, c.wantDetail)
			}
			// Refused means nothing was written - not the row, and not the announcement of a row
			// that is not there.
			if len(h.items.inserted) != 0 || len(h.events.appended) != 0 {
				t.Error("a refused request wrote something")
			}
		})
	}
}

// The collection may be left out when the parent says which one it is. Making a client repeat it
// is making it possible to contradict.
func TestTheCollectionIsTakenFromTheParentWhenItIsNotGiven(t *testing.T) {
	h := newItemHarness()
	task := h.withTask()

	pkg, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, ParentID: task.ID, Title: "Dairy aisle",
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if pkg.CollectionID != collectionID {
		t.Errorf("collection = %s, want the parent's", pkg.CollectionID)
	}
	// And the permission was still asked for at the full path, which needed the collection to be
	// resolved before the question could be put.
	if len(h.authorizer.requests[0].Path) != 3 {
		t.Errorf("path = %v", h.authorizer.requests[0].Path)
	}
}

// Invariant I-W3, and the reason it is also a security check: the right was asked for at one
// collection, so a parent in another one would place the item somewhere nobody was authorised for.
func TestAParentInAnotherCollectionIsRefused(t *testing.T) {
	h := newItemHarness()
	task := h.withTask()

	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000cf")
	h.containers.stored[elsewhere] = domain.Container{
		ID: elsewhere, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Other", OrderKey: "a1", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}

	_, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: elsewhere, ParentID: task.ID, Title: "Dairy",
	})
	if got := shared.AsError(err).DetailCode; got != "items.parent_not_in_collection" {
		t.Fatalf("error = %v, want items.parent_not_in_collection", err)
	}
}

// The rules are data, and this is what proves the use case reads them rather than knowing them:
// a narrowed profile changes the answer without a line of code changing.
func TestTheRulesComeFromTheProfilesRatherThanFromTheCode(t *testing.T) {
	h := newItemHarness()

	// This workspace has narrowed the task profile: no notes, and no children. The system
	// defaults are untouched, which is what a narrowing means.
	h.profiles.rows = []domain.CapabilityProfile{
		{
			Type:         domain.ItemTask,
			Capabilities: []domain.Capability{domain.CapabilityCompletion},
			MaxDepth:     1,
		},
	}
	h.profiles.system = systemProfiles()

	withNotes := taskCommand()
	withNotes.Notes = "Something"
	if _, err := h.handler.Execute(context.Background(), itemActor(), withNotes); !errors.Is(
		err, shared.ErrCapabilityNotSupported) {
		t.Errorf("a note was accepted although the narrowed profile has none: %v", err)
	}

	// A type this workspace no longer offers is refused as unsupported rather than as unknown:
	// the schema still knows WORK_PACKAGE, this installation does not offer it.
	_, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: collectionID, Title: "Dairy",
	})
	if got := shared.AsError(err).DetailCode; got != "items.type_unsupported" {
		t.Errorf("error = %v, want items.type_unsupported", err)
	}

	// The plain case still works, so the narrowing refused what it should and nothing else.
	if _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); err != nil {
		t.Errorf("a plain task was refused: %v", err)
	}
}

// A narrowing may never widen. A workspace that takes a task's children away leaves nothing in
// its own profiles accepting a work package - and read off that set alone, "a type nothing
// accepts sits at the top" would make the work package a top level type it was never allowed to
// be. The topology therefore comes from the system defaults, which is why the use case reads both
// lists (domain-model.md §2).
func TestANarrowedWorkspaceCannotPromoteATypeToTheTopLevel(t *testing.T) {
	h := newItemHarness()
	h.profiles.system = systemProfiles()
	h.profiles.rows = []domain.CapabilityProfile{
		{
			Type:         domain.ItemTask,
			Capabilities: []domain.Capability{domain.CapabilityCompletion},
			MaxDepth:     1,
		},
		{
			Type:              domain.ItemWorkPackage,
			Capabilities:      []domain.Capability{domain.CapabilityCompletion},
			AllowedChildTypes: []domain.ItemType{domain.ItemActivity},
			MaxDepth:          2,
		},
	}

	_, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: collectionID, Title: "Dairy",
	})
	if got := shared.AsError(err).DetailCode; got != "items.parent_item_required" {
		t.Fatalf("error = %v, want items.parent_item_required", err)
	}
	if len(h.items.inserted) != 0 {
		t.Error("a work package was created at the top level of a workspace that narrowed nothing there")
	}
}

// Siblings are ranked after one another, and the rank comes from the store rather than from a
// counter here: two devices inserting offline must be able to land between two existing keys.
func TestANewItemIsRankedAfterItsLastSibling(t *testing.T) {
	h := newItemHarness()
	h.items.lastKey = "a5"

	item, err := h.handler.Execute(context.Background(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if item.OrderKey <= "a5" {
		t.Errorf("order key = %q, want one sorting after a5", item.OrderKey)
	}
}

// A failing write rolls the whole thing back. The event, the change and the entry are in the same
// transaction as the row for exactly this reason: an announcement of a row that is not there is
// worse than no announcement.
func TestAFailedInsertAnnouncesNothing(t *testing.T) {
	h := newItemHarness()
	h.items.insertErr = shared.ErrUnavailable

	if _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); !errors.Is(
		err, shared.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if !h.uow.rolledBack {
		t.Error("the transaction did not roll back")
	}
	if len(h.events.appended) != 0 || len(h.changes.recorded) != 0 || len(h.audit.entries) != 0 {
		t.Error("something was announced for a row that was not written")
	}
}

// The descriptor is what makes the use case reachable through all three channels, and what the
// registry checks before it will accept it.
func TestTheDescriptorDeclaresWhatEveryChannelNeeds(t *testing.T) {
	descriptor := CreateWorkItem{}.Descriptor()

	if descriptor.Name != CreateWorkItemName {
		t.Errorf("name = %s", descriptor.Name)
	}
	if descriptor.RESTOperation() != "createWorkItem" || descriptor.MCPTool() != "create_work_item" ||
		descriptor.AutomationAction() != "CREATE_WORK_ITEM" {
		t.Errorf("the channel identities are %s, %s, %s", descriptor.RESTOperation(),
			descriptor.MCPTool(), descriptor.AutomationAction())
	}
	if !descriptor.Audit.Required || descriptor.Audit.Action != ItemCreatedAction {
		t.Errorf("audit declaration = %+v", descriptor.Audit)
	}
	if descriptor.TokenScope != itemsWrite {
		t.Errorf("token scope = %q", descriptor.TokenScope)
	}

	// The fields this use case does not own are not declared, so a request carrying one is
	// refused by name rather than accepted and dropped.
	declared := map[string]bool{}
	for _, field := range descriptor.Input {
		declared[field.Name] = true
	}
	for _, owned := range []string{"type", "title", "collection_id", "parent_id", "notes"} {
		if !declared[owned] {
			t.Errorf("%s is not declared", owned)
		}
	}
	for _, later := range []string{"bucket_id", "label_ids", "assignee_id", "due_at", "cover"} {
		if declared[later] {
			t.Errorf("%s is declared, though no use case writes it yet", later)
		}
	}

	if err := descriptor.ValidateInput(map[string]any{
		"type": "TASK", "title": "Buy milk", "bucket_id": "0192f000-0000-7000-8000-00000000000e",
	}); err == nil {
		t.Error("a field nothing writes was accepted rather than refused by name")
	}
}

// The output is the contract's shape, in the contract's words, so that all three channels
// describe the item alike.
func TestTheOutputIsTheContractsShape(t *testing.T) {
	h := newItemHarness()
	task := h.withTask()

	out, err := h.handler.Descriptor().Handler.Invoke(context.Background(), itemActor(),
		map[string]any{"type": "WORK_PACKAGE", "parent_id": task.ID.String(), "title": "Dairy"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	for _, field := range []string{
		"id", "type", "collection_id", "parent_id", "path", "depth", "title", "completion",
		"order_key", "created_by", "created_at", "updated_at", "version",
	} {
		if _, present := out[field]; !present {
			t.Errorf("%s is missing from the output", field)
		}
	}
	if _, present := out["notes"]; present {
		t.Error("an unset note was returned anyway")
	}
	completion, _ := out["completion"].(map[string]any)
	if completion["is_completed"] != false {
		t.Errorf("completion = %v, want an open one", out["completion"])
	}
}
