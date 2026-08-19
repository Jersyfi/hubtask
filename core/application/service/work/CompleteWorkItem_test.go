// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	taskID     = shared.MustParseID("0192f000-0000-7000-8000-000000000401")
	packageID  = shared.MustParseID("0192f000-0000-7000-8000-000000000402")
	activityID = shared.MustParseID("0192f000-0000-7000-8000-000000000403")
	otherActID = shared.MustParseID("0192f000-0000-7000-8000-000000000404")
)

// completionHarness is a task with a work package under it and two activities under that - the shape the
// roll-up has to walk. The tests set the completion states they need and read back what moved.
type completionHarness struct {
	writer     CompletionWriter
	items      *items
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newCompletionHarness(policy domain.CompletionPolicy) *completionHarness {
	store := &items{
		stored:   map[shared.ID]domain.WorkItem{},
		children: map[shared.ID]domain.ChildCompletion{},
	}
	containerStore := &containers{stored: map[shared.ID]domain.Container{}}

	h := &completionHarness{
		items: store, containers: containerStore,
		events: &events{}, changes: &changes{}, audit: &sink{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}
	h.writer = CompletionWriter{
		Items: store, Containers: containerStore, Profiles: &profiles{rows: systemProfiles()},
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}

	containerStore.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	containerStore.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now,
		CompletionPolicy: policy, Version: 1,
	}

	task := completionItem(taskID, domain.ItemTask, "")
	pack := completionItem(packageID, domain.ItemWorkPackage, taskID)
	first := completionItem(activityID, domain.ItemActivity, packageID)
	second := completionItem(otherActID, domain.ItemActivity, packageID)
	for _, item := range []domain.WorkItem{task, pack, first, second} {
		store.stored[item.ID] = item
	}

	// The shape as it stands: one work package under the task, two activities under the package.
	store.children[taskID] = domain.ChildCompletion{Total: 1}
	store.children[packageID] = domain.ChildCompletion{Total: 2}
	return h
}

func completionItem(id shared.ID, itemType domain.ItemType, parent shared.ID) domain.WorkItem {
	return domain.WorkItem{
		ID: id, TenantID: tenantID, CollectionID: collectionID, Type: itemType, ParentID: parent,
		Path: domain.RootPath(id), Depth: 1, Title: "Something", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
}

// complete marks one item done in the fake, so that a test can arrange the state the roll-up reads.
func (h *completionHarness) alreadyDone(id shared.ID) {
	item := h.items.stored[id]
	h.items.stored[id] = item.Completed(accountID, now)
}

// summarise sets what ChildCompletion answers for a parent.
func (h *completionHarness) summarise(parent shared.ID, total, completed int) {
	h.items.children[parent] = domain.ChildCompletion{Total: total, Completed: completed}
}

func (h *completionHarness) completed(id shared.ID) bool {
	for _, write := range h.items.completions {
		if write.item.ID == id {
			return write.item.Completion.IsCompleted
		}
	}
	return false
}

func (h *completionHarness) touched() []shared.ID {
	ids := make([]shared.ID, 0, len(h.items.completions))
	for _, write := range h.items.completions {
		ids = append(ids, write.item.ID)
	}
	return ids
}

// The acceptance criterion, first direction: completing the last activity completes the work package when
// the policy says so, and leaves it open when it does not.
func TestCompletingTheLastChildRollsUpOnlyUnderTheRollupPolicy(t *testing.T) {
	for _, policy := range []domain.CompletionPolicy{domain.CompletionRollup, domain.CompletionManual} {
		t.Run(string(policy), func(t *testing.T) {
			h := newCompletionHarness(policy)
			// One activity already done, and the other about to be: the level completes with this call.
			h.alreadyDone(otherActID)
			h.summarise(packageID, 2, 2)

			item, err := CompleteWorkItem{Completion: h.writer}.
				Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
			if err != nil {
				t.Fatalf("completing the activity: %v", err)
			}
			if !item.Completion.IsCompleted {
				t.Fatal("the activity itself is not completed")
			}

			rolledUp := h.completed(packageID)
			if policy == domain.CompletionRollup && !rolledUp {
				t.Errorf("the work package was not completed; touched %v", h.touched())
			}
			if policy == domain.CompletionManual && rolledUp {
				t.Errorf("the work package was completed under MANUAL; touched %v", h.touched())
			}
		})
	}
}

// One of two is not the last one. The commonest way for a roll-up to be wrong is to fire on every child.
func TestCompletingOneOfTwoChildrenRollsUpNothing(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.summarise(packageID, 2, 1)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	if touched := h.touched(); len(touched) != 1 || touched[0] != activityID {
		t.Errorf("the write touched %v, want only the activity", touched)
	}
}

// Two levels: completing the last activity completes the work package, and that in turn completes the task
// when the package was the task's only child.
func TestTheRollUpWalksAsFarAsTheTreeAllows(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(otherActID)
	h.summarise(packageID, 2, 2)
	h.summarise(taskID, 1, 1)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	for _, id := range []shared.ID{activityID, packageID, taskID} {
		if !h.completed(id) {
			t.Errorf("%s was not completed; touched %v", id, h.touched())
		}
	}
}

// And it stops where something is still open, rather than asking every level above.
func TestTheRollUpStopsAtTheFirstItemThatDoesNotMove(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(otherActID)
	h.summarise(packageID, 2, 2)
	// The task has another work package still open, so it must not complete.
	h.summarise(taskID, 2, 1)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	if h.completed(taskID) {
		t.Error("the task was completed while one of its work packages was open")
	}
	if len(h.touched()) != 2 {
		t.Errorf("the write touched %v, want the activity and the work package", h.touched())
	}
}

// The acceptance criterion, second direction: reopening a child reopens the parent per policy.
func TestReopeningAChildReopensTheParentPerPolicy(t *testing.T) {
	for _, policy := range []domain.CompletionPolicy{domain.CompletionRollup, domain.CompletionManual} {
		t.Run(string(policy), func(t *testing.T) {
			h := newCompletionHarness(policy)
			// Everything done, and one activity about to be reopened.
			for _, id := range []shared.ID{activityID, otherActID, packageID, taskID} {
				h.alreadyDone(id)
			}
			h.summarise(packageID, 2, 1)
			h.summarise(taskID, 1, 0)

			item, err := ReopenWorkItem{Completion: h.writer}.
				Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
			if err != nil {
				t.Fatalf("reopening: %v", err)
			}
			if item.Completion.IsCompleted {
				t.Fatal("the activity is still completed")
			}

			reopened := len(h.touched()) > 1 && !h.completed(packageID)
			if policy == domain.CompletionRollup && !reopened {
				t.Errorf("the work package was not reopened; touched %v", h.touched())
			}
			if policy == domain.CompletionManual && len(h.touched()) != 1 {
				t.Errorf("MANUAL touched %v, want only the activity", h.touched())
			}
		})
	}
}

// Both directions announce an event per item they change, and the parent's is caused by the child's -
// which is how a consumer tells a roll-up from a click without a second event type.
func TestEveryItemTheRollUpChangesIsAnnouncedInAChain(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(otherActID)
	h.summarise(packageID, 2, 2)
	h.summarise(taskID, 1, 1)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	if len(h.events.appended) != 3 {
		t.Fatalf("%d events for three completed items", len(h.events.appended))
	}
	for _, envelope := range h.events.appended {
		if envelope.Type != event.ItemCompleted {
			t.Errorf("an event of type %s was announced", envelope.Type)
		}
	}

	root, first, second := h.events.appended[0], h.events.appended[1], h.events.appended[2]
	if !root.CausationID.IsZero() {
		t.Error("the first event claims a cause")
	}
	if first.CausationID != root.ID || second.CausationID != first.ID {
		t.Errorf("the chain is %s <- %s <- %s", root.ID, first.CausationID, second.CausationID)
	}
	// One correlation for the whole consequence of one action, which is what ties a roll-up together in a
	// trace and in the automation trail.
	if first.CorrelationID != root.CorrelationID || second.CorrelationID != root.CorrelationID {
		t.Error("the roll-up left the correlation of the original action")
	}
	if first.CausationDepth != 1 || second.CausationDepth != 2 {
		t.Errorf("the depths are %d and %d", first.CausationDepth, second.CausationDepth)
	}
}

// A change log entry and an audit entry per item changed, inside the same transaction (test AT-5).
func TestEveryItemTheRollUpChangesIsRecorded(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(otherActID)
	h.summarise(packageID, 2, 2)
	h.summarise(taskID, 1, 1)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	if len(h.changes.recorded) != 3 {
		t.Errorf("%d change log entries for three changed items", len(h.changes.recorded))
	}
	if len(h.audit.entries) != 3 {
		t.Fatalf("%d audit entries for three changed items", len(h.audit.entries))
	}
	for _, entry := range h.audit.entries {
		if entry.Action != ItemCompletedAction {
			t.Errorf("an entry names the action %q", entry.Action)
		}
		// Rule 10: no user content in the trail. The title is what an item has, and none of it is here -
		// not even as a fingerprint, because a completion answers "who closed this, and when" without it.
		if _, present := entry.Changes["title"]; present {
			t.Error("the audit entry carries the title")
		}
		if _, present := entry.Changes["is_completed"]; !present {
			t.Error("the audit entry does not say what changed")
		}
	}
	if !h.uow.committed {
		t.Error("the transaction did not commit")
	}
}

// Idempotence at the use case, not only in the domain: completing an item that is already done writes
// nothing, spends no version and announces nothing.
func TestCompletingAnAlreadyCompletedItemChangesNothing(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(activityID)

	item, err := CompleteWorkItem{Completion: h.writer}.
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
	if err != nil {
		t.Fatalf("completing twice: %v", err)
	}
	if !item.Completion.IsCompleted {
		t.Error("the item came back open")
	}
	if len(h.items.completions) != 0 {
		t.Errorf("a repeat wrote %v", h.touched())
	}
	if len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Errorf("a repeat announced %d events and wrote %d audit entries",
			len(h.events.appended), len(h.audit.entries))
	}
}

func TestReopeningAnOpenItemChangesNothing(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)

	if _, err := (ReopenWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("reopening an open item: %v", err)
	}
	if len(h.items.completions) != 0 || len(h.events.appended) != 0 {
		t.Error("reopening an open item wrote something")
	}
}

// The If-Match is honoured even when the change would have been a no-op: the state the caller was
// reasoning about is not the state that is there, and telling it so is the point of the header.
func TestAStaleVersionIsRefusedEvenWhenNothingWouldChange(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(activityID)
	moved := h.items.stored[activityID]
	moved.Version = 5
	h.items.stored[activityID] = moved

	_, err := CompleteWorkItem{Completion: h.writer}.
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID, ExpectedVersion: 2})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version answered %v", err)
	}
}

// A refusal writes nothing at all, and the permission question is asked before the transaction so that the
// DENIED entry survives (audit.md §7).
func TestARefusedCompletionWritesNothing(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := CompleteWorkItem{Completion: h.writer}.
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v", err)
	}
	if len(h.items.completions) != 0 || len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused completion wrote something")
	}
	if h.uow.writes != 0 {
		t.Error("a refused completion opened a write transaction")
	}
}

// The permission asked for is the right to write items, on the path through the collection.
func TestCompletionAsksToWriteItems(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)

	if _, err := (CompleteWorkItem{Completion: h.writer}).
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions asked, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.TokenScope != itemsWrite {
		t.Errorf("asked for the scope %q", request.TokenScope)
	}
	// The action the refusal would be recorded against is the one that was attempted.
	if request.Action != ItemCompletedAction {
		t.Errorf("the refusal would name %q", request.Action)
	}
	if request.TargetID != activityID {
		t.Errorf("the refusal would name the target %s", request.TargetID)
	}
}

// An archived collection is read-only, and its items inherit that (I-C3).
func TestCompletionIsRefusedInAnArchivedCollection(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	collection := h.containers.stored[collectionID]
	collection.ArchivedAt = &now
	h.containers.stored[collectionID] = collection

	_, err := CompleteWorkItem{Completion: h.writer}.
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("answered %v, want a conflict", err)
	}
	if len(h.items.completions) != 0 {
		t.Error("something was written in an archived collection")
	}
}

// An archived parent bounds the roll-up rather than failing the request: the person's own action stands,
// and the automatic consequence stops where it is not allowed (I-W4).
func TestAnArchivedParentStopsTheRollUpWithoutFailingTheChild(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(otherActID)
	h.summarise(packageID, 2, 2)

	pack := h.items.stored[packageID]
	pack.ArchivedAt = &now
	h.items.stored[packageID] = pack

	item, err := CompleteWorkItem{Completion: h.writer}.
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
	if err != nil {
		t.Fatalf("completing under an archived parent: %v", err)
	}
	if !item.Completion.IsCompleted {
		t.Error("the activity was not completed")
	}
	if touched := h.touched(); len(touched) != 1 || touched[0] != activityID {
		t.Errorf("the write touched %v, want only the activity", touched)
	}
}

// A type whose profile does not carry COMPLETION cannot be completed by anybody, and the answer names the
// capability rather than the field (ADR-0006).
func TestCompletionIsRefusedWhereTheProfileDoesNotAllowIt(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.writer.Profiles = &profiles{rows: []domain.CapabilityProfile{
		{Type: domain.ItemTask, AllowedChildTypes: []domain.ItemType{domain.ItemWorkPackage}, MaxDepth: 3},
		{Type: domain.ItemWorkPackage, AllowedChildTypes: []domain.ItemType{domain.ItemActivity}, MaxDepth: 2},
		{Type: domain.ItemActivity, MaxDepth: 1},
	}}

	_, err := CompleteWorkItem{Completion: h.writer}.
		Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("answered %v, want a capability refusal", err)
	}
}

func TestAMissingItemIsNotFound(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)

	_, err := CompleteWorkItem{Completion: h.writer}.Execute(t.Context(), actorFixture(),
		CompletionCommand{ItemID: shared.MustParseID("0192f000-0000-7000-8000-0000000004ff")})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("answered %v", err)
	}
}

// `cascade_children` is in the contract with nothing behind it. Refused when true and accepted when false,
// because a client sending the documented default is asking for exactly what it gets.
func TestCascadeChildrenIsRefusedOnlyWhenAskedFor(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	handler := CompleteWorkItem{Completion: h.writer}

	_, err := handler.invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": activityID.String(), "cascade_children": true,
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("cascade_children=true answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.cascade_not_supported" {
		t.Errorf("the detail code is %q", got)
	}

	if _, err := handler.invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": activityID.String(), "cascade_children": false,
	}); err != nil {
		t.Errorf("cascade_children=false answered %v", err)
	}
}

// Both directions are writes and both declare their audit obligation, which gate SG-13 insists on.
func TestBothDirectionsDeclareTheirAudit(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{
		CompleteWorkItem{}.Descriptor(), ReopenWorkItem{}.Descriptor(),
	} {
		t.Run(descriptor.Name, func(t *testing.T) {
			if descriptor.ReadOnly {
				t.Error("a write is declared read-only")
			}
			if !descriptor.Audit.Required || descriptor.Audit.Action == "" {
				t.Error("declares no audit obligation")
			}
			if descriptor.Audit.Severity != audit.SeverityInfo {
				t.Errorf("the severity is %q", descriptor.Audit.Severity)
			}
			if descriptor.TokenScope != itemsWrite {
				t.Errorf("the token scope is %q", descriptor.TokenScope)
			}
		})
	}
	// The two must not share an action code: an auditor filtering on "work was finished" would otherwise
	// also see every reopening.
	forward, back := CompleteWorkItem{}.Descriptor(), ReopenWorkItem{}.Descriptor()
	if forward.Audit.Action == back.Audit.Action {
		t.Error("both directions record the same audit action")
	}
}
