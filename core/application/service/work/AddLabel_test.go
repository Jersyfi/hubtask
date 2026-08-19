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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var labelledItem = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")

// labelProfiles is the shared fixture with LABELS put back where the capability matrix grants it.
//
// systemProfiles is deliberately narrower than the seed - it is what the placement tests need, and
// a fixture that listed every capability would make those tests prove nothing. These tests are
// about the capability itself, so they read the matrix as db/migrations/0002 seeds it: a task and a
// work package carry labels, an activity does not (domain-model.md §2).
func labelProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type == domain.ItemTask || row.Type == domain.ItemWorkPackage {
			rows[i].Capabilities = append(row.Capabilities, domain.CapabilityLabels)
		}
	}
	return rows
}

// itemLabels is the fake membership and its OR-set tags. It keeps both, because writing one without
// the other is the failure the port exists to prevent: a membership with no tag merges as last
// writer wins and loses a concurrent change.
type itemLabels struct {
	carried  map[shared.ID][]shared.ID
	elements map[shared.ID]map[shared.ID]domain.SetElement

	listErr   error
	addErr    error
	removeErr error
}

func newItemLabels() *itemLabels {
	return &itemLabels{
		carried:  map[shared.ID][]shared.ID{},
		elements: map[shared.ID]map[shared.ID]domain.SetElement{},
	}
}

func (l *itemLabels) List(_ context.Context, itemID shared.ID) ([]shared.ID, error) {
	if l.listErr != nil {
		return nil, l.listErr
	}
	return slices.Clone(l.carried[itemID]), nil
}

func (l *itemLabels) Add(
	_ context.Context, itemID, labelID shared.ID, tag shared.HLC,
) error {
	if l.addErr != nil {
		return l.addErr
	}
	if !slices.Contains(l.carried[itemID], labelID) {
		l.carried[itemID] = append(l.carried[itemID], labelID)
	}
	l.tag(itemID, labelID, func(element *domain.SetElement) {
		element.AddedAt, element.RemovedAt = tag, shared.HLC{}
	})
	return nil
}

func (l *itemLabels) Remove(
	_ context.Context, itemID, labelID shared.ID, tag shared.HLC,
) (bool, error) {
	if l.removeErr != nil {
		return false, l.removeErr
	}
	carried := slices.Contains(l.carried[itemID], labelID)
	l.carried[itemID] = slices.DeleteFunc(l.carried[itemID], func(id shared.ID) bool {
		return id == labelID
	})
	l.tag(itemID, labelID, func(element *domain.SetElement) { element.RemovedAt = tag })
	return carried, nil
}

func (l *itemLabels) Elements(_ context.Context, itemID shared.ID) ([]domain.SetElement, error) {
	elements := make([]domain.SetElement, 0, len(l.elements[itemID]))
	for _, element := range l.elements[itemID] {
		elements = append(elements, element)
	}
	return elements, nil
}

func (l *itemLabels) tag(itemID, labelID shared.ID, apply func(*domain.SetElement)) {
	if l.elements[itemID] == nil {
		l.elements[itemID] = map[shared.ID]domain.SetElement{}
	}
	element := l.elements[itemID][labelID]
	element.ElementID = labelID
	apply(&element)
	l.elements[itemID][labelID] = element
}

var _ repository.ItemLabels = (*itemLabels)(nil)

type itemLabelHarness struct {
	add        AddLabel
	remove     RemoveLabel
	items      *items
	itemLabels *itemLabels
	labels     *labels
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newItemLabelHarness(t *testing.T) *itemLabelHarness {
	t.Helper()

	h := &itemLabelHarness{
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		itemLabels: newItemLabels(),
		labels:     &labels{stored: map[shared.ID]domain.Label{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		events:     &events{}, changes: &changes{}, audit: &sink{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}

	writer := ItemLabelWriter{
		Items: h.items, ItemLabels: h.itemLabels, Labels: h.labels, Containers: h.containers,
		Profiles: &profiles{rows: labelProfiles()}, Authorizer: h.authorizer, Events: h.events,
		Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.add = AddLabel{Writer: writer}
	h.remove = RemoveLabel{Writer: writer}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.labels.stored[urgentLabel] = domain.Label{
		ID: urgentLabel, TenantID: tenantID, CollectionID: collectionID,
		Name: "Urgent", ColorToken: "accent.red", Version: 1,
	}
	return h
}

func (h *itemLabelHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: labelledItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(labelledItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[labelledItem] = item
	return item
}

func labelCmd() LabelCommand {
	return LabelCommand{ItemID: labelledItem, LabelID: urgentLabel}
}

// One change owes four things, and this is the test that says so.
func TestAddingALabelWritesTheSetTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	set, err := h.add.Execute(ctx, actor(), labelCmd())
	if err != nil {
		t.Fatalf("adding the label failed: %v", err)
	}

	if set.ItemID != labelledItem || !slices.Contains(set.LabelIDs, urgentLabel) {
		t.Fatalf("the entry does not carry the label: %+v", set)
	}

	t.Run("the membership and its tag are written together", func(t *testing.T) {
		elements, err := h.itemLabels.Elements(ctx, labelledItem)
		if err != nil {
			t.Fatalf("reading the tags: %v", err)
		}
		if len(elements) != 1 || !elements[0].IsPresent() {
			t.Fatalf("the tags say the label is absent: %+v", elements)
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
		if announcement.Type != event.ItemLabelAdded {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["label_id"] != urgentLabel.String() {
			t.Errorf("the event names %v", announcement.Payload["label_id"])
		}
		if _, snapshot := announcement.Payload["title"]; snapshot {
			t.Error("the event carries an entry snapshot, which the set would already have merged")
		}
	})

	// The payload is the one element that moved, not the whole set: a payload naming the array
	// would let the later of two writers erase the other's label (offline-sync.md §4.2).
	t.Run("the change names the element and the tag decides it", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if change.Payload["set"] != string(domain.SetLabels) ||
			change.Payload["element_id"] != urgentLabel.String() ||
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

	// The label is recorded by identifier: its name is user content and stays out of the trail,
	// and an identifier is what an auditor needs in order to ask which label it was.
	t.Run("the audit entry names the label by identifier", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != ItemLabelAddedAction || entry.TargetID != labelledItem {
			t.Errorf("unexpected entry: %+v", entry)
		}
		recorded, _ := entry.Changes["label_id"].(map[string]any)
		if recorded == nil || recorded["to"] != urgentLabel.String() {
			t.Errorf("the label is not in the trail: %+v", entry.Changes)
		}
	})
}

// The permission question is the item's rather than the container's: defining a vocabulary is
// structure, tagging an entry with a word from it is work.
func TestLabellingAsksForThePermissionToWriteItems(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)

	if _, err := h.add.Execute(context.Background(), actor(), labelCmd()); err != nil {
		t.Fatalf("adding the label failed: %v", err)
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

// Adding a label an entry already carries succeeds and announces nothing - but the tag moves
// forward all the same, so that a concurrent removal on another device does not win a merge it
// should not.
func TestAddingALabelTwiceAnnouncesNothingAndMovesTheTag(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.add.Execute(ctx, actor(), labelCmd()); err != nil {
		t.Fatalf("the first addition failed: %v", err)
	}
	first, err := h.itemLabels.Elements(ctx, labelledItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}

	set, err := h.add.Execute(ctx, actor(), labelCmd())
	if err != nil {
		t.Fatalf("the second addition failed: %v", err)
	}

	if len(set.LabelIDs) != 1 {
		t.Errorf("the label is carried twice: %+v", set)
	}
	if len(h.events.appended) != 1 || len(h.audit.entries) != 1 {
		t.Error("the second addition announced something")
	}

	second, err := h.itemLabels.Elements(ctx, labelledItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}
	if !second[0].AddedAt.After(first[0].AddedAt) {
		t.Error("the tag did not move, so a concurrent removal would win a merge it should not")
	}
}

// A removal is recorded whether or not the entry carried the label: a device that removes something
// this replica never saw added has still made a decision another replica has to merge against.
func TestRemovingALabelTheEntryDoesNotCarryStillRecordsTheTag(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	set, err := h.remove.Execute(ctx, actor(), labelCmd())
	if err != nil {
		t.Fatalf("removing the label failed: %v", err)
	}
	if len(set.LabelIDs) != 0 {
		t.Errorf("the entry carries %+v", set.LabelIDs)
	}
	if len(h.events.appended) != 0 {
		t.Error("a removal that removed nothing announced something")
	}

	elements, err := h.itemLabels.Elements(ctx, labelledItem)
	if err != nil {
		t.Fatalf("reading the tags: %v", err)
	}
	if len(elements) != 1 || elements[0].RemovedAt.IsZero() {
		t.Fatalf("the removal was not recorded: %+v", elements)
	}
}

func TestRemovingALabelTheEntryCarries(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.add.Execute(ctx, actor(), labelCmd()); err != nil {
		t.Fatalf("adding the label failed: %v", err)
	}

	set, err := h.remove.Execute(ctx, actor(), labelCmd())
	if err != nil {
		t.Fatalf("removing the label failed: %v", err)
	}
	if len(set.LabelIDs) != 0 {
		t.Fatalf("the entry still carries %+v", set.LabelIDs)
	}
	if len(h.events.appended) != 2 || h.events.appended[1].Type != event.ItemLabelRemoved {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if h.changes.recorded[1].Payload["op"] != "remove" {
		t.Errorf("the change does not say what happened: %+v", h.changes.recorded[1].Payload)
	}
}

// The capability matrix: an activity has no labels, and setting a field whose capability is not
// active is refused rather than ignored (domain-model.md §2).
func TestAnActivityHasNoLabels(t *testing.T) {
	h := newItemLabelHarness(t)
	item := h.withItem(domain.ItemActivity)
	item.ParentID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")
	h.items.stored[labelledItem] = item

	_, err := h.add.Execute(context.Background(), actor(), labelCmd())
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("an activity was labelled: %v", err)
	}
	if shared.AsError(err).DetailCode != "items.capability_not_supported" {
		t.Errorf("detail code %s", shared.AsError(err).DetailCode)
	}
	if len(h.itemLabels.carried[labelledItem]) != 0 {
		t.Error("the label was written anyway")
	}
}

// The capability is asked before the lifecycle: "an activity has no labels" is true of the type
// whatever state one particular activity is in, and answering with the state first would send a
// client off to unarchive an entry whose labels would still be refused.
func TestTheCapabilityIsAskedBeforeTheLifecycle(t *testing.T) {
	h := newItemLabelHarness(t)
	item := h.withItem(domain.ItemActivity)
	item.ParentID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")
	archivedAt := now
	item.ArchivedAt = &archivedAt
	h.items.stored[labelledItem] = item

	_, err := h.add.Execute(context.Background(), actor(), labelCmd())
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("the state was reported before the capability: %v", err)
	}
}

// I-W4: a trashed or archived entry is read-only.
func TestAnArchivedEntryCannotBeLabelled(t *testing.T) {
	h := newItemLabelHarness(t)
	item := h.withItem(domain.ItemTask)
	archivedAt := now
	item.ArchivedAt = &archivedAt
	h.items.stored[labelledItem] = item

	_, err := h.add.Execute(context.Background(), actor(), labelCmd())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an archived entry was labelled: %v", err)
	}
}

// Invariant I-W6: a label is a vocabulary one collection agreed on, and one from elsewhere would be
// a reference that resolves to a word nobody in this collection chose.
func TestALabelFromAnotherCollectionIsRefused(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)

	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000c9")
	h.labels.stored[elsewhere] = domain.Label{
		ID: elsewhere, TenantID: tenantID,
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-0000000000cf"),
		Name:         "Elsewhere", ColorToken: "accent.blue", Version: 1,
	}

	_, err := h.add.Execute(context.Background(), actor(), LabelCommand{
		ItemID: labelledItem, LabelID: elsewhere,
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a foreign label was accepted: %v", err)
	}
	if shared.AsError(err).DetailCode != "labels.not_in_collection" {
		t.Errorf("detail code %s", shared.AsError(err).DetailCode)
	}
}

// A deleted label is out of the vocabulary, and tagging an entry with it would be tagging it with
// something no read will ever show.
func TestADeletedLabelCannotBeAdded(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)

	label := h.labels.stored[urgentLabel]
	deleted, _, err := label.Deleted(now)
	if err != nil {
		t.Fatalf("deleting the label: %v", err)
	}
	h.labels.stored[urgentLabel] = deleted

	_, err = h.add.Execute(context.Background(), actor(), labelCmd())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a deleted label was added: %v", err)
	}
}

func TestLabellingNeedsBothIdentifiers(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)

	for _, c := range []struct {
		name       string
		cmd        LabelCommand
		detailCode string
	}{
		{
			name:       "no entry",
			cmd:        LabelCommand{LabelID: urgentLabel},
			detailCode: "items.item_id_required",
		},
		{
			name:       "no label",
			cmd:        LabelCommand{ItemID: labelledItem},
			detailCode: "labels.label_id_required",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.add.Execute(context.Background(), actor(), c.cmd)
			if shared.AsError(err).DetailCode != c.detailCode {
				t.Fatalf("detail code %s, want %s", shared.AsError(err).DetailCode, c.detailCode)
			}
		})
	}
}

func TestARefusedLabellingWritesNothing(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)
	h.authorizer.err = shared.ErrForbidden

	_, err := h.add.Execute(context.Background(), actor(), labelCmd())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if len(h.itemLabels.carried[labelledItem]) != 0 || h.uow.writes != 0 {
		t.Error("a refused labelling wrote something")
	}
}

func TestTheItemLabelDescriptorsCarryWhatTheChannelsNeed(t *testing.T) {
	add := AddLabel{}.Descriptor()
	if add.Name != AddLabelName || add.Audit.Action != ItemLabelAddedAction || !add.Audit.Required {
		t.Errorf("unexpected descriptor: %+v", add)
	}

	remove := RemoveLabel{}.Descriptor()
	if remove.Audit.Action != ItemLabelRemovedAction {
		t.Errorf("the removal writes the wrong audit action: %s", remove.Audit.Action)
	}
}

// The catalogue's answer is the set the entry now carries, so that every channel reports it alike.
func TestTheLabelSetOutputNamesTheEntryAndItsLabels(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)

	out, err := h.add.invoke(context.Background(), actor(), map[string]any{
		"item_id": labelledItem.String(), "label_id": urgentLabel.String(),
	})
	if err != nil {
		t.Fatalf("adding the label failed: %v", err)
	}

	if out["item_id"] != labelledItem.String() {
		t.Errorf("item_id is %v", out["item_id"])
	}
	ids, _ := out["label_ids"].([]string)
	if len(ids) != 1 || ids[0] != urgentLabel.String() {
		t.Errorf("label_ids is %v", out["label_ids"])
	}
}
