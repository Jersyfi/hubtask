// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// itemMembers is the fake membership and its OR-set tags, the shape itemLabels has. It keeps both,
// because writing one without the other is the failure the port exists to prevent: a membership
// with no tag merges as last writer wins and loses a concurrent change.
type itemMembers struct {
	carried  map[shared.ID][]shared.ID
	elements map[shared.ID]map[shared.ID]domain.SetElement

	listErr   error
	addErr    error
	removeErr error
}

func newItemMembers() *itemMembers {
	return &itemMembers{
		carried:  map[shared.ID][]shared.ID{},
		elements: map[shared.ID]map[shared.ID]domain.SetElement{},
	}
}

func (m *itemMembers) List(_ context.Context, itemID shared.ID) ([]shared.ID, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return slices.Clone(m.carried[itemID]), nil
}

func (m *itemMembers) Add(_ context.Context, itemID, accountID shared.ID, tag shared.HLC) error {
	if m.addErr != nil {
		return m.addErr
	}
	if !slices.Contains(m.carried[itemID], accountID) {
		m.carried[itemID] = append(m.carried[itemID], accountID)
	}
	m.tag(itemID, accountID, func(element *domain.SetElement) {
		element.AddedAt, element.RemovedAt = tag, shared.HLC{}
	})
	return nil
}

func (m *itemMembers) Remove(
	_ context.Context, itemID, accountID shared.ID, tag shared.HLC,
) (bool, error) {
	if m.removeErr != nil {
		return false, m.removeErr
	}
	carried := slices.Contains(m.carried[itemID], accountID)
	m.carried[itemID] = slices.DeleteFunc(m.carried[itemID], func(id shared.ID) bool {
		return id == accountID
	})
	m.tag(itemID, accountID, func(element *domain.SetElement) { element.RemovedAt = tag })
	return carried, nil
}

func (m *itemMembers) Elements(_ context.Context, itemID shared.ID) ([]domain.SetElement, error) {
	elements := make([]domain.SetElement, 0, len(m.elements[itemID]))
	for _, element := range m.elements[itemID] {
		elements = append(elements, element)
	}
	return elements, nil
}

func (m *itemMembers) tag(itemID, accountID shared.ID, apply func(*domain.SetElement)) {
	if m.elements[itemID] == nil {
		m.elements[itemID] = map[shared.ID]domain.SetElement{}
	}
	element := m.elements[itemID][accountID]
	element.ElementID = accountID
	apply(&element)
	m.elements[itemID][accountID] = element
}

var _ repository.ItemMembers = (*itemMembers)(nil)

type itemMemberHarness struct {
	add         AddMember
	remove      RemoveMember
	items       *items
	itemMembers *itemMembers
	containers  *containers
	visibility  *visibility
	events      *events
	changes     *changes
	audit       *sink
	history     *journal
	authorizer  *authorizer
	uow         *unitOfWork
}

func newItemMemberHarness(t *testing.T) *itemMemberHarness {
	t.Helper()

	h := &itemMemberHarness{
		items:       &items{stored: map[shared.ID]domain.WorkItem{}},
		itemMembers: newItemMembers(),
		containers:  &containers{stored: map[shared.ID]domain.Container{}},
		visibility:  newVisibility(assigneeID, accountID),
		events:      &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}

	writer := ItemMemberWriter{
		Items: h.items, ItemMembers: h.itemMembers, Containers: h.containers,
		Profiles: &profiles{rows: assignmentProfiles()}, Authorizer: h.authorizer,
		Visibility: h.visibility, Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.add = AddMember{Writer: writer}
	h.remove = RemoveMember{Writer: writer}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

func (h *itemMemberHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: assignedItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(assignedItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[assignedItem] = item
	return item
}

func memberCmd() MemberCommand {
	return MemberCommand{ItemID: assignedItem, AccountID: assigneeID}
}

// One change owes four things, and this is the test that says so.
func TestAddingAMemberWritesTheSetTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	set, err := h.add.Execute(ctx, actor(), memberCmd())
	if err != nil {
		t.Fatalf("adding the member failed: %v", err)
	}

	if set.ItemID != assignedItem || !slices.Contains(set.AccountIDs, assigneeID) {
		t.Fatalf("the entry does not carry the member: %+v", set)
	}

	t.Run("the membership and its tag are written together", func(t *testing.T) {
		elements, err := h.itemMembers.Elements(ctx, assignedItem)
		if err != nil {
			t.Fatalf("reading the tags: %v", err)
		}
		if len(elements) != 1 || !elements[0].IsPresent() {
			t.Fatalf("the tags say the account is absent: %+v", elements)
		}
		if elements[0].AddedAt.IsZero() {
			t.Error("the addition carries no clock reading, so nothing can merge it")
		}
	})

	t.Run("the event carries the reference rather than a snapshot", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.ItemMemberAdded {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["account_id"] != assigneeID.String() {
			t.Errorf("the event names %v", announcement.Payload["account_id"])
		}
		if _, snapshot := announcement.Payload["title"]; snapshot {
			t.Error("the event carries an entry snapshot, which the set would already have merged")
		}
	})

	// The payload is the one element that moved, not the whole set: a payload naming the array
	// would let the later of two writers erase the other's member (offline-sync.md §4.2).
	t.Run("the change names the element, the set, and the tag that decides it", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if change.Payload["set"] != string(domain.SetMembers) ||
			change.Payload["element_id"] != assigneeID.String() ||
			change.Payload["op"] != "add" {
			t.Errorf("unexpected payload: %+v", change.Payload)
		}
		if change.HLC.IsZero() {
			t.Error("the change carries no clock reading")
		}
		if change.ContainerID != hubID {
			t.Errorf("the change is filed under %s, want the hub", change.ContainerID)
		}
	})

	t.Run("the audit entry names the account by identifier", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != ItemMemberAddedAction || entry.TargetID != assignedItem {
			t.Errorf("unexpected entry: %+v", entry)
		}
		recorded, _ := entry.Changes["account_id"].(map[string]any)
		if recorded == nil || recorded["to"] != assigneeID.String() {
			t.Errorf("the account is not in the trail: %+v", entry.Changes)
		}
	})

	t.Run("the history keeps the account on the side it moved to", func(t *testing.T) {
		if len(h.history.entries) != 1 {
			t.Fatalf("%d history entries, want 1", len(h.history.entries))
		}
		step := h.history.entries[0]
		if step.Verb != activity.ItemMemberAdded {
			t.Errorf("verb %s", step.Verb)
		}
		field, _ := step.ChangeSet["account_id"].(map[string]any)
		if field == nil || field["to"] != assigneeID.String() {
			t.Errorf("the step does not name the account: %+v", step.ChangeSet)
		}
	})
}

// The permission question is the entry's rather than the workspace's: putting somebody on a piece
// of work is writing an entry, not granting them access to anything.
func TestAddingAMemberAsksForThePermissionToWriteItems(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)

	if _, err := h.add.Execute(context.Background(), actor(), memberCmd()); err != nil {
		t.Fatalf("adding the member failed: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionWriteItems {
		t.Errorf("permission %s, want the one for writing entries", request.Permission)
	}
	if request.TokenScope != itemsWrite {
		t.Errorf("token scope %s", request.TokenScope)
	}
}

// Somebody who cannot see the entry is refused rather than stored, with the same answer as an
// account of another tenant and one that does not exist (T-04).
func TestAddingAnAccountWithoutAccessIsRefused(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)

	cmd := memberCmd()
	cmd.AccountID = strangerID

	_, err := h.add.Execute(context.Background(), actor(), cmd)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation refusal", err)
	}
	if code := shared.AsError(err).DetailCode; code != "items.account_without_access" {
		t.Errorf("detail code %q", code)
	}
	if len(h.itemMembers.carried[assignedItem]) != 0 {
		t.Error("the member was written anyway")
	}
}

// The check is on the way in only. Somebody who has lost access has to be removable - that is
// precisely the tidying up a revoked membership leaves behind, and refusing it would strand them on
// the entry for good.
func TestAnAccountThatLostAccessCanStillBeRemoved(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.add.Execute(ctx, actor(), memberCmd()); err != nil {
		t.Fatalf("adding the member failed: %v", err)
	}
	delete(h.visibility.reachable, assigneeID)

	set, err := h.remove.Execute(ctx, actor(), memberCmd())
	if err != nil {
		t.Fatalf("removing a member who lost access failed: %v", err)
	}
	if len(set.AccountIDs) != 0 {
		t.Errorf("the entry still carries %v", set.AccountIDs)
	}
}

// Adding somebody an entry already carries succeeds and announces nothing - but the tag moves
// forward all the same, so that a concurrent removal on another device does not win a merge it
// should not.
func TestAddingAMemberTwiceAnnouncesNothingAndMovesTheTag(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.add.Execute(ctx, actor(), memberCmd()); err != nil {
		t.Fatalf("the first addition failed: %v", err)
	}
	first, err := h.itemMembers.Elements(ctx, assignedItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}

	if _, err := h.add.Execute(ctx, actor(), memberCmd()); err != nil {
		t.Fatalf("the second addition failed: %v", err)
	}

	if len(h.events.appended) != 1 {
		t.Errorf("%d events, want the repeat to have announced nothing", len(h.events.appended))
	}
	second, err := h.itemMembers.Elements(ctx, assignedItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}
	if !second[0].AddedAt.After(first[0].AddedAt) {
		t.Error("the tag did not move, so a concurrent removal could win a merge it should not")
	}
}

// A removal of somebody who was never there succeeds and records the decision anyway: a device that
// never saw the addition still has to merge the removal.
func TestRemovingAMemberThatIsNotThereRecordsTheDecision(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	set, err := h.remove.Execute(ctx, actor(), memberCmd())
	if err != nil {
		t.Fatalf("removing failed: %v", err)
	}
	if len(set.AccountIDs) != 0 {
		t.Errorf("the entry carries %v", set.AccountIDs)
	}
	if len(h.events.appended) != 0 {
		t.Errorf("%d events, want none: the set did not move", len(h.events.appended))
	}

	elements, err := h.itemMembers.Elements(ctx, assignedItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}
	if len(elements) != 1 || elements[0].RemovedAt.IsZero() {
		t.Errorf("the removal left no tag to merge: %+v", elements)
	}
}

// An activity has one assignee and no member list, which is the one row of the capability matrix
// where the two part company. Refused rather than ignored: a client that put somebody on an
// activity and received a 200 would believe they were on it.
func TestAnActivityHasNoMemberList(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemActivity)

	_, err := h.add.Execute(context.Background(), actor(), memberCmd())
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error = %v, want the capability refusal", err)
	}
	if code := shared.AsError(err).DetailCode; code != "items.capability_not_supported" {
		t.Errorf("detail code %q", code)
	}
}

// The capability before the lifecycle: "an activity has no member list" is true of the type
// whatever state one activity is in, and answering with the state first would send a client off to
// unarchive an entry whose members would still be refused.
func TestTheMemberCapabilityIsReportedBeforeTheLifecycle(t *testing.T) {
	h := newItemMemberHarness(t)
	item := h.withItem(domain.ItemActivity)
	trashedAt := now
	item.DeletedAt = &trashedAt
	h.items.stored[assignedItem] = item

	_, err := h.add.Execute(context.Background(), actor(), memberCmd())
	if code := shared.AsError(err).DetailCode; code != "items.capability_not_supported" {
		t.Errorf("a trashed activity answered %q", code)
	}
}

func TestAddingAMemberToATrashedEntryIsRefused(t *testing.T) {
	h := newItemMemberHarness(t)
	item := h.withItem(domain.ItemTask)
	trashedAt := now
	item.DeletedAt = &trashedAt
	h.items.stored[assignedItem] = item

	_, err := h.add.Execute(context.Background(), actor(), memberCmd())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want a conflict", err)
	}
}

// Both directions travel through the catalogue, so the untyped channel has to reach the same
// command the typed call does - and answer in the contract's words.
func TestTheMemberChannelsReachTheSameCommand(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	out, err := h.add.Descriptor().Handler.Invoke(ctx, actor(), map[string]any{
		"item_id": assignedItem.String(), "account_id": assigneeID.String(),
	})
	if err != nil {
		t.Fatalf("the catalogue refused the call: %v", err)
	}
	ids, _ := out["member_ids"].([]string)
	if len(ids) != 1 || ids[0] != assigneeID.String() {
		t.Errorf("member_ids is %v", out["member_ids"])
	}

	out, err = h.remove.Descriptor().Handler.Invoke(ctx, actor(), map[string]any{
		"item_id": assignedItem.String(), "account_id": assigneeID.String(),
	})
	if err != nil {
		t.Fatalf("the catalogue refused the removal: %v", err)
	}
	if ids, _ := out["member_ids"].([]string); len(ids) != 0 {
		t.Errorf("member_ids is %v, want an empty list", out["member_ids"])
	}
}

// Two devices adding two different people converge to both. That is the whole reason the set has
// tags rather than an array: last writer wins over the array would keep one of the two.
func TestTwoMembersAddedConcurrentlyBothSurvive(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	h.visibility.reachable[strangerID] = true
	ctx := context.Background()

	if _, err := h.add.Execute(ctx, actor(), memberCmd()); err != nil {
		t.Fatalf("the first addition failed: %v", err)
	}
	second := memberCmd()
	second.AccountID = strangerID
	set, err := h.add.Execute(ctx, actor(), second)
	if err != nil {
		t.Fatalf("the second addition failed: %v", err)
	}

	if len(set.AccountIDs) != 2 {
		t.Fatalf("the entry carries %v, want both", set.AccountIDs)
	}

	elements, err := h.itemMembers.Elements(ctx, assignedItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}
	if present := domain.PresentElements(elements); len(present) != 2 {
		t.Errorf("the tags say %v is present, want both", present)
	}
}

// A concurrent add and remove of the same person resolves by tag rather than by row order: the
// later reading wins, whichever statement ran first.
func TestAnAddAndARemoveOfTheSameMemberResolveByTag(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.add.Execute(ctx, actor(), memberCmd()); err != nil {
		t.Fatalf("the addition failed: %v", err)
	}
	if _, err := h.remove.Execute(ctx, actor(), memberCmd()); err != nil {
		t.Fatalf("the removal failed: %v", err)
	}

	elements, err := h.itemMembers.Elements(ctx, assignedItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}
	if len(elements) != 1 {
		t.Fatalf("%d tagged elements, want 1", len(elements))
	}
	// Both tags are kept. A removal that erased the addition would make a re-add on another device
	// indistinguishable from an element that had never been added, and the two merge differently.
	if elements[0].AddedAt.IsZero() || elements[0].RemovedAt.IsZero() {
		t.Fatalf("one of the tags was dropped: %+v", elements[0])
	}
	if elements[0].IsPresent() {
		t.Error("the later removal did not win")
	}
}

func TestAMemberCommandWithoutAnAccountIsRefusedByName(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)

	_, err := h.add.Execute(context.Background(), actor(), MemberCommand{ItemID: assignedItem})
	if code := shared.AsError(err).DetailCode; code != "items.account_id_required" {
		t.Fatalf("detail code %q", code)
	}
}
