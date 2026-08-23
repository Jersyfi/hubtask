// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	identitymodel "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	thirdID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000f4")
	groupOne = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	groupTwo = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
	policyID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
)

// groupStore fakes the group repository with the one method this use case asks: who is in a
// group, resolved at draw time.
type groupStore struct {
	members map[shared.ID][]shared.ID
}

func (g *groupStore) Find(context.Context, shared.ID) (identitymodel.Group, error) {
	return identitymodel.Group{}, shared.ErrNotFound
}
func (g *groupStore) Insert(context.Context, identitymodel.Group) error        { return nil }
func (g *groupStore) Update(context.Context, identitymodel.Group, int) error   { return nil }
func (g *groupStore) Delete(context.Context, shared.ID) error                  { return nil }
func (g *groupStore) AddMember(context.Context, shared.ID, shared.ID) error    { return nil }
func (g *groupStore) RemoveMember(context.Context, shared.ID, shared.ID) error { return nil }
func (g *groupStore) Members(_ context.Context, groupID shared.ID) ([]shared.ID, error) {
	return g.members[groupID], nil
}

type autoAssignHarness struct {
	*assignmentHarness
	auto     AutoAssignWorkItem
	policies *policyStore
	groups   *groupStore
	random   *clock.Scripted
}

func newAutoAssignHarness(t *testing.T, draws ...int) *autoAssignHarness {
	t.Helper()
	if len(draws) == 0 {
		draws = []int{0}
	}

	h := &autoAssignHarness{
		assignmentHarness: newAssignmentHarness(t),
		policies:          newPolicyStore(),
		groups:            &groupStore{members: map[shared.ID][]shared.ID{}},
		random:            clock.NewScripted(draws...),
	}
	h.auto = AutoAssignWorkItem{
		Assignment: h.assign.Assignment,
		Policies:   h.policies,
		Groups:     h.groups,
		Random:     h.random,
	}
	return h
}

// withPolicy configures the collection's policy the way the adapter would hand it up: the
// definition on the container read, the row in the policy store.
func (h *autoAssignHarness) withPolicy(
	strategy domain.AutoAssignStrategy, cursor int, candidates ...domain.AutoAssignCandidate,
) {
	definition := &domain.AutoAssignDefinition{
		Strategy: strategy, Candidates: candidates, Enabled: true,
	}
	collection := h.containers.stored[collectionID]
	collection.AutoAssign = definition
	h.containers.stored[collectionID] = collection

	h.policies.stored[collectionID] = domain.AutoAssignPolicy{
		ID: policyID, TenantID: tenantID,
		ScopeType: domain.AutoAssignScopeCollection, ScopeID: collectionID,
		Strategy: strategy, Candidates: candidates,
		State: domain.AutoAssignState{Cursor: cursor}, Enabled: true, Version: 1,
	}
}

func accounts(ids ...shared.ID) []domain.AutoAssignCandidate {
	candidates := make([]domain.AutoAssignCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, domain.AutoAssignCandidate{
			Kind: domain.CandidateAccount, ID: id,
		})
	}
	return candidates
}

func groupCandidates(ids ...shared.ID) []domain.AutoAssignCandidate {
	candidates := make([]domain.AutoAssignCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, domain.AutoAssignCandidate{
			Kind: domain.CandidateGroup, ID: id,
		})
	}
	return candidates
}

func autoCmd() AssignmentCommand { return AssignmentCommand{ItemID: assignedItem} }

func TestAutoAssigningWritesTheStrategyIntoTheEvent(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	h.withPolicy(domain.AssignFixed, 0, accounts(assigneeID)...)

	item, outcome, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("handing out failed: %v", err)
	}

	if item.AssigneeID != assigneeID {
		t.Errorf("the entry is on %q, want the fixed candidate", item.AssigneeID)
	}
	if !outcome.Assigned || outcome.Strategy != domain.AssignFixed || outcome.Code != "" {
		t.Errorf("outcome %+v, want an assignment by FIXED", outcome)
	}
	if len(h.items.assignments) != 1 {
		t.Fatalf("rows written: %d, want one", len(h.items.assignments))
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemAssigned {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if got := h.events.appended[0].Payload["strategy"]; got != "FIXED" {
		t.Errorf("the event carries strategy %v, want FIXED (domain-model.md §4)", got)
	}
	if len(h.changes.recorded) != 1 || len(h.audit.entries) != 1 || len(h.history.entries) != 1 {
		t.Errorf("%d changes, %d audit entries, %d history steps - want one of each",
			len(h.changes.recorded), len(h.audit.entries), len(h.history.entries))
	}
}

func TestACandidateWhoCannotSeeTheEntryIsSkipped(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	// strangerID holds no membership; only assigneeID is visible in the default harness.
	h.withPolicy(domain.AssignRandomMember, 0, accounts(strangerID, assigneeID)...)

	item, outcome, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("handing out failed: %v", err)
	}
	if item.AssigneeID != assigneeID || !outcome.Assigned {
		t.Fatalf("the entry is on %q (%+v), want the one visible candidate", item.AssigneeID, outcome)
	}
	if len(h.visibility.asked) != 2 {
		t.Errorf("eligibility asked about %v, want both candidates", h.visibility.asked)
	}
}

func TestNobodyEligibleLeavesTheEntryAsItWas(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	h.withPolicy(domain.AssignRandomMember, 0, accounts(strangerID)...)

	item, outcome, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("an empty pool must not fail the call: %v", err)
	}
	if outcome.Assigned || outcome.Code != "items.auto_assign_no_candidate" {
		t.Fatalf("outcome %+v, want unassigned with the stable code", outcome)
	}
	if !item.AssigneeID.IsZero() {
		t.Errorf("the entry landed on %q", item.AssigneeID)
	}
	if len(h.items.assignments)+len(h.events.appended)+len(h.changes.recorded) != 0 {
		t.Error("an empty pool still wrote something")
	}
}

func TestWithoutAPolicyTheAskIsRefused(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)

	_, _, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	assertValidation(t, err, "items.auto_assign_unavailable")
}

func TestTheRotationAdvancesUnderTheLockAndSkipsTheIneligible(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	h.visibility.reachable[thirdID] = true
	// Three configured; the cursor stands on strangerID, who cannot see the collection: the turn
	// passes to thirdID, and the cursor ends up one past them.
	h.withPolicy(domain.AssignRoundRobin, 1, accounts(assigneeID, strangerID, thirdID)...)

	item, outcome, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("handing out failed: %v", err)
	}
	if item.AssigneeID != thirdID || !outcome.Assigned {
		t.Fatalf("the entry is on %q (%+v), want the next eligible in the rotation",
			item.AssigneeID, outcome)
	}
	if len(h.policies.saved) != 1 || h.policies.saved[0].State.Cursor != 0 {
		t.Fatalf("rotation state saved: %+v, want the cursor one past the pick", h.policies.saved)
	}
	if got := h.events.appended[0].Payload["strategy"]; got != "ROUND_ROBIN" {
		t.Errorf("the event carries strategy %v", got)
	}
}

func TestGroupsAreResolvedAtDrawTime(t *testing.T) {
	// Draw 1 of 2 groups, then 0 of that group's eligible members.
	h := newAutoAssignHarness(t, 1, 0)
	h.withItem(domain.ItemTask)
	h.visibility.reachable[thirdID] = true
	h.groups.members[groupOne] = []shared.ID{strangerID} // nobody eligible: out of the draw
	h.groups.members[groupTwo] = []shared.ID{strangerID, thirdID, assigneeID}
	h.withPolicy(domain.AssignRandomGroupMember, 0, groupCandidates(groupOne, groupTwo)...)

	item, outcome, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("handing out failed: %v", err)
	}
	// groupOne fell out (nobody eligible), so the group draw is over one group and lands on
	// groupTwo whatever the script says; the member draw's 0 is its first eligible member.
	if item.AssigneeID != thirdID || !outcome.Assigned {
		t.Fatalf("the entry is on %q, want the drawn member of the one drawable group", item.AssigneeID)
	}
}

func TestLeastLoadedPicksTheEmptiestPlate(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	h.visibility.reachable[thirdID] = true
	h.items.load = map[shared.ID]int{assigneeID: 3, thirdID: 1}
	h.withPolicy(domain.AssignLeastLoaded, 0, accounts(assigneeID, thirdID)...)

	item, _, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("handing out failed: %v", err)
	}
	if item.AssigneeID != thirdID {
		t.Errorf("the entry is on %q, want the least loaded", item.AssigneeID)
	}
}

func TestThePolicyPickingTheCurrentAssigneeAnnouncesNothing(t *testing.T) {
	h := newAutoAssignHarness(t)
	item := h.withItem(domain.ItemTask)
	h.items.stored[assignedItem] = item.Assigned(assigneeID, now)
	h.withPolicy(domain.AssignRoundRobin, 0, accounts(assigneeID, thirdID)...)

	changed, outcome, err := h.auto.Execute(t.Context(), actor(), autoCmd())
	if err != nil {
		t.Fatalf("the repeat was refused: %v", err)
	}
	if !outcome.Assigned || changed.AssigneeID != assigneeID {
		t.Fatalf("outcome %+v on %q, want the standing assignment confirmed", outcome, changed.AssigneeID)
	}
	if len(h.events.appended) != 0 || len(h.items.assignments) != 0 {
		t.Error("a no-op wrote or announced something")
	}
	if len(h.policies.saved) != 0 {
		t.Error("a no-op advanced the rotation - no assignment happened to spend the turn on")
	}
}

// The channel adapter: the outcome travels beside the entry, in the contract's field names.
func TestTheAutoAssignChannelCarriesTheOutcome(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	h.withPolicy(domain.AssignFixed, 0, accounts(assigneeID)...)

	out, err := h.auto.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{
		"item_id": assignedItem.String(),
	})
	if err != nil {
		t.Fatalf("the channel refused: %v", err)
	}
	outcome, ok := out["auto_assign"].(map[string]any)
	if !ok || outcome["assigned"] != true || outcome["strategy"] != "FIXED" {
		t.Fatalf("the output carries %+v", out["auto_assign"])
	}
	if _, told := outcome["code"]; told {
		t.Error("a successful run carries a code")
	}
}

// The eligibility question is asked about the candidates, never about the actor - the same
// distinction the manual assignment's visibility fake exists to catch.
func TestEligibilityIsAskedAboutTheCandidates(t *testing.T) {
	h := newAutoAssignHarness(t)
	h.withItem(domain.ItemTask)
	h.withPolicy(domain.AssignFixed, 0, accounts(assigneeID)...)

	if _, _, err := h.auto.Execute(t.Context(), actor(), autoCmd()); err != nil {
		t.Fatal(err)
	}
	if len(h.visibility.asked) != 1 || h.visibility.asked[0] != assigneeID {
		t.Errorf("eligibility asked about %v, want the candidate", h.visibility.asked)
	}
}
