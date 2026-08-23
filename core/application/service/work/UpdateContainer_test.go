// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var (
	shoppingID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
	otherHubID    = shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	archivedEarly = now.Add(-time.Hour)
)

// policyStore fakes the assignment policy rows, keyed by scope. It records every write, because
// the C-02 tests care about what reached the row as much as about what came back.
type policyStore struct {
	stored   map[shared.ID]domain.AutoAssignPolicy
	upserted []domain.AutoAssignPolicy
	deleted  []shared.ID
	saved    []domain.AutoAssignPolicy
}

func newPolicyStore() *policyStore {
	return &policyStore{stored: map[shared.ID]domain.AutoAssignPolicy{}}
}

func (p *policyStore) FindForScope(
	_ context.Context, _ domain.AutoAssignScope, scopeID shared.ID,
) (domain.AutoAssignPolicy, error) {
	policy, found := p.stored[scopeID]
	if !found {
		return domain.AutoAssignPolicy{}, shared.ErrNotFound
	}
	return policy, nil
}

func (p *policyStore) Lock(
	ctx context.Context, scope domain.AutoAssignScope, scopeID shared.ID,
) (domain.AutoAssignPolicy, error) {
	return p.FindForScope(ctx, scope, scopeID)
}

func (p *policyStore) Upsert(_ context.Context, policy domain.AutoAssignPolicy) error {
	if existing, found := p.stored[policy.ScopeID]; found {
		// The row keeps its identity and the rotation resets, exactly as the statement behind
		// the real adapter writes it.
		policy.ID = existing.ID
		policy.Version = existing.Version + 1
	} else {
		policy.Version = 1
	}
	policy.State = domain.AutoAssignState{}
	p.stored[policy.ScopeID] = policy
	p.upserted = append(p.upserted, policy)
	return nil
}

func (p *policyStore) Delete(
	_ context.Context, _ domain.AutoAssignScope, scopeID shared.ID,
) error {
	delete(p.stored, scopeID)
	p.deleted = append(p.deleted, scopeID)
	return nil
}

func (p *policyStore) SaveState(_ context.Context, policy domain.AutoAssignPolicy) error {
	stored, found := p.stored[policy.ScopeID]
	if !found {
		return shared.ErrNotFound
	}
	stored.State = policy.State
	p.stored[policy.ScopeID] = stored
	p.saved = append(p.saved, policy)
	return nil
}

// containerHarness wires the writer every lifecycle use case shares, so a test names only what it
// is about.
type containerHarness struct {
	writer     ContainerWriter
	containers *containers
	policies   *policyStore
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
	jobs       *jobs
}

func newContainerHarness() *containerHarness {
	store := &containers{stored: map[shared.ID]domain.Container{}}
	h := &containerHarness{
		containers: store,
		policies:   newPolicyStore(),
		events:     &events{},
		changes:    &changes{},
		audit:      &sink{},
		authorizer: &authorizer{},
		uow:        &unitOfWork{},
		jobs:       &jobs{},
	}
	h.writer = ContainerWriter{
		Containers: store, Policies: h.policies, Authorizer: h.authorizer, Events: h.events,
		Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now),
		IDs: &ids{}, HLC: &hlcSource{}, Queue: h.jobs,
	}
	return h
}

// withCollection stores a collection under the hub, as a read would hand it up.
func (h *containerHarness) withCollection() domain.Container {
	collection := domain.Container{
		ID: shoppingID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CompletionPolicy: domain.CompletionManual,
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 3,
	}
	h.containers.stored[shoppingID] = collection
	return collection
}

func (h *containerHarness) withHub(id shared.ID, name string) domain.Container {
	hub := domain.Container{
		ID: id, TenantID: tenantID, Type: domain.ContainerHub, Name: name, OrderKey: "a0",
		CompletionPolicy: domain.CompletionManual, CreatedBy: accountID, CreatedAt: now,
		UpdatedAt: now, Version: 1,
	}
	h.containers.stored[id] = hub
	return hub
}

func value(s string) *string { return &s }

// One write owes four things - the row, the event, the change log entry and the audit entry - and
// this is the test that says so for a rename.
func TestRenamingWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	updated, err := RenameContainer{Writer: h.writer}.Execute(ctx, actor(), RenameContainerCommand{
		ContainerID: shoppingID,
		Attributes:  domain.ContainerAttributes{Name: value("Groceries"), Icon: value("basket")},
	})
	if err != nil {
		t.Fatalf("renaming failed: %v", err)
	}

	if updated.Name != "Groceries" || updated.Icon != "basket" {
		t.Errorf("unexpected container: %+v", updated)
	}
	if updated.Version != 4 {
		t.Errorf("version %d, want the stored 3 plus one", updated.Version)
	}
	if len(h.containers.written) != 1 || h.containers.written[0].method != "attributes" {
		t.Fatalf("unexpected writes: %+v", h.containers.written)
	}
	if h.containers.written[0].expected != 3 {
		t.Errorf("written against version %d, want the one that was read", h.containers.written[0].expected)
	}

	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerRenamed {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	// One change log entry per field, each with its own HLC: the merge rule is last writer wins per
	// field, and one entry covering both would let the later HLC decide them together
	// (offline-sync.md §4.2).
	if len(h.changes.recorded) != 2 {
		t.Fatalf("%d change log entries, want one per field", len(h.changes.recorded))
	}
	if h.changes.recorded[0].HLC == h.changes.recorded[1].HLC {
		t.Error("the two entries share an HLC, so a merge would decide them together")
	}
	// The hub above it, so a device subscribed to the hub sees the change.
	if h.changes.recorded[0].ContainerID != hubID {
		t.Errorf("the change is filed under %s, want the hub", h.changes.recorded[0].ContainerID)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ContainerRenamedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
}

// Rule 10: a container's name is user content and does not go into the trail. The entry says the
// field changed and carries a fingerprint of each side, which is enough to see that two entries
// concern the same name and not enough to read it.
func TestTheRenameAuditEntryCarriesNoName(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := RenameContainer{Writer: h.writer}.Execute(t.Context(), actor(), RenameContainerCommand{
		ContainerID: shoppingID,
		Attributes:  domain.ContainerAttributes{Name: value("Groceries")},
	})
	if err != nil {
		t.Fatalf("renaming failed: %v", err)
	}

	entry := h.audit.entries[0]
	if entry.TargetLabel != "" {
		t.Errorf("the entry carries a label: %q", entry.TargetLabel)
	}
	masked, ok := entry.Changes["name"].(map[string]any)
	if !ok {
		t.Fatalf("the entry does not record the name as changed: %+v", entry.Changes)
	}
	if masked["changed"] != true || masked["to_hash"] == nil || masked["from_hash"] == nil {
		t.Errorf("the name is not masked as a pair of fingerprints: %+v", masked)
	}
	if rendered := fmt.Sprint(entry.Changes); strings.Contains(rendered, "Groceries") ||
		strings.Contains(rendered, "Shopping") {
		t.Errorf("the entry carries the name in clear text: %s", rendered)
	}
}

// The idempotency contract: asking for what is already stored succeeds, writes nothing, spends no
// version and announces nothing. That is what makes a client that echoes the whole object back
// harmless rather than merely accepted.
func TestRenamingToWhatIsAlreadyStoredWritesNothing(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	updated, err := RenameContainer{Writer: h.writer}.Execute(t.Context(), actor(), RenameContainerCommand{
		ContainerID: shoppingID,
		Attributes:  domain.ContainerAttributes{Name: value("Shopping")},
	})
	if err != nil {
		t.Fatalf("the repeat was refused: %v", err)
	}
	if updated.Version != 3 {
		t.Errorf("version %d, want the stored one", updated.Version)
	}
	if len(h.containers.written) != 0 || len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Errorf("a no-op wrote something: %d rows, %d events, %d entries",
			len(h.containers.written), len(h.events.appended), len(h.audit.entries))
	}
}

// The If-Match is honoured even when the change would have been a no-op: the state the caller was
// reasoning about is not the state that is there, whatever it asked for.
func TestAStaleContainerVersionIsRefusedEvenWhenNothingWouldChange(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := RenameContainer{Writer: h.writer}.Execute(t.Context(), actor(), RenameContainerCommand{
		ContainerID:     shoppingID,
		Attributes:      domain.ContainerAttributes{Name: value("Shopping")},
		ExpectedVersion: 2,
	})
	assertConflict(t, err, "containers.version_conflict")
}

// I-C3 through the use case: a collection whose hub is archived is read-only, and the refusal says
// which container to unarchive.
func TestRenamingIsRefusedInsideAnArchivedHub(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.ParentArchivedAt = &archivedEarly
	h.containers.stored[shoppingID] = collection

	_, err := RenameContainer{Writer: h.writer}.Execute(t.Context(), actor(), RenameContainerCommand{
		ContainerID: shoppingID,
		Attributes:  domain.ContainerAttributes{Name: value("Groceries")},
	})
	assertConflict(t, err, "containers.archived")
	if got := shared.AsError(err).Params["archived_id"]; got != hubID.String() {
		t.Errorf("the refusal names %q, want the hub to unarchive", got)
	}
	if len(h.containers.written) != 0 || len(h.events.appended) != 0 {
		t.Error("a refused rename wrote something anyway")
	}
}

// The permission is asked before the transaction, against the path the container sits on: a
// membership held at the hub applies to the collections in it (domain-model.md §3.2).
func TestTheRenamePermissionIsAskedAgainstTheWholePath(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := RenameContainer{Writer: h.writer}.Execute(t.Context(), actor(), RenameContainerCommand{
		ContainerID: shoppingID,
		Attributes:  domain.ContainerAttributes{Name: value("Groceries")},
	})
	if err != nil {
		t.Fatalf("renaming failed: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want one", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionStructure {
		t.Errorf("permission %s, want STRUCTURE - a container is the shape of the workspace", request.Permission)
	}
	if len(request.Path) != 3 {
		t.Errorf("the path is %+v, want tenant, hub and collection", request.Path)
	}
}

func TestRenamingAContainerThatIsNotThere(t *testing.T) {
	h := newContainerHarness()

	_, err := RenameContainer{Writer: h.writer}.Execute(t.Context(), actor(), RenameContainerCommand{
		ContainerID: shoppingID,
		Attributes:  domain.ContainerAttributes{Name: value("Groceries")},
	})
	assertNotFound(t, err, "containers.not_found")
	if len(h.authorizer.requests) != 0 {
		t.Error("a container that does not exist was still put to the authorisation service")
	}
}

func TestUpdatingThePoliciesWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	updated, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: shoppingID,
			Policies:    domain.ContainerPolicies{CompletionPolicy: domain.CompletionRollup},
		})
	if err != nil {
		t.Fatalf("configuring the collection failed: %v", err)
	}

	if updated.CompletionPolicy != domain.CompletionRollup {
		t.Errorf("policy %q, want ROLLUP", updated.CompletionPolicy)
	}
	if len(h.containers.written) != 1 || h.containers.written[0].method != "policies" {
		t.Fatalf("unexpected writes: %+v", h.containers.written)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerPoliciesUpdated {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if len(h.changes.recorded) != 1 || len(h.audit.entries) != 1 {
		t.Fatalf("%d changes and %d entries, want one of each",
			len(h.changes.recorded), len(h.audit.entries))
	}
	if h.audit.entries[0].Action != ContainerPoliciesUpdatedAction {
		t.Errorf("audit action %q, want the policies one", h.audit.entries[0].Action)
	}
}

// A policy is a value this installation defined rather than something a person typed, so it is in
// the trail in clear text: an auditor asking when a collection started rolling up has no other way
// to answer it, and there is no personal data in "ROLLUP".
func TestThePolicyAuditEntryCarriesTheValue(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: shoppingID,
			Policies:    domain.ContainerPolicies{CompletionPolicy: domain.CompletionRollup},
		})
	if err != nil {
		t.Fatalf("configuring the collection failed: %v", err)
	}

	recorded, ok := h.audit.entries[0].Changes["completion_policy"].(map[string]any)
	if !ok || recorded["to"] != "ROLLUP" || recorded["from"] != "MANUAL" {
		t.Errorf("the entry does not record the policy in the open: %+v", h.audit.entries[0].Changes)
	}
}

// A hub holds collections and no items, so a completion policy on one would decide nothing.
func TestThePoliciesOfAHubAreRefused(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Private")

	_, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: hubID,
			Policies:    domain.ContainerPolicies{CompletionPolicy: domain.CompletionRollup},
		})
	assertValidation(t, err, "containers.policies_not_supported")
}

// A PUT replaces: a key that is not sent is the default, not what happens to be stored. With one
// key in the document today, this is the whole of that contract.
func TestAnAbsentPolicyKeyFallsBackToTheDefault(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.CompletionPolicy = domain.CompletionRollup
	h.containers.stored[shoppingID] = collection

	updated, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{ContainerID: shoppingID})
	if err != nil {
		t.Fatalf("configuring the collection failed: %v", err)
	}
	if updated.CompletionPolicy != domain.CompletionManual {
		t.Errorf("policy %q, want the default back", updated.CompletionPolicy)
	}
}

// The auto_assign key of the document lives in its own row, and the store writes both sides
// inside one transaction (C-02): the JSONB key for the completion policy, the policy row for the
// assignment.
func TestConfiguringAutoAssignWritesTheRowBesideTheDocument(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	rotation := &domain.AutoAssignDefinition{
		Strategy: domain.AssignRoundRobin,
		Candidates: []domain.AutoAssignCandidate{
			{Kind: domain.CandidateAccount, ID: assigneeID},
			{Kind: domain.CandidateAccount, ID: strangerID},
		},
		Enabled: true,
	}
	updated, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: shoppingID,
			Policies: domain.ContainerPolicies{
				CompletionPolicy: domain.CompletionManual, AutoAssign: rotation,
			},
		})
	if err != nil {
		t.Fatalf("configuring the collection failed: %v", err)
	}

	if updated.AutoAssign == nil || updated.AutoAssign.Strategy != domain.AssignRoundRobin {
		t.Fatalf("the collection does not carry the key: %+v", updated.AutoAssign)
	}
	if len(h.policies.upserted) != 1 {
		t.Fatalf("policy rows written: %+v", h.policies.upserted)
	}
	row := h.policies.upserted[0]
	if row.ScopeType != domain.AutoAssignScopeCollection || row.ScopeID != shoppingID ||
		row.Strategy != domain.AssignRoundRobin || len(row.Candidates) != 2 || !row.Enabled {
		t.Fatalf("the row does not say what the document says: %+v", row)
	}
	if row.ID.IsZero() {
		t.Error("the row was written without an identifier")
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerPoliciesUpdated {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if payload, ok := h.events.appended[0].Payload["auto_assign"].(map[string]any); !ok ||
		payload["strategy"] != "ROUND_ROBIN" {
		t.Errorf("the event snapshot does not carry the policy: %+v",
			h.events.appended[0].Payload["auto_assign"])
	}
}

func TestLeavingTheAutoAssignKeyOutRemovesTheRow(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.AutoAssign = &domain.AutoAssignDefinition{
		Strategy:   domain.AssignFixed,
		Candidates: []domain.AutoAssignCandidate{{Kind: domain.CandidateAccount, ID: assigneeID}},
		Enabled:    true,
	}
	h.containers.stored[shoppingID] = collection

	updated, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: shoppingID,
			Policies:    domain.ContainerPolicies{CompletionPolicy: domain.CompletionManual},
		})
	if err != nil {
		t.Fatalf("removing the key failed: %v", err)
	}
	if updated.AutoAssign != nil {
		t.Fatal("the key survived its removal")
	}
	if len(h.policies.deleted) != 1 || h.policies.deleted[0] != shoppingID {
		t.Fatalf("rows deleted: %+v, want the collection's", h.policies.deleted)
	}
	if len(h.policies.upserted) != 0 {
		t.Errorf("a removal upserted a row: %+v", h.policies.upserted)
	}
}

func TestAnInvalidAutoAssignDocumentWritesNothing(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: shoppingID,
			Policies: domain.ContainerPolicies{
				AutoAssign: &domain.AutoAssignDefinition{
					Strategy: domain.AssignRandomGroupMember,
					Candidates: []domain.AutoAssignCandidate{
						{Kind: domain.CandidateAccount, ID: assigneeID},
					},
				},
			},
		})
	assertValidation(t, err, "containers.auto_assign_candidate_kind_invalid")
	if len(h.policies.upserted)+len(h.policies.deleted) != 0 ||
		len(h.containers.written) != 0 || len(h.events.appended) != 0 {
		t.Error("a refused document still moved something")
	}
}

// The untyped input every channel hands in carries the key as decoded JSON, and the parse is one
// code path in the domain - so one channel test covers the three channels.
func TestThePoliciesChannelParsesTheAutoAssignKey(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	handler := UpdateContainerPolicies{Writer: h.writer}.Descriptor().Handler
	_, err := handler.Invoke(t.Context(), actor(), usecase.Input{
		"container_id": shoppingID.String(),
		"auto_assign": map[string]any{
			"strategy": "FIXED",
			"candidates": []any{
				map[string]any{"kind": "ACCOUNT", "id": assigneeID.String()},
			},
			"enabled": false,
		},
	})
	if err != nil {
		t.Fatalf("the channel refused the key: %v", err)
	}
	if len(h.policies.upserted) != 1 || h.policies.upserted[0].Strategy != domain.AssignFixed ||
		h.policies.upserted[0].Enabled {
		t.Fatalf("the parsed policy is %+v", h.policies.upserted)
	}
}

func TestUpdatingThePoliciesToWhatIsStoredWritesNothing(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := UpdateContainerPolicies{Writer: h.writer}.Execute(
		t.Context(), actor(), UpdateContainerPoliciesCommand{
			ContainerID: shoppingID,
			Policies:    domain.ContainerPolicies{CompletionPolicy: domain.CompletionManual},
		})
	if err != nil {
		t.Fatalf("the repeat was refused: %v", err)
	}
	if len(h.containers.written) != 0 || len(h.events.appended) != 0 {
		t.Errorf("a no-op wrote something: %+v", h.containers.written)
	}
}

// Both use cases reach every channel through the same untyped input, and the distinction between
// "not sent" and "sent empty" has to survive it - a rename that could not tell them apart would
// clear the icon of every client that only meant to rename something.
func TestTheRenameInvocationKeepsTheDifferenceBetweenAbsentAndEmpty(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()
	handler := RenameContainer{Writer: h.writer}

	_, err := handler.Descriptor().Handler.Invoke(t.Context(), actor(), map[string]any{
		"container_id": shoppingID.String(), "icon": "",
	})
	if err != nil {
		t.Fatalf("clearing the icon was refused: %v", err)
	}
	if len(h.containers.written) != 0 {
		t.Error("clearing an icon that was already empty wrote a row")
	}

	// And an input naming no field at all is a request that says nothing, which is refused rather
	// than answered with the container unchanged.
	_, err = handler.Descriptor().Handler.Invoke(t.Context(), actor(), map[string]any{
		"container_id": shoppingID.String(),
	})
	assertValidation(t, err, "containers.update_empty")
}

func assertConflict(t *testing.T, err error, detailCode string) {
	t.Helper()
	assertCode(t, err, detailCode)
}

func assertNotFound(t *testing.T, err error, detailCode string) {
	t.Helper()
	assertCode(t, err, detailCode)
}

func assertValidation(t *testing.T, err error, detailCode string) {
	t.Helper()
	assertCode(t, err, detailCode)
}

func assertCode(t *testing.T, err error, detailCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want %s", detailCode)
	}
	if got := shared.AsError(err).DetailCode; got != detailCode {
		t.Fatalf("detail code %s, want %s", got, detailCode)
	}
}
