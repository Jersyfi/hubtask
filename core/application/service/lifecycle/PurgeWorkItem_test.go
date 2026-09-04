// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The two doubles the use cases need beyond the engine's: the entry and the collection they read,
// and the questions they ask of the authorisation service.

type itemStore struct {
	stored map[shared.ID]work.WorkItem
}

func (s *itemStore) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, found := s.stored[id]
	if !found {
		return work.WorkItem{}, shared.ErrNotFound
	}
	return item, nil
}

func (s *itemStore) List(context.Context, workrepo.ItemQuery) (workrepo.ItemPage, error) {
	return workrepo.ItemPage{}, nil
}
func (s *itemStore) ChildCompletion(context.Context, shared.ID) (work.ChildCompletion, error) {
	return work.ChildCompletion{}, nil
}
func (s *itemStore) SetCompletion(context.Context, work.WorkItem, int) error { return nil }
func (s *itemStore) SetAttributes(context.Context, work.WorkItem, int) error { return nil }
func (s *itemStore) Neighbours(
	context.Context, workrepo.Level, shared.ID, shared.ID,
) (string, string, error) {
	return "", "", nil
}
func (s *itemStore) SetOrderKey(context.Context, work.WorkItem, int) error { return nil }
func (s *itemStore) SetAssignee(context.Context, work.WorkItem, int) error { return nil }
func (s *itemStore) CountOpenByAssignee(context.Context, []shared.ID) (map[shared.ID]int, error) {
	return nil, nil
}
func (s *itemStore) SetCustomField(
	context.Context, work.WorkItem, string, shared.ID, int,
) error {
	return nil
}
func (s *itemStore) SetCover(context.Context, work.WorkItem, int) error { return nil }

func (s *itemStore) SetDueDate(context.Context, work.WorkItem, int) error { return nil }
func (s *itemStore) MoveSubtree(context.Context, workrepo.Move) (int, []work.DroppedReference, error) {
	return 0, nil, nil
}
func (s *itemStore) LastOrderKey(context.Context, shared.ID, shared.ID) (string, error) {
	return "", nil
}
func (s *itemStore) Insert(context.Context, work.WorkItem) error { return nil }
func (s *itemStore) Subtree(context.Context, work.WorkItem, int) ([]work.WorkItem, error) {
	return nil, nil
}
func (s *itemStore) InsertCopy(context.Context, workrepo.Copy) error       { return nil }
func (s *itemStore) SetArchived(context.Context, work.WorkItem, int) error { return nil }
func (s *itemStore) TrashSubtree(context.Context, workrepo.ItemTrash) (int, error) {
	return 0, nil
}
func (s *itemStore) RestoreBatch(context.Context, workrepo.ItemTrash) (int, error) {
	return 0, nil
}

func (s *itemStore) Query(
	context.Context, workrepo.ItemSearch,
) (workrepo.ItemQueryResult, error) {
	return workrepo.ItemQueryResult{}, nil
}
func (s *itemStore) Search(
	context.Context, workrepo.TextSearch,
) (workrepo.ItemHitPage, error) {
	return workrepo.ItemHitPage{}, nil
}

type containerStore struct{ stored map[shared.ID]work.Container }

func (s *containerStore) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, found := s.stored[id]
	if !found {
		return work.Container{}, shared.ErrNotFound
	}
	return container, nil
}

func (s *containerStore) List(context.Context, workrepo.ContainerQuery) (workrepo.ContainerPage, error) {
	return workrepo.ContainerPage{}, nil
}
func (s *containerStore) LastOrderKey(context.Context, shared.ID) (string, error)  { return "", nil }
func (s *containerStore) Insert(context.Context, work.Container) error             { return nil }
func (s *containerStore) SetAttributes(context.Context, work.Container, int) error { return nil }
func (s *containerStore) SetPolicies(context.Context, work.Container, int) error   { return nil }
func (s *containerStore) SetArchived(context.Context, work.Container, int) error   { return nil }
func (s *containerStore) TrashSubtree(context.Context, workrepo.ContainerTrash) (workrepo.Cascade, error) {
	return workrepo.Cascade{}, nil
}
func (s *containerStore) RestoreBatch(context.Context, workrepo.ContainerTrash) (workrepo.Cascade, error) {
	return workrepo.Cascade{}, nil
}
func (s *containerStore) SetPlacement(context.Context, work.Container, int) error { return nil }
func (s *containerStore) SetRank(context.Context, work.Container, int) error      { return nil }
func (s *containerStore) Neighbours(
	context.Context, shared.ID, shared.ID, shared.ID,
) (string, string, error) {
	return "", "", nil
}

type authorizerDouble struct {
	err      error
	requests []access.Request
}

func (a *authorizerDouble) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	return a.err
}

type unitOfWork struct{ writes, reads int }

func (u *unitOfWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	u.writes++
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	u.reads++
	return fn(ctx)
}

type purgeHarness struct {
	*harness
	items      *itemStore
	containers *containerStore
	authorizer *authorizerDouble
	uow        *unitOfWork
}

func newPurgeHarness() *purgeHarness {
	base := newHarness()
	h := &purgeHarness{
		harness:    base,
		items:      &itemStore{stored: map[shared.ID]work.WorkItem{}},
		containers: &containerStore{stored: map[shared.ID]work.Container{}},
		authorizer: &authorizerDouble{},
		uow:        &unitOfWork{},
	}
	h.containers.stored[collectionID] = work.Container{
		ID: collectionID, TenantID: tenantID, Type: work.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", Version: 1,
	}
	h.items.stored[taskID] = trashedTask()
	return h
}

func (h *purgeHarness) purge() PurgeWorkItem {
	return PurgeWorkItem{
		Items: h.items, Containers: h.containers, Purger: h.purger,
		Authorizer: h.authorizer, UnitOfWork: h.uow,
	}
}

func (h *purgeHarness) empty() EmptyTrash {
	return EmptyTrash{Purger: h.purger, Authorizer: h.authorizer, UnitOfWork: h.uow}
}

// The named purge: the entry goes, the records are written, and the trail gets one summary rather
// than an entry per row (data-retention.md §5).
func TestPurgingANamedEntryRemovesItAndRecordsASummary(t *testing.T) {
	h := newPurgeHarness()
	h.trash.subtree = []shared.ID{taskID, packageID}

	removed, err := h.purge().Execute(
		t.Context(), actor(), PurgeWorkItemCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("the purge was refused: %v", err)
	}

	if removed != 2 {
		t.Errorf("%d rows removed, want 2", removed)
	}
	if len(h.removals.recorded) != 2 {
		t.Errorf("%d removals recorded, want 2", len(h.removals.recorded))
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemPurgedAction {
		t.Fatalf("the audit entries are %v, want one %s", h.audit.entries, ItemPurgedAction)
	}
	if !h.uow.committed() {
		t.Error("the purge did not open a write transaction")
	}
}

func (u *unitOfWork) committed() bool { return u.writes > 0 }

// The right to write entries, not the owner's right to delete a container: the trash already
// accepted this deletion under that right, and purging only skips the waiting.
func TestPurgingAnEntryAsksToWriteEntries(t *testing.T) {
	h := newPurgeHarness()
	h.trash.subtree = []shared.ID{taskID}

	if _, err := h.purge().Execute(
		t.Context(), actor(), PurgeWorkItemCommand{ItemID: taskID}); err != nil {
		t.Fatalf("the purge was refused: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionWriteItems {
		t.Errorf("permission %s, want %s", request.Permission, service.PermissionWriteItems)
	}
	if request.Action != ItemPurgedAction {
		t.Errorf("action %s, want %s", request.Action, ItemPurgedAction)
	}
	if len(request.Path) != 3 {
		t.Errorf("the path is %v, want the tenant, the hub and the collection", request.Path)
	}
}

// A refusal writes nothing, and the audit entry it produces lives outside the transaction so that it
// survives the rollback (audit.md §7).
func TestARefusedPurgeRemovesNothing(t *testing.T) {
	h := newPurgeHarness()
	h.trash.subtree = []shared.ID{taskID}
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := h.purge().Execute(t.Context(), actor(), PurgeWorkItemCommand{ItemID: taskID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal reported %v", err)
	}
	if len(h.trash.purgedItem) != 0 || len(h.removals.recorded) != 0 {
		t.Error("a refused purge removed or recorded something")
	}
	if h.uow.writes != 0 {
		t.Error("a refused purge opened a write transaction")
	}
}

// Only out of the trash. A live entry is deleted by deleting it, which is reversible - an endpoint
// that removed a live entry for good would make the trash something a caller could skip.
func TestPurgingALiveEntryIsRefused(t *testing.T) {
	h := newPurgeHarness()
	live := trashedTask()
	live.DeletedAt, live.TrashBatchID = nil, ""
	h.items.stored[taskID] = live

	_, err := h.purge().Execute(t.Context(), actor(), PurgeWorkItemCommand{ItemID: taskID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("purging a live entry reported %v, want a conflict", err)
	}
	if detail := shared.AsError(err).DetailCode; detail != "items.not_trashed" {
		t.Errorf("the detail code is %q, want items.not_trashed", detail)
	}
}

func TestPurgingAnEntryThatIsNotThereSaysSo(t *testing.T) {
	h := newPurgeHarness()

	_, err := h.purge().Execute(t.Context(), actor(), PurgeWorkItemCommand{ItemID: otherTaskID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("purging a missing entry reported %v", err)
	}

	if _, err := h.purge().Execute(
		t.Context(), actor(), PurgeWorkItemCommand{}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("purging without an identifier reported %v", err)
	}
}

// Emptying a trash reaches hubs and collections as readily as entries, so it asks the owner's right
// to delete a container - somebody who may not delete a hub must not be able to delete one by
// emptying a trash it happens to be in.
func TestEmptyingTheTrashAsksToDeleteAContainer(t *testing.T) {
	h := newPurgeHarness()
	longAgo := now.Add(-120 * 24 * time.Hour)
	h.expired.items = []repository.ExpiredItem{expiredItem(taskID, longAgo)}
	h.expired.containers = []repository.ExpiredContainer{
		{ID: collectionID, Type: work.ContainerCollection, ParentID: hubID, DeletedAt: longAgo},
	}

	outcome, err := h.empty().Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("emptying the trash was refused: %v", err)
	}

	if outcome.Removed != 2 {
		t.Errorf("removed %d, want 2", outcome.Removed)
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionDeleteContainer {
		t.Errorf("permission %s, want %s", request.Permission, service.PermissionDeleteContainer)
	}
	if len(request.Path) != 1 {
		t.Errorf("the path is %v, want the tenant alone", request.Path)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != TrashEmptiedAction {
		t.Errorf("the audit entries are %v, want one %s", h.audit.entries, TrashEmptiedAction)
	}
}

// A hold does not fail the call: it is counted, so that emptying a trash with one held entry in it
// still empties the rest.
func TestEmptyingTheTrashKeepsWhatIsHeldAndSaysSo(t *testing.T) {
	h := newPurgeHarness()
	longAgo := now.Add(-120 * 24 * time.Hour)
	h.expired.items = []repository.ExpiredItem{
		expiredItem(taskID, longAgo), expiredItem(otherTaskID, longAgo),
	}
	h.holds.holds = domain.Holds{
		{ID: holdID, Scope: domain.HoldItem, ScopeID: otherTaskID, Reason: "Litigation"},
	}

	outcome, err := h.empty().Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("emptying the trash was refused: %v", err)
	}

	if outcome.Removed != 1 || outcome.Blocked[BlockedByLegalHold] != 1 {
		t.Errorf("the pass reports %+v, want one removed and one held", outcome)
	}

	out, err := h.empty().Descriptor().Handler.Invoke(t.Context(), actor(), nil)
	if err != nil {
		t.Fatalf("invoking through the catalogue: %v", err)
	}
	blocked, _ := out["blocked"].(map[string]any)
	if blocked[BlockedByLegalHold] != 1 {
		t.Errorf("the answer says %v was held, want 1", blocked[BlockedByLegalHold])
	}
}

// Both are declared destructive, so an agent client asks before calling either (ai-first.md §1.1).
func TestBothPurgeVerbsAreDeclaredDestructive(t *testing.T) {
	if !(PurgeWorkItem{}).Descriptor().Destructive {
		t.Error("PurgeWorkItem is not declared destructive")
	}
	if !(EmptyTrash{}).Descriptor().Destructive {
		t.Error("EmptyTrash is not declared destructive")
	}
}
