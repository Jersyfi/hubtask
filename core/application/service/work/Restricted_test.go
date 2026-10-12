// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// No rule puts a person whose processing is restricted on an entry (UC-PRV-04 check 4,
// data-protection.md §4); a person doing it by hand is not stopped (check 2). An explicit
// assignment is refused with a code the engine records as SKIPPED; a create or a copy that would
// carry the person is made without them.

type restrictedOnes []shared.ID

func (r restrictedOnes) Restricted(_ context.Context, ids []shared.ID) (map[shared.ID]bool, error) {
	answer := map[shared.ID]bool{}
	for _, id := range ids {
		if slices.Contains(r, id) {
			answer[id] = true
		}
	}
	return answer, nil
}

var ruleID = shared.ID("01936f2a-7c1e-7000-8000-0000000007e1")

// underRule is the actor the engine builds for one action of a rule.
func underRule(actor appshared.ActorContext) appshared.ActorContext {
	actor.Kind, actor.RunsForRule = appshared.ActorServiceAccount, ruleID
	return actor
}

func refusedAsRestricted(t *testing.T, err error) {
	t.Helper()
	if got := shared.AsError(err); got == nil || got.DetailCode != AssigneeRestrictedCode ||
		got.Category != shared.CategoryValidation {
		t.Errorf("answered %v, want %s", err, AssigneeRestrictedCode)
	}
}

func TestARuleAssignsNobodyRestricted(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	writer := h.assign.Assignment
	writer.Accounts = restrictedOnes{assigneeID}
	assign := AssignWorkItem{Assignment: writer}

	_, err := assign.Execute(t.Context(), underRule(actor()), assignCmd())
	refusedAsRestricted(t, err)
	if len(h.items.assignments) != 0 {
		t.Errorf("%d assignments written under a rule", len(h.items.assignments))
	}

	// By hand it goes through.
	item, err := assign.Execute(t.Context(), actor(), assignCmd())
	if err != nil || item.AssigneeID != assigneeID {
		t.Errorf("by hand: %v, on %q", err, item.AssigneeID)
	}
}

func TestARuleAddsNobodyRestrictedAsAMember(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	writer := h.add.Writer
	writer.Accounts = restrictedOnes{assigneeID}
	add := AddMember{Writer: writer}

	_, err := add.Execute(t.Context(), underRule(actor()), memberCmd())
	refusedAsRestricted(t, err)
	if carried, _ := h.itemMembers.List(t.Context(), assignedItem); len(carried) != 0 {
		t.Errorf("the entry carries %v after a refused rule", carried)
	}

	set, err := add.Execute(t.Context(), actor(), memberCmd())
	if err != nil || !slices.Contains(set.AccountIDs, assigneeID) {
		t.Errorf("by hand: %v, %v", err, set)
	}
}

func TestARuleCreatesAnEntryForARestrictedPersonUnassigned(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.handler.AutoAssign.Assignment.Accounts = restrictedOnes{assigneeID}

	cmd := taskCommand()
	cmd.AssigneeID = assigneeID
	item, _, err := h.handler.Execute(t.Context(), underRule(itemActor()), cmd)
	if err != nil {
		t.Fatalf("creating under a rule: %v", err)
	}
	if !item.AssigneeID.IsZero() {
		t.Errorf("the entry is on %q, want nobody", item.AssigneeID)
	}

	item, _, err = h.handler.Execute(t.Context(), itemActor(), cmd)
	if err != nil || item.AssigneeID != assigneeID {
		t.Errorf("by hand: %v, on %q", err, item.AssigneeID)
	}
}

func TestARuleCopiesAnEntryOffARestrictedPerson(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	h.handler.Restrictions = restrictedOnes{colleagueAccountID}

	result, err := h.handler.Execute(t.Context(), underRule(actor()), DuplicateWorkItemCommand{ItemID: original.ID})
	if err != nil {
		t.Fatalf("copying under a rule: %v", err)
	}
	if !result.Item.AssigneeID.IsZero() {
		t.Errorf("the copy is on %q, want nobody", result.Item.AssigneeID)
	}
	reported := false
	for _, reference := range result.DroppedReferences {
		reported = reported || reference.Kind == domain.ReferenceAssignee && reference.Code == AssigneeRestrictedCode
	}
	if !reported {
		t.Errorf("the drop is not reported: %+v", result.DroppedReferences)
	}
}
