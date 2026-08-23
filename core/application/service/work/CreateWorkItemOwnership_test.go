// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The decision on issue #84: a role whose writes are narrowed to what is assigned to it may create,
// and what it creates is its own. Without that, the entry would be out of its creator's reach the
// moment it existed.

func TestCreatingOnBehalfOfARoleThatWritesOnlyItsOwn(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()
	h.authorizer.onlyOwn = true

	item, outcome, err := h.handler.Execute(t.Context(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("creating was refused: %v", err)
	}
	if item.AssigneeID != accountID {
		t.Errorf("the entry is on %q, want its creator", item.AssigneeID)
	}
	if outcome != nil {
		t.Errorf("outcome %+v, want none - the assignment was not a policy's", outcome)
	}
}

// The auto-assignment is not optional, and it is not a correction either: naming somebody else is
// refused rather than quietly rewritten, because a create that landed somewhere the client did not
// ask for is worse than one that did not happen.
func TestSuchARoleMayNotNameAnybodyElse(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(cmd *CreateWorkItemCommand)
	}{
		{"a second person", func(cmd *CreateWorkItemCommand) { cmd.AssigneeID = assigneeID }},
		// A policy can land the entry on anybody, which is the same refusal by a different route.
		{"the collection's policy", func(cmd *CreateWorkItemCommand) { cmd.AutoAssign = true }},
	}

	for _, c := range cases {
		h := newItemHarness()
		h.withAssignment()
		h.authorizer.onlyOwn = true

		cmd := taskCommand()
		c.prepare(&cmd)

		_, _, err := h.handler.Execute(t.Context(), itemActor(), cmd)
		if !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s: error %v, want forbidden", c.name, err)
		}
		if got := shared.AsError(err).DetailCode; got != "items.assignee_must_be_the_creator" {
			t.Errorf("%s: detail code %s", c.name, got)
		}
		if len(h.items.stored) != 0 {
			t.Errorf("%s: an entry was written despite the refusal", c.name)
		}
	}

	// Naming oneself is what the rule already produces, so it is accepted rather than refused for
	// asking for exactly what it gets.
	h := newItemHarness()
	h.withAssignment()
	h.authorizer.onlyOwn = true

	cmd := taskCommand()
	cmd.AssigneeID = accountID
	if _, _, err := h.handler.Execute(t.Context(), itemActor(), cmd); err != nil {
		t.Errorf("naming oneself was refused: %v", err)
	}
}

// Every other role creates as it always did: on nobody unless it says otherwise.
func TestAnUnqualifiedRoleCreatesOnNobody(t *testing.T) {
	h := newItemHarness()
	h.withAssignment()

	item, _, err := h.handler.Execute(t.Context(), itemActor(), taskCommand())
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if !item.AssigneeID.IsZero() {
		t.Errorf("the entry landed on %q, want nobody", item.AssigneeID)
	}
}

// The permission question a creation asks names no entry: there is nothing yet to share or to
// assign, so the path ends at the container it would be created under.
func TestTheCreationNamesNoEntry(t *testing.T) {
	h := newItemHarness()

	if _, _, err := h.handler.Execute(t.Context(), itemActor(), taskCommand()); err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if len(h.authorizer.requests) == 0 {
		t.Fatal("no permission question was asked")
	}

	request := h.authorizer.requests[0]
	if request.On.Does != service.ItemCreate {
		t.Errorf("the request does %q, want a creation", request.On.Does)
	}
	if !request.On.ID.IsZero() {
		t.Errorf("the creation named entry %q", request.On.ID)
	}
}
