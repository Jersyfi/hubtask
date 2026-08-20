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
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	tenantID     = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	hubID        = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	accountID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	taskID       = shared.MustParseID("0192f000-0000-7000-8000-000000000001")
	packageID    = shared.MustParseID("0192f000-0000-7000-8000-000000000002")
	otherTaskID  = shared.MustParseID("0192f000-0000-7000-8000-000000000003")
	holdID       = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	now          = time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	window       = 90 * 24 * time.Hour
)

// The fakes. They record rather than judge, because what is under test here is the order and the
// completeness of a removal - which of the four records were written, and in which order the rows
// went - and a fake that also decided would be deciding the thing being measured.

type trashStore struct {
	subtree    []shared.ID
	purgedItem []shared.ID
	purgedCont []shared.ID
	purgeErr   error
}

func (s *trashStore) List(context.Context, workrepo.Page) (workrepo.TrashPage, error) {
	return workrepo.TrashPage{}, nil
}
func (s *trashStore) SubtreeIDs(context.Context, string) ([]shared.ID, error) {
	return s.subtree, nil
}
func (s *trashStore) PurgeItems(_ context.Context, ids []shared.ID) (int, error) {
	if s.purgeErr != nil {
		return 0, s.purgeErr
	}
	s.purgedItem = append(s.purgedItem, ids...)
	return len(ids), nil
}
func (s *trashStore) PurgeContainers(_ context.Context, ids []shared.ID) (int, error) {
	if s.purgeErr != nil {
		return 0, s.purgeErr
	}
	s.purgedCont = append(s.purgedCont, ids...)
	return len(ids), nil
}

type expiredStore struct {
	items      []repository.ExpiredItem
	containers []repository.ExpiredContainer
	askedAt    time.Time
	askedBatch int
}

func (s *expiredStore) Items(
	_ context.Context, cutoff time.Time, batch int,
) ([]repository.ExpiredItem, error) {
	s.askedAt, s.askedBatch = cutoff, batch
	return s.items, nil
}
func (s *expiredStore) Containers(
	context.Context, time.Time, int,
) ([]repository.ExpiredContainer, error) {
	return s.containers, nil
}

type holdStore struct{ holds domain.Holds }

func (s *holdStore) Active(context.Context) (domain.Holds, error) { return s.holds, nil }

type removalStore struct {
	recorded   []domain.Removal
	deletedAt  time.Time
	purgeAfter time.Time
}

func (s *removalStore) Record(
	_ context.Context, removals []domain.Removal, deletedAt, purgeAfter time.Time,
) error {
	s.recorded = append(s.recorded, removals...)
	s.deletedAt, s.purgeAfter = deletedAt, purgeAfter
	return nil
}

type eventSink struct{ appended []event.Envelope }

func (e *eventSink) Append(_ context.Context, envelope event.Envelope) error {
	e.appended = append(e.appended, envelope)
	return nil
}

func (e *eventSink) PendingFor(context.Context, int) ([]event.Envelope, error) { return nil, nil }
func (e *eventSink) MarkDispatched(context.Context, []shared.ID) error         { return nil }
func (e *eventSink) MarkFailed(context.Context, shared.ID, string) error       { return nil }

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}

type idSource struct{ issued int }

func (i *idSource) NewID() shared.ID {
	i.issued++
	return shared.MustParseID("0192f000-0000-7000-8000-00000000010" + string(rune('0'+i.issued)))
}

type harness struct {
	purger   Purger
	trash    *trashStore
	expired  *expiredStore
	holds    *holdStore
	removals *removalStore
	events   *eventSink
	audit    *auditSink
}

func newHarness() *harness {
	h := &harness{
		trash: &trashStore{}, expired: &expiredStore{}, holds: &holdStore{},
		removals: &removalStore{}, events: &eventSink{}, audit: &auditSink{},
	}
	h.purger = Purger{
		Trash: h.trash, Expired: h.expired, Holds: h.holds, Removals: h.removals,
		Events: h.events, Audit: h.audit, Clock: clock.Fixed(now), IDs: &idSource{},
		TombstoneWindow: window, BatchSize: 1000,
	}
	return h
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
		AccountName: "Anna Beispiel",
	}
}

func trashedTask() work.WorkItem {
	deletedAt := now.Add(-40 * 24 * time.Hour)
	return work.WorkItem{
		ID: taskID, TenantID: tenantID, CollectionID: collectionID, Type: work.ItemTask,
		Path: work.RootPath(taskID), Depth: 1, Title: "Weekly shop", OrderKey: "a0",
		DeletedAt: &deletedAt, TrashBatchID: shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 2,
	}
}

// The four things a removal owes, and the order the two that can be got wrong have to be in: the
// journal and the tombstones are written before the rows go, so a transaction that fails between
// them leaves a record of a deletion that did not happen rather than a deletion with no record.
func TestPurgingASubtreeRecordsBeforeItRemoves(t *testing.T) {
	h := newHarness()
	h.trash.subtree = []shared.ID{taskID, packageID}

	removed, err := h.purger.Subtree(
		t.Context(), actor(), trashedTask(), hubID, domain.DeletedByUser, now)
	if err != nil {
		t.Fatalf("the purge was refused: %v", err)
	}

	if removed != 2 {
		t.Errorf("%d rows removed, want 2", removed)
	}
	if len(h.removals.recorded) != 2 {
		t.Fatalf("%d removals recorded, want 2", len(h.removals.recorded))
	}
	for _, removal := range h.removals.recorded {
		if removal.Entity != workItemEntity || removal.Reason != domain.DeletedByUser {
			t.Errorf("the removal reads %+v", removal)
		}
	}
	if h.removals.purgeAfter.Sub(h.removals.deletedAt) != window {
		t.Errorf("the tombstone window is %v, want %v",
			h.removals.purgeAfter.Sub(h.removals.deletedAt), window)
	}
	if len(h.trash.purgedItem) != 2 {
		t.Errorf("%d rows purged, want 2", len(h.trash.purgedItem))
	}
}

// One event per row, unlike the deletion into the trash, which announces its root alone. The purge is
// the last moment a media store or a search index can clean up what it holds for that row, and the
// payload carries no content of what was deleted (ADR-0018).
func TestEveryPurgedRowIsAnnouncedAndCarriesNoContent(t *testing.T) {
	h := newHarness()
	h.trash.subtree = []shared.ID{taskID, packageID}

	if _, err := h.purger.Subtree(
		t.Context(), actor(), trashedTask(), hubID, domain.DeletedByRetention, now); err != nil {
		t.Fatalf("the purge was refused: %v", err)
	}

	if len(h.events.appended) != 2 {
		t.Fatalf("%d events, want one per row removed", len(h.events.appended))
	}
	for _, envelope := range h.events.appended {
		if envelope.Type != event.ItemPurged {
			t.Errorf("the event is a %s, want a %s", envelope.Type, event.ItemPurged)
		}
		for _, field := range []string{"title", "notes", "completion"} {
			if _, present := envelope.Payload[field]; present {
				t.Errorf("the purge event carries %s", field)
			}
		}
		if envelope.Payload["reason"] != string(domain.DeletedByRetention) {
			t.Errorf("reason = %v", envelope.Payload["reason"])
		}
	}
}

// The first rule of data-retention.md §4: a legal hold overrides everything. On a named entry it is
// an error rather than a number - somebody asked for one thing, and "it was skipped" is not an answer
// they can act on. The hold's own reason does not travel: it is what an operator wrote for an
// auditor, not an explanation owed to whoever tried to delete something.
func TestALegalHoldRefusesANamedPurge(t *testing.T) {
	for _, c := range []struct {
		name string
		hold domain.LegalHold
	}{
		{"on the tenant", domain.LegalHold{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}},
		{"on the hub", domain.LegalHold{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID, Reason: "Litigation"}},
		{"on the entry", domain.LegalHold{ID: holdID, Scope: domain.HoldItem, ScopeID: taskID, Reason: "Litigation"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness()
			h.trash.subtree = []shared.ID{taskID}
			h.holds.holds = domain.Holds{c.hold}

			_, err := h.purger.Subtree(
				t.Context(), actor(), trashedTask(), hubID, domain.DeletedByUser, now)
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("a held purge reported %v, want a conflict", err)
			}
			if detail := shared.AsError(err).DetailCode; detail != "lifecycle.legal_hold" {
				t.Errorf("the detail code is %q", detail)
			}
			if params := shared.AsError(err).Params; params["reason"] != "" {
				t.Error("the hold's reason reached the client")
			}
			if len(h.trash.purgedItem) != 0 || len(h.removals.recorded) != 0 {
				t.Error("a held purge removed or recorded something")
			}
		})
	}
}

func expiredItem(id shared.ID, deletedAt time.Time) repository.ExpiredItem {
	return repository.ExpiredItem{
		ID: id, Type: work.ItemTask, Path: work.RootPath(id),
		CollectionID: collectionID, HubID: hubID, DeletedAt: deletedAt,
	}
}

// A sweep counts refusals rather than raising them: it selects rather than names, so "one of these is
// held" is a fact about the run and not a failure of it.
func TestASweepCountsWhatItCouldNotRemove(t *testing.T) {
	h := newHarness()
	longAgo := now.Add(-120 * 24 * time.Hour)
	h.expired.items = []repository.ExpiredItem{
		expiredItem(taskID, longAgo),
		expiredItem(otherTaskID, longAgo),
	}
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldItem, ScopeID: otherTaskID, Reason: "Litigation"}}

	outcome, err := h.purger.Sweep(t.Context(), actor(), Selection{
		Cutoff: now, Reason: domain.DeletedByRetention, ObserveTombstoneWindow: true,
	}, now)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if outcome.Matched != 2 {
		t.Errorf("matched %d, want 2", outcome.Matched)
	}
	if outcome.Removed != 1 {
		t.Errorf("removed %d, want 1", outcome.Removed)
	}
	if outcome.Blocked[BlockedByLegalHold] != 1 {
		t.Errorf("blocked by legal hold %d, want 1", outcome.Blocked[BlockedByLegalHold])
	}
	if len(h.trash.purgedItem) != 1 || h.trash.purgedItem[0] != taskID {
		t.Errorf("the sweep removed %v, want only the entry that is not held", h.trash.purgedItem)
	}
}

// The floor offline clients impose (offline-sync.md §7): an object may only disappear for good once
// every known device has had the chance to learn of the deletion. Past its period and still not
// removable is a state the run reports rather than hides.
func TestASweepRespectsTheTombstoneWindow(t *testing.T) {
	h := newHarness()
	h.expired.items = []repository.ExpiredItem{
		expiredItem(taskID, now.Add(-120*24*time.Hour)), // older than the window
		expiredItem(otherTaskID, now.Add(-40*24*time.Hour)),
	}

	outcome, err := h.purger.Sweep(t.Context(), actor(), Selection{
		Cutoff: now, Reason: domain.DeletedByRetention, ObserveTombstoneWindow: true,
	}, now)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if outcome.Removed != 1 {
		t.Errorf("removed %d, want 1 - the younger deletion is inside the offline window", outcome.Removed)
	}
	if outcome.Blocked[BlockedByTombstoneWindow] != 1 {
		t.Errorf("blocked by the window %d, want 1", outcome.Blocked[BlockedByTombstoneWindow])
	}
	if len(h.trash.purgedItem) != 1 || h.trash.purgedItem[0] != taskID {
		t.Errorf("the sweep removed %v", h.trash.purgedItem)
	}
}

// Entries before containers, and the cutoff read once for the whole pass: two readings of the clock
// would let a long batch use two definitions of "expired".
func TestASweepRemovesEntriesBeforeContainersAndAsksOneCutoff(t *testing.T) {
	h := newHarness()
	longAgo := now.Add(-120 * 24 * time.Hour)
	h.expired.items = []repository.ExpiredItem{expiredItem(taskID, longAgo)}
	h.expired.containers = []repository.ExpiredContainer{
		{ID: collectionID, Type: work.ContainerCollection, ParentID: hubID, DeletedAt: longAgo},
		{ID: hubID, Type: work.ContainerHub, DeletedAt: longAgo},
	}
	cutoff := now.Add(-30 * 24 * time.Hour)

	outcome, err := h.purger.Sweep(t.Context(), actor(), Selection{
		Cutoff: cutoff, Reason: domain.DeletedByRetention, ObserveTombstoneWindow: true,
	}, now)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if !h.expired.askedAt.Equal(cutoff) {
		t.Errorf("the read was asked for %v, want the cutoff %v", h.expired.askedAt, cutoff)
	}
	if h.expired.askedBatch != 1000 {
		t.Errorf("the batch was %d, want the configured 1000", h.expired.askedBatch)
	}
	if outcome.Removed != 3 {
		t.Errorf("removed %d, want 3", outcome.Removed)
	}
	// The journal names the entries first and the containers after, which is the order the rows go
	// in - a container removed before the entries in it would take them through the foreign key, and
	// rows removed by a cascade nobody counted leave no record behind.
	if len(h.removals.recorded) != 3 {
		t.Fatalf("%d removals recorded, want 3", len(h.removals.recorded))
	}
	if h.removals.recorded[0].Entity != workItemEntity {
		t.Errorf("the first removal is a %s, want an entry", h.removals.recorded[0].Entity)
	}
	if h.trash.purgedCont[0] != collectionID {
		t.Errorf("the containers went in the order %v, want the collection first", h.trash.purgedCont)
	}
}

// The trail gets a summary rather than an entry per row: an audit that grew with every deleted object
// would grow faster than the payload data it is about (data-retention.md §5). A warning, because this
// is the entry somebody is looking for when work has gone and nobody knows where.
func TestTheRemovalIsAuditedAsASummary(t *testing.T) {
	h := newHarness()
	outcome := Outcome{Matched: 12, Removed: 10}
	outcome.blocked(BlockedByLegalHold)
	outcome.blocked(BlockedByTombstoneWindow)

	err := h.purger.RecordAudit(t.Context(), actor(), "trash.emptied", trashTarget, tenantID,
		outcome, domain.DeletedByUser, now)
	if err != nil {
		t.Fatalf("writing the audit entry: %v", err)
	}

	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Severity != audit.SeverityWarning {
		t.Errorf("severity %s, want a warning", entry.Severity)
	}
	for field, want := range map[string]string{
		"matched": "12", "removed": "10",
		"blocked_legal_hold": "1", "blocked_tombstone_window": "1",
		"reason": "USER",
	} {
		got, _ := entry.Changes[field].(map[string]any)["to"].(string)
		if got != want {
			t.Errorf("the entry records %s = %q, want %q", field, got, want)
		}
	}
}

// Nothing to remove is the commonest run there is, and it costs nothing: no records, no events, and
// no statement.
func TestASweepWithNothingToDoWritesNothing(t *testing.T) {
	h := newHarness()

	outcome, err := h.purger.Sweep(t.Context(), actor(), Selection{
		Cutoff: now, Reason: domain.DeletedByRetention, ObserveTombstoneWindow: true,
	}, now)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if outcome.Matched != 0 || outcome.Removed != 0 {
		t.Errorf("an empty sweep reports %+v", outcome)
	}
	if len(h.removals.recorded) != 0 || len(h.events.appended) != 0 {
		t.Error("an empty sweep wrote something")
	}
}

// The window is a floor on the automatic paths and not on an explicit one. A person emptying their
// own trash has said so, and the tombstone the purge writes is what stops the resurrection there.
func TestAnExplicitSweepDoesNotWaitOutTheOfflineWindow(t *testing.T) {
	h := newHarness()
	h.expired.items = []repository.ExpiredItem{
		expiredItem(taskID, now.Add(-40*24*time.Hour)), // well inside the window
	}

	outcome, err := h.purger.Sweep(t.Context(), actor(), Selection{
		Cutoff: now, Reason: domain.DeletedByUser, ObserveTombstoneWindow: false,
	}, now)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if outcome.Removed != 1 || outcome.Blocked[BlockedByTombstoneWindow] != 0 {
		t.Errorf("an explicit sweep reports %+v, want the entry removed", outcome)
	}
	// The marker still outlives the row by the whole window, which is what makes it safe.
	if h.removals.purgeAfter.Sub(h.removals.deletedAt) != window {
		t.Errorf("the tombstone window is %v, want %v",
			h.removals.purgeAfter.Sub(h.removals.deletedAt), window)
	}
}
