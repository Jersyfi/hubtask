// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var (
	assignedItem = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	assigneeID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	strangerID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f3")
)

// assignmentProfiles is the shared fixture with ASSIGNMENT and MEMBERS put back where the
// capability matrix grants them: every type carries an assignee, an activity carries no member list
// (domain-model.md §2). systemProfiles is deliberately narrower - it is what the placement tests
// need - and these tests are about the capabilities themselves.
func assignmentProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		rows[i].Capabilities = append(row.Capabilities, domain.CapabilityAssignment)
		if row.Type != domain.ItemActivity {
			rows[i].Capabilities = append(rows[i].Capabilities, domain.CapabilityMembers)
		}
	}
	return rows
}

// visibility is the second permission question as the writers see it: who can reach what.
//
// It records the account it was asked about, because "the check ran" and "the check ran about the
// right person" are different things, and a use case that asked about the actor instead of the
// account it was handed would pass every test that only counted the calls.
type visibility struct {
	reachable map[shared.ID]bool
	asked     []shared.ID
	err       error
}

func newVisibility(accounts ...shared.ID) *visibility {
	v := &visibility{reachable: map[shared.ID]bool{}}
	for _, id := range accounts {
		v.reachable[id] = true
	}
	return v
}

func (v *visibility) CanSee(
	_ context.Context, _ appshared.ActorContext, accountID shared.ID, _ []identity.Scope,
) (bool, error) {
	v.asked = append(v.asked, accountID)
	if v.err != nil {
		return false, v.err
	}
	return v.reachable[accountID], nil
}

type assignmentHarness struct {
	assign     AssignWorkItem
	unassign   UnassignWorkItem
	items      *items
	containers *containers
	visibility *visibility
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
	uow        *unitOfWork
}

func newAssignmentHarness(t *testing.T) *assignmentHarness {
	t.Helper()

	h := &assignmentHarness{
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		visibility: newVisibility(assigneeID, accountID),
		events:     &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}

	writer := AssignmentWriter{
		Items: h.items, Containers: h.containers,
		Profiles: &profiles{rows: assignmentProfiles()}, Authorizer: h.authorizer,
		Visibility: h.visibility, Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.assign = AssignWorkItem{Assignment: writer}
	h.unassign = UnassignWorkItem{Assignment: writer}

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

func (h *assignmentHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: assignedItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(assignedItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[assignedItem] = item
	return item
}

func assignCmd() AssignmentCommand {
	return AssignmentCommand{ItemID: assignedItem, AccountID: assigneeID}
}

// One change owes four things, and this is the test that says so.
func TestAssigningWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	item, err := h.assign.Execute(ctx, actor(), assignCmd())
	if err != nil {
		t.Fatalf("assigning failed: %v", err)
	}

	if item.AssigneeID != assigneeID {
		t.Fatalf("the entry is on %q, want %q", item.AssigneeID, assigneeID)
	}
	if item.Version != 2 {
		t.Errorf("version %d, want the write to have spent one", item.Version)
	}

	t.Run("the row is written against the version that was read", func(t *testing.T) {
		if len(h.items.assignments) != 1 {
			t.Fatalf("%d writes, want 1", len(h.items.assignments))
		}
		if h.items.assignments[0].expectedVersion != 1 {
			t.Errorf("written against version %d", h.items.assignments[0].expectedVersion)
		}
	})

	t.Run("the event carries the reference rather than a snapshot", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.ItemAssigned {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["assignee_id"] != assigneeID.String() {
			t.Errorf("the event names %v", announcement.Payload["assignee_id"])
		}
		if _, snapshot := announcement.Payload["title"]; snapshot {
			t.Error("the event carries an entry snapshot")
		}
	})

	// A scalar merges last writer wins per field, so the entry names one field and takes an HLC of
	// its own (offline-sync.md §4.2).
	t.Run("the change names the one field that moved", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if len(change.Payload) != 1 || change.Payload[domain.FieldAssigneeID] != assigneeID.String() {
			t.Errorf("unexpected payload: %+v", change.Payload)
		}
		if change.HLC.IsZero() {
			t.Error("the change carries no clock reading")
		}
	})

	// An account identifier is PERSONAL_BASIC rather than content, and "who was this given to" is
	// not answerable without it (audit.md §4).
	t.Run("the audit entry names both accounts by identifier", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != ItemAssignedAction || entry.TargetID != assignedItem {
			t.Errorf("unexpected entry: %+v", entry)
		}
		recorded, _ := entry.Changes[domain.FieldAssigneeID].(map[string]any)
		if recorded == nil || recorded["to"] != assigneeID.String() {
			t.Errorf("the assignee is not in the trail: %+v", entry.Changes)
		}
	})

	t.Run("the history keeps both sides of the field", func(t *testing.T) {
		if len(h.history.entries) != 1 {
			t.Fatalf("%d history entries, want 1", len(h.history.entries))
		}
		step := h.history.entries[0]
		if step.Verb != activity.ItemAssigned {
			t.Errorf("verb %s", step.Verb)
		}
		field, _ := step.ChangeSet[domain.FieldAssigneeID].(map[string]any)
		if field == nil || field["to"] != assigneeID.String() {
			t.Errorf("the step does not name who has it now: %+v", step.ChangeSet)
		}
	})
}

// The permission question is the entry's: assigning work is writing an entry, not managing who is
// in the workspace.
func TestAssigningAsksForThePermissionToWriteItems(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)

	if _, err := h.assign.Execute(context.Background(), actor(), assignCmd()); err != nil {
		t.Fatalf("assigning failed: %v", err)
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
	if request.Action != ItemAssignedAction {
		t.Errorf("a refusal would be recorded against %s", request.Action)
	}
}

// The second question, and it is about the second person: the account being given the entry has to
// be able to see it. Asked about them rather than about the actor, which is the mistake a test that
// only counted the calls would not catch.
func TestAssigningAsksWhetherTheAccountCanSeeTheEntry(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)

	if _, err := h.assign.Execute(context.Background(), actor(), assignCmd()); err != nil {
		t.Fatalf("assigning failed: %v", err)
	}

	if len(h.visibility.asked) != 1 || h.visibility.asked[0] != assigneeID {
		t.Errorf("the visibility question was asked about %v", h.visibility.asked)
	}
}

// An assignment to somebody who gets a 404 on the entry is a piece of work nobody can do. Refused
// rather than stored, and refused the same way whether the account has no membership, belongs to
// another tenant, or does not exist at all - anything else is an oracle for which identifiers exist
// (T-04).
func TestAssigningAnAccountWithoutAccessIsRefused(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)

	cmd := assignCmd()
	cmd.AccountID = strangerID

	_, err := h.assign.Execute(context.Background(), actor(), cmd)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation refusal", err)
	}
	if code := shared.AsError(err).DetailCode; code != "items.account_without_access" {
		t.Errorf("detail code %q", code)
	}
	if len(h.items.assignments) != 0 {
		t.Error("the entry was written anyway")
	}
	if len(h.events.appended) != 0 {
		t.Error("a refusal announced something")
	}
}

// The refusal is one answer for three situations. A nonexistent account and one from another tenant
// both reach the check as "no membership anywhere on this path", because row level security removed
// their rows from the query - so the use case cannot tell them apart, and neither can a caller.
func TestAnUnknownAndACrossTenantAccountAreRefusedAlike(t *testing.T) {
	unknown := shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")

	codes := map[shared.ID]string{}
	for _, account := range []shared.ID{strangerID, unknown} {
		h := newAssignmentHarness(t)
		h.withItem(domain.ItemTask)

		cmd := assignCmd()
		cmd.AccountID = account
		_, err := h.assign.Execute(context.Background(), actor(), cmd)
		codes[account] = shared.AsError(err).DetailCode
	}

	if codes[strangerID] != codes[unknown] {
		t.Errorf("two answers, %q and %q", codes[strangerID], codes[unknown])
	}
}

// Handing an entry on is one call: one version, one event, one step of the history. Two calls would
// leave a moment in which the work belongs to nobody.
func TestHandingAnEntryOnIsOneStep(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	h.visibility.reachable[strangerID] = true
	ctx := context.Background()

	if _, err := h.assign.Execute(ctx, actor(), assignCmd()); err != nil {
		t.Fatalf("the first assignment failed: %v", err)
	}

	cmd := assignCmd()
	cmd.AccountID = strangerID
	item, err := h.assign.Execute(ctx, actor(), cmd)
	if err != nil {
		t.Fatalf("handing the entry on failed: %v", err)
	}

	if item.AssigneeID != strangerID {
		t.Errorf("the entry is on %q", item.AssigneeID)
	}
	if len(h.events.appended) != 2 {
		t.Fatalf("%d events, want one per assignment", len(h.events.appended))
	}
	if h.events.appended[1].Type != event.ItemAssigned {
		t.Errorf("handing on announced %s", h.events.appended[1].Type)
	}

	step := h.history.entries[1]
	field, _ := step.ChangeSet[domain.FieldAssigneeID].(map[string]any)
	if field == nil || field["from"] != assigneeID.String() || field["to"] != strangerID.String() {
		t.Errorf("the history does not say it changed hands: %+v", step.ChangeSet)
	}
}

// Idempotence is what makes two devices assigning the same person converge on one version.
func TestAssigningTheSamePersonAgainWritesNothing(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.assign.Execute(ctx, actor(), assignCmd()); err != nil {
		t.Fatalf("the first assignment failed: %v", err)
	}

	item, err := h.assign.Execute(ctx, actor(), assignCmd())
	if err != nil {
		t.Fatalf("the second assignment failed: %v", err)
	}

	if item.Version != 2 {
		t.Errorf("version %d, want the second call to have spent none", item.Version)
	}
	if len(h.items.assignments) != 1 {
		t.Errorf("%d writes, want the repeat to have written nothing", len(h.items.assignments))
	}
	if len(h.events.appended) != 1 {
		t.Errorf("%d events, want the repeat to have announced nothing", len(h.events.appended))
	}
}

func TestUnassigningClearsTheRowAndAnnouncesWhoItWas(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.assign.Execute(ctx, actor(), assignCmd()); err != nil {
		t.Fatalf("assigning failed: %v", err)
	}

	item, err := h.unassign.Execute(ctx, actor(), AssignmentCommand{ItemID: assignedItem})
	if err != nil {
		t.Fatalf("unassigning failed: %v", err)
	}

	if !item.AssigneeID.IsZero() {
		t.Errorf("the entry is still on %q", item.AssigneeID)
	}

	announcement := h.events.appended[1]
	if announcement.Type != event.ItemUnassigned {
		t.Fatalf("event type %s", announcement.Type)
	}
	// The person it was, rather than nobody: an event carrying nobody could only tell everybody or
	// nobody at all.
	if announcement.Payload["assignee_id"] != assigneeID.String() {
		t.Errorf("the event names %v, want the former assignee", announcement.Payload["assignee_id"])
	}

	// An absent field in a change log entry means "not touched", so the removal names the field as
	// empty rather than leaving it out.
	change := h.changes.recorded[1]
	if value, named := change.Payload[domain.FieldAssigneeID]; !named || value != "" {
		t.Errorf("the change does not clear the field: %+v", change.Payload)
	}
}

func TestUnassigningAnEntryNobodyIsOnWritesNothing(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)

	item, err := h.unassign.Execute(
		context.Background(), actor(), AssignmentCommand{ItemID: assignedItem})
	if err != nil {
		t.Fatalf("unassigning failed: %v", err)
	}

	if item.Version != 1 {
		t.Errorf("version %d, want no version spent", item.Version)
	}
	if len(h.events.appended) != 0 {
		t.Errorf("%d events, want none", len(h.events.appended))
	}
	// No visibility question either: an unassignment names nobody, so there is nobody to ask about.
	if len(h.visibility.asked) != 0 {
		t.Errorf("the visibility question was asked about %v", h.visibility.asked)
	}
}

// Every type carries an assignee, an activity included - it is the one field an activity has beyond
// its completion, and the row of the matrix where the assignee and the member list part company.
func TestAnActivityCanBeAssigned(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemActivity)

	item, err := h.assign.Execute(context.Background(), actor(), assignCmd())
	if err != nil {
		t.Fatalf("assigning an activity failed: %v", err)
	}
	if item.AssigneeID != assigneeID {
		t.Errorf("the activity is on %q", item.AssigneeID)
	}
}

// An activity's history is compact (domain-model.md §2): the verb, the actor and the time are the
// whole of the step, and the change set is empty. This is the one verb where that is reached in
// practice rather than in principle.
func TestAnActivityKeepsACompactHistoryOfItsAssignment(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemActivity)

	if _, err := h.assign.Execute(context.Background(), actor(), assignCmd()); err != nil {
		t.Fatalf("assigning an activity failed: %v", err)
	}

	step := h.history.entries[0]
	if step.Verb != activity.ItemAssigned {
		t.Errorf("verb %s", step.Verb)
	}
	if len(step.ChangeSet) != 0 {
		t.Errorf("the compact history carries a change set: %+v", step.ChangeSet)
	}
}

// A type whose profile does not carry ASSIGNMENT refuses the field rather than ignoring it: a
// client that assigned somebody and received a 200 would believe the work had an owner.
func TestATypeWithoutAssignmentIsRefused(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)

	narrowed := systemProfiles()
	h.assign.Assignment.Profiles = &profiles{rows: narrowed}

	_, err := h.assign.Execute(context.Background(), actor(), assignCmd())
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error = %v, want the capability refusal", err)
	}
}

// A trashed entry is read-only (I-W4). A conflict rather than a validation failure: the request is
// well formed and the state is what makes it impossible.
func TestAssigningATrashedEntryIsRefused(t *testing.T) {
	h := newAssignmentHarness(t)
	item := h.withItem(domain.ItemTask)
	trashedAt := now
	item.DeletedAt = &trashedAt
	h.items.stored[assignedItem] = item

	_, err := h.assign.Execute(context.Background(), actor(), assignCmd())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want a conflict", err)
	}
}

// The If-Match is honoured even when the change would have been a no-op: the state the caller was
// reasoning about is not the state that is there.
func TestAStaleVersionIsRefusedEvenWhenTheAssigneeWouldNotChange(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	if _, err := h.assign.Execute(ctx, actor(), assignCmd()); err != nil {
		t.Fatalf("assigning failed: %v", err)
	}

	cmd := assignCmd()
	cmd.ExpectedVersion = 1
	_, err := h.assign.Execute(ctx, actor(), cmd)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error = %v, want a version conflict", err)
	}
}

// Both directions travel through the catalogue, so the untyped channel has to reach the same
// command the typed call does.
func TestTheAssignmentChannelsReachTheSameCommand(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	ctx := context.Background()

	out, err := h.assign.Descriptor().Handler.Invoke(ctx, actor(), map[string]any{
		"item_id": assignedItem.String(), "account_id": assigneeID.String(),
	})
	if err != nil {
		t.Fatalf("the catalogue refused the call: %v", err)
	}
	if out["assignee_id"] != assigneeID.String() {
		t.Errorf("the output does not name the assignee: %+v", out)
	}

	out, err = h.unassign.Descriptor().Handler.Invoke(ctx, actor(), map[string]any{
		"item_id": assignedItem.String(),
	})
	if err != nil {
		t.Fatalf("the catalogue refused the unassignment: %v", err)
	}
	if out["assignee_id"] != nil {
		t.Errorf("the output still names an assignee: %+v", out)
	}
}

// An assignment naming nobody is a mistake a client can make and has to be told about, rather than
// an unassignment nobody asked for.
func TestAssigningNobodyIsRefusedByName(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)

	_, err := h.assign.Execute(
		context.Background(), actor(), AssignmentCommand{ItemID: assignedItem})
	if code := shared.AsError(err).DetailCode; code != "items.account_id_required" {
		t.Fatalf("detail code %q", code)
	}
}
