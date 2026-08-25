// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
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
	inserted []domain.WorkItem
	stored   map[shared.ID]domain.WorkItem
	lastKey  string
	// page is what List answers with, and asked is what it was asked (B-04's read side).
	page  repository.ItemPage
	asked repository.ItemQuery
	// result is what Query answers with, and searched is every specification it was handed
	// (B-12's query language).
	result   repository.ItemQueryResult
	searched []repository.ItemSearch
	queryErr error
	// hits is what Search answers with, and searchedText is every request it was handed (C-08's
	// full text search).
	hits         repository.ItemHitPage
	searchedText []repository.TextSearch
	searchErr    error
	// children is what ChildCompletion answers, per parent, and completions records every write
	// SetCompletion took - the roll-up tests care about both: one is the state it decided from, the
	// other is what it decided.
	children    map[shared.ID]domain.ChildCompletion
	completions []completionWrite
	// load is what CountOpenByAssignee answers, per account (C-02's LEAST_LOADED material).
	load map[shared.ID]int
	// attributes records every write SetAttributes took: the B-05 tests care about what was stored as
	// much as about what came back, because a use case that returned the right item and wrote the wrong
	// one would pass every assertion made on the answer alone.
	attributes []attributeWrite
	findErr    error
	listErr    error
	insertErr  error
	setErr     error
	conflictOn shared.ID
	// The move and reorder fakes: what the neighbours answer, and what each write was asked to store. The
	// B-08 tests care about both - one is the position the ordering service measured against, the other is
	// where the item ended up.
	previousKey string
	nextKey     string
	askedLevel  repository.Level
	askedBefore shared.ID
	ranks       []rankWrite
	moves       []repository.Move
	rankErr     error
	// dropped is what MoveSubtree reports as lost, for the tests about I-W6.
	dropped     []domain.DroppedReference
	moveErr     error
	subtreeSize int
	// The trash side (B-10): what each call was asked to do, and the stamps it left on `stored`.
	// Both are needed - one proves the use case passed the batch and the version it read, the other
	// is what a second pass over the same item reads, which is what makes an idempotence test mean
	// anything.
	trashed  []repository.ItemTrash
	restored []repository.ItemTrash
	trashErr error
	// The assignment side (C-01): every call to SetAssignee, so that a test can say which version
	// the write was made against as well as what it wrote.
	assignments []attributeWrite
	// The cover side (C-06), same shape.
	covers []attributeWrite
	// The custom field side (C-07), same shape again.
	customFields []attributeWrite
	// The due date side (D-01), same shape again.
	dueDates []attributeWrite
	// The copy side (C-11): every call to InsertCopy, and the failure a test asks the subtree read
	// for.
	copies     []repository.Copy
	subtreeErr error
}

// rankWrite is one call to SetOrderKey: the item as it would be stored, and the version it was written
// against.
type rankWrite struct {
	item            domain.WorkItem
	expectedVersion int
}

func (i *items) Neighbours(
	_ context.Context, level repository.Level, beforeID, _ shared.ID,
) (string, string, error) {
	i.askedLevel, i.askedBefore = level, beforeID
	return i.previousKey, i.nextKey, nil
}

func (i *items) SetOrderKey(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if i.rankErr != nil {
		return i.rankErr
	}
	i.ranks = append(i.ranks, rankWrite{item: item, expectedVersion: expectedVersion})
	i.stored[item.ID] = item
	return nil
}

// CountOpenByAssignee answers from the load map, absent accounts omitted as the real query omits
// them. The C-02 tests fill it; everything else never asks.
func (i *items) CountOpenByAssignee(
	_ context.Context, accounts []shared.ID,
) (map[shared.ID]int, error) {
	counts := make(map[shared.ID]int)
	for _, account := range accounts {
		if load, carries := i.load[account]; carries {
			counts[account] = load
		}
	}
	return counts, nil
}

// SetCover mirrors SetAssignee: the same optimistic lock, the same version bump on the stored
// row, recorded in covers so the C-06 tests can say what was written.
func (i *items) SetCover(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if item.ID == i.conflictOn || i.stored[item.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.covers = append(i.covers, attributeWrite{item: item, expectedVersion: expectedVersion})
	written := item
	written.Version = expectedVersion + 1
	i.stored[item.ID] = written
	return nil
}

// SetCustomField mirrors SetCover: the same optimistic lock, the same version bump on the stored
// row, recorded in customFields so the C-07 tests can say what was written. The fake stores the
// whole wanted state, which is what the real adapter's per-key write converges the row to.
func (i *items) SetCustomField(
	_ context.Context, item domain.WorkItem, _ string, _ shared.ID, expectedVersion int,
) error {
	if item.ID == i.conflictOn || i.stored[item.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.customFields = append(i.customFields, attributeWrite{item: item, expectedVersion: expectedVersion})
	written := item
	written.Version = expectedVersion + 1
	i.stored[item.ID] = written
	return nil
}

// SetDueDate mirrors SetCover: the same optimistic lock, the same version bump on the stored
// row, recorded in dueDates so the D-01 tests can say what was written.
func (i *items) SetDueDate(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if item.ID == i.conflictOn || i.stored[item.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.dueDates = append(i.dueDates, attributeWrite{item: item, expectedVersion: expectedVersion})
	written := item
	written.Version = expectedVersion + 1
	i.stored[item.ID] = written
	return nil
}

func (i *items) SetAssignee(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if i.setErr != nil {
		return i.setErr
	}
	// The same optimistic lock SetAttributes models, and for the same reason: a use case that
	// passed the wrong version would otherwise look correct at this level.
	if item.ID == i.conflictOn || i.stored[item.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.assignments = append(i.assignments, attributeWrite{item: item, expectedVersion: expectedVersion})
	// Stored with the version the statement behind it produces: `version = version + 1` runs in the
	// database, so a second pass over the same entry reads the version the first pass left. Without
	// that, an idempotence test and an If-Match test would both be reading a row that never moved.
	written := item
	written.Version = expectedVersion + 1
	i.stored[item.ID] = written
	return nil
}

func (i *items) MoveSubtree(
	_ context.Context, move repository.Move,
) (int, []domain.DroppedReference, error) {
	if i.moveErr != nil {
		return 0, nil, i.moveErr
	}
	i.moves = append(i.moves, move)

	moved := i.stored[move.Item.ID]
	moved.ParentID = move.TargetParentID
	moved.CollectionID = move.CollectionID
	moved.Path = move.NewPrefix
	moved.Depth += move.DepthDelta
	moved.OrderKey = move.OrderKey
	moved.BucketID = move.BucketID
	moved.UpdatedAt = move.UpdatedAt
	i.stored[move.Item.ID] = moved

	// What the destination could not resolve. Set by a test that wants a loss reported; the
	// repository decides it for real, out of the two statements that clear the subtree (I-W6).
	size := i.subtreeSize
	if size == 0 {
		size = 1
	}
	return size, i.dropped, nil
}

func (i *items) Find(_ context.Context, id shared.ID) (domain.WorkItem, error) {
	if i.findErr != nil {
		return domain.WorkItem{}, i.findErr
	}
	item, found := i.stored[id]
	if !found {
		return domain.WorkItem{}, shared.ErrNotFound
	}
	return item, nil
}

func (i *items) List(_ context.Context, query repository.ItemQuery) (repository.ItemPage, error) {
	i.asked = query
	if i.listErr != nil {
		return repository.ItemPage{}, i.listErr
	}
	return i.page, nil
}

// completionWrite is one call to SetCompletion: what it was asked to store and against which version.
type completionWrite struct {
	item            domain.WorkItem
	expectedVersion int
}

func (i *items) ChildCompletion(_ context.Context, parentID shared.ID) (domain.ChildCompletion, error) {
	return i.children[parentID], nil
}

func (i *items) SetCompletion(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if i.setErr != nil {
		return i.setErr
	}
	if item.ID == i.conflictOn {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.completions = append(i.completions, completionWrite{item: item, expectedVersion: expectedVersion})
	// Kept, so that a second pass over the same item reads the state the first pass wrote - which is what
	// makes an idempotence test mean anything.
	i.stored[item.ID] = item
	return nil
}

// attributeWrite is one call to SetAttributes: what it was asked to store and against which version.
type attributeWrite struct {
	item            domain.WorkItem
	expectedVersion int
}

func (i *items) SetAttributes(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if i.setErr != nil {
		return i.setErr
	}
	// The optimistic lock, as the statement behind it works: the update matches nothing when the row has
	// moved on, and the caller is told rather than overwriting whoever moved it. Modelled here rather
	// than only in the integration test, because a use case that passed the wrong version would
	// otherwise look correct at this level.
	if item.ID == i.conflictOn || i.stored[item.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.attributes = append(i.attributes, attributeWrite{item: item, expectedVersion: expectedVersion})
	// Stored with the version the statement behind it produces, as SetArchived models it: the due
	// write of the same patch goes against the version this write left (D-01).
	written := item
	written.Version = expectedVersion + 1
	i.stored[item.ID] = written
	return nil
}

// SetArchived writes the stamp and moves the version on, as `version = version + 1` in the statement
// does. The stored version has to move: a use case that archived twice would otherwise read the
// pre-write version on its second pass, and an idempotence test would be measuring the fake.
func (i *items) SetArchived(_ context.Context, item domain.WorkItem, expectedVersion int) error {
	if i.setErr != nil {
		return i.setErr
	}
	if item.ID == i.conflictOn || i.stored[item.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.attributes = append(i.attributes, attributeWrite{item: item, expectedVersion: expectedVersion})
	item.Version = expectedVersion + 1
	i.stored[item.ID] = item
	return nil
}

// TrashSubtree stamps the item and everything below it, the way the statement behind it does -
// including the part that matters: a row already in the trash keeps its own deletion and is not
// counted (I-C2).
func (i *items) TrashSubtree(_ context.Context, trash repository.ItemTrash) (int, error) {
	if i.trashErr != nil {
		return 0, i.trashErr
	}
	if trash.Item.ID == i.conflictOn || i.stored[trash.Item.ID].Version != trash.ExpectedVersion {
		return 0, shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.trashed = append(i.trashed, trash)
	stamped := trash.Item
	stamped.Version = trash.ExpectedVersion + 1
	i.stored[trash.Item.ID] = stamped

	moved := 1
	for id, stored := range i.stored {
		if id == trash.Item.ID || stored.IsTrashed() || !strings.HasPrefix(stored.Path, trash.Prefix) {
			continue
		}
		stored.DeletedAt, stored.TrashBatchID = trash.Item.DeletedAt, trash.BatchID
		stored.Version++
		i.stored[id] = stored
		moved++
	}
	return moved, nil
}

// Query records the specification the use case compiled and answers with what the test staged.
//
// The specification is what a service test has an opinion about: whether the scope was resolved,
// whether the placeholders were replaced, whether the page was clamped. What that specification
// becomes in SQL is the adapter's business, and is proved against a real database instead.
func (i *items) Query(_ context.Context, search repository.ItemSearch) (repository.ItemQueryResult, error) {
	i.searched = append(i.searched, search)
	if i.queryErr != nil {
		return repository.ItemQueryResult{}, i.queryErr
	}
	return i.result, nil
}

// Search records the request the use case built and answers with what the test staged, for the
// reason Query does: what a search *asks for* is the use case's business - the scope it resolved,
// the narrowing it decided, the language it filled in - and what that becomes in SQL is proved
// against a real database instead.
func (i *items) Search(_ context.Context, search repository.TextSearch) (repository.ItemHitPage, error) {
	i.searchedText = append(i.searchedText, search)
	if i.searchErr != nil {
		return repository.ItemHitPage{}, i.searchErr
	}
	return i.hits, nil
}

// RestoreBatch clears the stamp on every row of one deletion, keyed on the batch rather than on the
// path - which is the whole point of the batch, and what a test about a younger deletion inside the
// same subtree measures.
func (i *items) RestoreBatch(_ context.Context, restore repository.ItemTrash) (int, error) {
	if i.trashErr != nil {
		return 0, i.trashErr
	}
	if restore.Item.ID == i.conflictOn || i.stored[restore.Item.ID].Version != restore.ExpectedVersion {
		return 0, shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	i.restored = append(i.restored, restore)
	cleared := restore.Item
	cleared.Version = restore.ExpectedVersion + 1
	i.stored[restore.Item.ID] = cleared

	moved := 1
	for id, stored := range i.stored {
		if id == restore.Item.ID || stored.TrashBatchID != restore.BatchID {
			continue
		}
		stored.DeletedAt, stored.TrashBatchID = nil, ""
		stored.Version++
		i.stored[id] = stored
		moved++
	}
	return moved, nil
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

// Subtree answers what the real statement answers: everything stored whose path begins with the
// entry's, the entry itself and the trashed rows left out, parents before children. Sorted by
// depth and then by rank, because a map iterates in no order and the copy depends on meeting a
// parent before its children (C-11).
func (i *items) Subtree(_ context.Context, item domain.WorkItem, limit int) ([]domain.WorkItem, error) {
	if i.subtreeErr != nil {
		return nil, i.subtreeErr
	}

	var below []domain.WorkItem
	for _, stored := range i.stored {
		if stored.ID == item.ID || !strings.HasPrefix(stored.Path, item.Path) || stored.IsTrashed() {
			continue
		}
		below = append(below, stored)
	}
	slices.SortFunc(below, func(a, b domain.WorkItem) int {
		if a.Depth != b.Depth {
			return a.Depth - b.Depth
		}
		if order := strings.Compare(a.OrderKey, b.OrderKey); order != 0 {
			return order
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})

	// One row beyond the bound, exactly as the statement reads one: that is how the caller tells
	// "as large as allowed" from "larger than allowed".
	if len(below) > limit+1 {
		below = below[:limit+1]
	}
	return below, nil
}

// InsertCopy records the copy and the definitions its values were written under, and stores the
// row: a test asserts on both, the written entry and what it was written to stand behind.
func (i *items) InsertCopy(_ context.Context, duplicate repository.Copy) error {
	if i.insertErr != nil {
		return i.insertErr
	}
	i.copies = append(i.copies, duplicate)
	i.stored[duplicate.Item.ID] = duplicate.Item
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
	history    *journal
	authorizer *authorizer
	uow        *unitOfWork
	visibility *visibility
	policies   *policyStore
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
		history:    &journal{},
		authorizer: &authorizer{},
		uow:        &unitOfWork{},
		visibility: newVisibility(assigneeID, accountID),
		policies:   newPolicyStore(),
	}
	h.handler = CreateWorkItem{
		Items: store, Containers: containerStore, Profiles: h.profiles,
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		Ownership:  h.authorizer,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
		// The C-02 machinery, over the same fakes: the create path is its second caller.
		AutoAssign: AutoAssignWorkItem{
			Assignment: AssignmentWriter{
				Items: store, Containers: containerStore, Profiles: h.profiles,
				Authorizer: h.authorizer, Visibility: h.visibility, Events: h.events,
				Changes: h.changes, Audit: h.audit,
				Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
				UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
			},
			Policies: h.policies,
			Groups:   &groupStore{members: map[shared.ID][]shared.ID{}},
			Random:   clock.NewScripted(0),
		},
		// The D-01 machinery, over the same fakes: the create path is its second caller.
		DueDates: DueDateWriter{
			Items: store, Containers: containerStore, Profiles: h.profiles,
			Reminders:  newReminders(),
			Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
			Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
			UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
		},
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

	task, _, err := h.handler.Execute(ctx, itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("the task: %v", err)
	}
	if task.Type != domain.ItemTask || task.Depth != 1 || !task.ParentID.IsZero() {
		t.Errorf("unexpected task: %+v", task)
	}
	if task.Title != "Buy milk" {
		t.Errorf("title = %q, want it trimmed", task.Title)
	}

	pkg, _, err := h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: collectionID, ParentID: task.ID,
		Title: "Dairy aisle",
	})
	if err != nil {
		t.Fatalf("the work package: %v", err)
	}
	if pkg.Depth != 2 || pkg.ParentID != task.ID {
		t.Errorf("unexpected work package: %+v", pkg)
	}

	leaf, _, err := h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemActivity, CollectionID: collectionID, ParentID: pkg.ID,
		Title: "Semi-skimmed, two litres",
	})
	if err != nil {
		t.Fatalf("the activity: %v", err)
	}
	if leaf.Depth != 3 || leaf.ParentID != pkg.ID {
		t.Errorf("unexpected activity: %+v", leaf)
	}

	// The subtree is what the paths say it is, and that is what every later query rests on.
	if !hasPrefix(pkg.Path, task.Path) || !hasPrefix(leaf.Path, pkg.Path) {
		t.Errorf("the paths do not nest: %q, %q, %q", task.Path, pkg.Path, leaf.Path)
	}
	if task.CollectionID != collectionID || leaf.CollectionID != collectionID {
		t.Error("the collection is not carried down the subtree")
	}
}

func hasPrefix(path, prefix string) bool {
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}

// One write owes five things, and this is the test that says so.
func TestCreatingAnItemWritesTheRowTheEventTheChangeTheEntryAndTheHistory(t *testing.T) {
	h := newItemHarness()
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	item, _, err := h.handler.Execute(ctx, itemActor(), taskCommand())
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

	// The first step of the entry's own history. No change set: nothing moved, the entry came into
	// being, and a diff of every field against nothing would be the item written out twice.
	step := h.history.only(t)
	switch {
	case step.Verb != activity.ItemCreated:
		t.Errorf("the history recorded %s, want %s", step.Verb, activity.ItemCreated)
	case step.ItemID != item.ID || step.TenantID != tenantID:
		t.Errorf("the step is about %s in %s", step.ItemID, step.TenantID)
	case step.CollectionID != collectionID:
		t.Errorf("the step is filed under %s rather than the collection", step.CollectionID)
	case step.Actor.ID != accountID || step.Actor.Kind != shared.ActorUser:
		t.Errorf("the actor reads %+v", step.Actor)
	case len(step.ChangeSet) != 0:
		t.Errorf("the change set holds %v, want nothing", step.ChangeSet)
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

	if _, _, err := h.handler.Execute(context.Background(), itemActor(), cmd); err != nil {
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

	if _, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); err != nil {
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

	_, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand())
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

			item, _, err := h.handler.Execute(context.Background(), itemActor(), cmd)
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

	pkg, _, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
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

	_, _, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
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
	if _, _, err := h.handler.Execute(context.Background(), itemActor(), withNotes); !errors.Is(
		err, shared.ErrCapabilityNotSupported) {
		t.Errorf("a note was accepted although the narrowed profile has none: %v", err)
	}

	// A type this workspace no longer offers is refused as unsupported rather than as unknown:
	// the schema still knows WORK_PACKAGE, this installation does not offer it.
	_, _, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: collectionID, Title: "Dairy",
	})
	if got := shared.AsError(err).DetailCode; got != "items.type_unsupported" {
		t.Errorf("error = %v, want items.type_unsupported", err)
	}

	// The plain case still works, so the narrowing refused what it should and nothing else.
	if _, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); err != nil {
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

	_, _, err := h.handler.Execute(context.Background(), itemActor(), CreateWorkItemCommand{
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

	item, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand())
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

	if _, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); !errors.Is(
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
	for _, owned := range []string{
		"type", "title", "collection_id", "parent_id", "notes", "bucket_id",
		"assignee_id", "auto_assign", "start_at", "due_at", "due_date_only", "due_time_zone",
	} {
		if !declared[owned] {
			t.Errorf("%s is not declared", owned)
		}
	}
	for _, later := range []string{"label_ids", "member_ids", "cover"} {
		if declared[later] {
			t.Errorf("%s is declared, though no use case writes it yet", later)
		}
	}

	if err := descriptor.ValidateInput(map[string]any{
		"type": "TASK", "title": "Buy milk",
		"member_ids": []any{"0192f000-0000-7000-8000-00000000000e"},
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

// The language an entry is written in, and where it comes from when nobody says (C-08,
// i18n-l10n.md §5). The default is the creator's locale - a guess, and the honest kind: an entry
// that stated no language at all would be indexed word by word, which is the worse guess of the two.
func TestAnEntryTakesTheCreatorsLocaleWhenNoLanguageIsStated(t *testing.T) {
	h := newItemHarness()
	actor := itemActor()
	actor.Locale = "de-AT"

	item, _, err := h.handler.Execute(context.Background(), actor, taskCommand())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.ContentLanguage != "de-AT" {
		t.Errorf("the language is %q, want the creator's locale", item.ContentLanguage)
	}
}

func TestAStatedLanguageBeatsTheCreatorsLocale(t *testing.T) {
	h := newItemHarness()
	actor := itemActor()
	actor.Locale = "de-AT"

	cmd := taskCommand()
	cmd.ContentLanguage = "en"

	item, _, err := h.handler.Execute(context.Background(), actor, cmd)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.ContentLanguage != "en" {
		t.Errorf("the language is %q, want the one the client stated", item.ContentLanguage)
	}
}

// An anonymous or locale-less actor leaves the entry stating nothing, rather than inventing a
// language for it: `simple` is what an unstated language is indexed under, and claiming English
// would send a German entry through an English stemmer.
func TestAnEntryStatesNoLanguageWhenTheCreatorHasNoLocale(t *testing.T) {
	h := newItemHarness()

	item, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.ContentLanguage != "" {
		t.Errorf("the language is %q, want none", item.ContentLanguage)
	}
}
