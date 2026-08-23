// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The create path's half of C-02: an entry created already on somebody - by name, or by the
// collection's policy - with the same records the standalone assignment writes.

// withAssignment gives the harness the profiles that carry ASSIGNMENT and returns a create
// command; the fixture profiles of the placement tests are deliberately narrower.
func (h *itemHarness) withAssignment() {
	h.profiles.rows = assignmentProfiles()
}

// withCollectionPolicy configures the collection's policy the way the adapter hands it up.
func (h *itemHarness) withCollectionPolicy(enabled bool, candidates ...domain.AutoAssignCandidate) {
	definition := &domain.AutoAssignDefinition{
		Strategy: domain.AssignFixed, Candidates: candidates, Enabled: enabled,
	}
	collection := h.containers.stored[collectionID]
	collection.AutoAssign = definition
	h.containers.stored[collectionID] = collection

	h.policies.stored[collectionID] = domain.AutoAssignPolicy{
		ID: policyID, TenantID: tenantID,
		ScopeType: domain.AutoAssignScopeCollection, ScopeID: collectionID,
		Strategy: definition.Strategy, Candidates: candidates, Enabled: enabled, Version: 1,
	}
}

func TestCreatingAlreadyAssignedWritesTheCreateAndTheAssignment(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()

	cmd := taskCommand()
	cmd.AssigneeID = assigneeID
	item, outcome, err := h.handler.Execute(t.Context(), itemActor(), cmd)
	if err != nil {
		t.Fatalf("creating assigned failed: %v", err)
	}

	if item.AssigneeID != assigneeID {
		t.Errorf("the entry is on %q, want the named person", item.AssigneeID)
	}
	if outcome != nil {
		t.Errorf("outcome %+v, want none - nothing automatic ran", outcome)
	}
	if item.Version != 2 {
		t.Errorf("version %d, want the create's 1 plus the assignment's own", item.Version)
	}
	if len(h.events.appended) != 2 ||
		h.events.appended[0].Type != event.ItemCreated ||
		h.events.appended[1].Type != event.ItemAssigned {
		t.Fatalf("events %+v, want the creation and then the assignment", h.events.appended)
	}
	if _, carries := h.events.appended[1].Payload["strategy"]; carries {
		t.Error("a named person is not a strategy - the event must not claim one")
	}
	if len(h.visibility.asked) != 1 || h.visibility.asked[0] != assigneeID {
		t.Errorf("visibility asked about %v, want the assignee", h.visibility.asked)
	}
	if len(h.history.entries) != 2 {
		t.Errorf("%d history steps, want the creation and the assignment", len(h.history.entries))
	}
}

func TestAnAssigneeWhoCannotSeeTheCollectionIsRefusedOnCreate(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()

	cmd := taskCommand()
	cmd.AssigneeID = strangerID
	_, _, err := h.handler.Execute(t.Context(), itemActor(), cmd)
	assertValidation(t, err, "items.account_without_access")
	if len(h.items.inserted) != 0 {
		t.Error("the refused create still wrote the row")
	}
}

func TestAnEnabledPolicyAppliesItselfToWhatIsCreated(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.withCollectionPolicy(true, domain.AutoAssignCandidate{
		Kind: domain.CandidateAccount, ID: assigneeID,
	})

	item, outcome, err := h.handler.Execute(t.Context(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if item.AssigneeID != assigneeID {
		t.Errorf("the entry is on %q, want the policy's pick", item.AssigneeID)
	}
	if outcome == nil || !outcome.Assigned || outcome.Strategy != domain.AssignFixed {
		t.Fatalf("outcome %+v, want an assignment by FIXED", outcome)
	}
	if got := h.events.appended[1].Payload["strategy"]; got != "FIXED" {
		t.Errorf("the assignment event carries strategy %v", got)
	}
}

func TestADisabledPolicyWaitsToBeAsked(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.withCollectionPolicy(false, domain.AutoAssignCandidate{
		Kind: domain.CandidateAccount, ID: assigneeID,
	})

	item, outcome, err := h.handler.Execute(t.Context(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if !item.AssigneeID.IsZero() || outcome != nil {
		t.Fatalf("a policy that is not enabled ran by itself: %q, %+v", item.AssigneeID, outcome)
	}

	asked := taskCommand()
	asked.AutoAssign = true
	item, outcome, err = h.handler.Execute(t.Context(), itemActor(), asked)
	if err != nil {
		t.Fatalf("the explicit ask failed: %v", err)
	}
	if item.AssigneeID != assigneeID || outcome == nil || !outcome.Assigned {
		t.Fatalf("the explicit ask did not run the policy: %q, %+v", item.AssigneeID, outcome)
	}
}

func TestAskingWithoutAPolicyIsRefused(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()

	cmd := taskCommand()
	cmd.AutoAssign = true
	_, _, err := h.handler.Execute(t.Context(), itemActor(), cmd)
	assertValidation(t, err, "items.auto_assign_unavailable")
	if len(h.items.inserted) != 0 {
		t.Error("the refused create still wrote the row")
	}
}

func TestNamingAPersonAndAskingForThePolicyIsRefused(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.withCollectionPolicy(true, domain.AutoAssignCandidate{
		Kind: domain.CandidateAccount, ID: assigneeID,
	})

	cmd := taskCommand()
	cmd.AssigneeID = assigneeID
	cmd.AutoAssign = true
	_, _, err := h.handler.Execute(t.Context(), itemActor(), cmd)
	assertValidation(t, err, "items.assignee_conflicts_auto_assign")
}

// The acceptance criterion of C-02: a policy with nobody eligible leaves the entry unassigned and
// says so in the result with a stable code, instead of failing the creation.
func TestNobodyEligibleStillCreatesTheEntry(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.withCollectionPolicy(true, domain.AutoAssignCandidate{
		Kind: domain.CandidateAccount, ID: strangerID,
	})

	item, outcome, err := h.handler.Execute(t.Context(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("an empty pool must not fail the creation: %v", err)
	}
	if len(h.items.inserted) != 1 || !item.AssigneeID.IsZero() {
		t.Fatalf("the entry: inserted %d, assignee %q - want created and unassigned",
			len(h.items.inserted), item.AssigneeID)
	}
	if outcome == nil || outcome.Assigned || outcome.Code != "items.auto_assign_no_candidate" {
		t.Fatalf("outcome %+v, want the stable code", outcome)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemCreated {
		t.Errorf("events %+v, want only the creation", h.events.appended)
	}
}

// The channel adapter carries the outcome beside the entry, exactly as :auto-assign does.
func TestTheCreateChannelCarriesTheOutcome(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.withCollectionPolicy(true, domain.AutoAssignCandidate{
		Kind: domain.CandidateAccount, ID: assigneeID,
	})

	out, err := h.handler.Descriptor().Handler.Invoke(t.Context(), itemActor(), map[string]any{
		"type": "TASK", "collection_id": collectionID.String(), "title": "Buy milk",
		"auto_assign": true,
	})
	if err != nil {
		t.Fatalf("the channel refused: %v", err)
	}
	outcome, ok := out["auto_assign"].(map[string]any)
	if !ok || outcome["assigned"] != true || outcome["strategy"] != "FIXED" {
		t.Fatalf("the output carries %+v", out["auto_assign"])
	}
	if out.String("assignee_id") != assigneeID.String() {
		t.Errorf("assignee_id = %v", out["assignee_id"])
	}
}
