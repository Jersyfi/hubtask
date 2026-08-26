// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	syncrepo "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// `:retain` (E-07, data-retention.md §5): the third of the three ways to take an object out, and
// the only one that says so out loud.

// changeLog is what offline clients are told.
type changeLog struct{ recorded []syncrepo.Change }

func (l *changeLog) Record(_ context.Context, change syncrepo.Change) error {
	l.recorded = append(l.recorded, change)
	return nil
}

// hlcSource stamps a change. Fixed, so a test can assert on what was written.
type hlcSource struct{ issued int }

func (h *hlcSource) Next() shared.HLC {
	h.issued++
	stamped, _ := shared.NewHLC(now, uint32(h.issued), "server")
	return stamped
}

// markedItems is the marking a `:retain` reads and clears.
type markedItems struct {
	*markingStore
	pending map[shared.ID]repository.Candidate
}

func (m *markedItems) Marking(_ context.Context, id shared.ID) (repository.Candidate, error) {
	marked, found := m.pending[id]
	if !found {
		return repository.Candidate{ID: id}, nil
	}
	return marked, nil
}

type retainHarness struct {
	items      *itemStore
	containers *containerStore
	marking    *markedItems
	authorizer *authorizerDouble
	audit      *auditSink
	changes    *changeLog
	uow        *unitOfWork
}

func newRetainHarness() *retainHarness {
	h := &retainHarness{
		items:      &itemStore{stored: map[shared.ID]work.WorkItem{}},
		containers: &containerStore{stored: map[shared.ID]work.Container{}},
		marking: &markedItems{
			markingStore: &markingStore{},
			pending:      map[shared.ID]repository.Candidate{},
		},
		authorizer: &authorizerDouble{}, audit: &auditSink{},
		changes: &changeLog{}, uow: &unitOfWork{},
	}
	h.containers.stored[collectionID] = work.Container{
		ID: collectionID, TenantID: tenantID, Type: work.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", Version: 1,
	}
	announced := now.Add(14 * 24 * time.Hour)
	h.items.stored[taskID] = work.WorkItem{
		ID: taskID, TenantID: tenantID, CollectionID: collectionID, Type: work.ItemTask,
		Path: work.RootPath(taskID), Depth: 1, Title: "Weekly shop", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
		Retention: &work.RetentionState{
			Action: string(domain.ActionArchive), EffectiveAt: announced, PolicyID: holdID,
		},
	}
	h.marking.pending[taskID] = repository.Candidate{
		ID: taskID, Pending: announced, Rule: holdID, Action: domain.ActionArchive,
	}
	return h
}

func (h *retainHarness) service() RetainItem {
	return RetainItem{
		Items: h.items, Containers: h.containers, Marking: h.marking,
		Authorizer: h.authorizer, Audit: h.audit, Changes: h.changes,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), HLC: &hlcSource{},
	}
}

func TestRetainingTakesTheEntryOutAndEndsTheRulesClaim(t *testing.T) {
	h := newRetainHarness()

	if _, err := h.service().Execute(context.Background(), actor(), taskID); err != nil {
		t.Fatalf("retaining: %v", err)
	}

	if len(h.marking.cleared) != 1 || h.marking.cleared[0] != taskID {
		t.Fatalf("the marking was not cleared: %+v", h.marking.cleared)
	}
	// Taking an entry out means the rule no longer owns it - the next pass judges it afresh, which
	// is what makes editing it, moving it and this three ways of doing the same thing.
	if h.marking.keptRule {
		t.Error("the rule kept its claim on an entry somebody took out")
	}
	// Offline clients are told, because what changed is a field they show.
	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d change log entries", len(h.changes.recorded))
	}
	if h.changes.recorded[0].Op != syncrepo.Upsert || h.changes.recorded[0].EntityID != taskID {
		t.Errorf("the change log says %+v", h.changes.recorded[0])
	}
	// A decision to keep data the workspace's own rule said should go is in the trail.
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != RetainedAction {
		t.Fatalf("the audit trail holds %+v", h.audit.entries)
	}
	if h.audit.entries[0].Severity != "WARNING" {
		t.Errorf("the entry is a %s", h.audit.entries[0].Severity)
	}
}

// An entry nothing has announced anything about is refused rather than answered as if something had
// happened: a client that got 200 for this would show a button that does nothing.
func TestRetainingAnEntryNothingHasAnnouncedIsRefused(t *testing.T) {
	h := newRetainHarness()
	delete(h.marking.pending, taskID)

	_, err := h.service().Execute(context.Background(), actor(), taskID)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeNotMarked {
		t.Fatalf("refused with %v", err)
	}
	if len(h.marking.cleared) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused retain cleared or recorded something")
	}
}

// A write on the entry, narrowed like any other (C-04).
func TestRetainingIsAWriteOnTheEntry(t *testing.T) {
	h := newRetainHarness()

	if _, err := h.service().Execute(context.Background(), actor(), taskID); err != nil {
		t.Fatalf("retaining: %v", err)
	}

	request := h.authorizer.requests[0]
	if request.Permission != domainservice.PermissionWriteItems {
		t.Errorf("retaining asked for %q", request.Permission)
	}
	if request.On.ID != taskID {
		t.Error("retaining is not narrowed to the entry, so a contributor could retain anybody's")
	}
}

// §6: the entry says what is coming, and the answer carries it.
func TestTheAnsweredEntryCarriesWhatIsComing(t *testing.T) {
	h := newRetainHarness()

	out, err := h.service().Descriptor().Handler.Invoke(
		context.Background(), actor(), map[string]any{"item_id": taskID.String()})
	if err != nil {
		t.Fatalf("retaining through the handler: %v", err)
	}

	// The entry is read back after the clearing, so what a caller gets is the entry as it now
	// stands - which in the fake still carries the announcement, because the fake's Find does not
	// re-read. What matters here is that the mapping carries the field at all.
	retention, present := out["retention"].(map[string]any)
	if !present {
		t.Fatalf("the answered entry carries no retention: %+v", out)
	}
	if retention["action"] != string(domain.ActionArchive) {
		t.Errorf("the announcement says %v", retention["action"])
	}
	if retention["can_retain"] != true {
		t.Errorf("the announcement does not say it can be taken out")
	}
}
