// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func portalActor() appshared.ActorContext {
	a := actor()
	a.Channel = string(domain.ChannelPortal)
	return a
}

// UC-SVC-05 check 5: on the portal's door a requester answers on their own request with REQUEST,
// always publicly, and never on anybody else's or under an internal note.
func TestARequesterAnswersOnTheirOwnRequestPublicly(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)

	cmd := addCmd("It still does not print.")
	cmd.Internal = true
	comment, err := AddComment{Writer: h.writer}.Execute(t.Context(), portalActor(), cmd)
	if err != nil {
		t.Fatalf("commenting failed: %v", err)
	}
	if comment.Internal {
		t.Error("a requester wrote an internal note")
	}
	if asked := h.authorizer.requests[len(h.authorizer.requests)-1]; asked.Permission != service.PermissionRequest || asked.On.Does != "" {
		t.Errorf("the portal asked %+v", asked)
	}

	note := h.withComment("0190a4c2-7b1e-7000-8000-0000000000aa", "0190a4c2-7b1e-7000-8000-0000000000bb", "Check the switch.")
	note.Internal = true
	h.comments.stored[note.ID] = note
	reply := addCmd("What switch?")
	reply.ParentCommentID = note.ID
	if _, err := (AddComment{Writer: h.writer}).Execute(t.Context(), portalActor(), reply); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a requester answered an internal note: %v", err)
	}

	other := h.items.stored[assignedItem]
	other.CreatedBy = "0190a4c2-7b1e-7000-8000-0000000000cc"
	h.items.stored[assignedItem] = other
	if _, err := (AddComment{Writer: h.writer}).Execute(t.Context(), portalActor(), addCmd("Me too.")); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a requester commented on somebody else's request: %v", err)
	}
}

// UC-SVC-05 check 2: a request raised on the portal's door is created with REQUEST at the
// collection, in the requester's name; every other door still asks WRITE_ITEMS.
func TestARequestRaisedOnThePortalAsksRequest(t *testing.T) {
	h := newItemHarness()
	requester := itemActor()
	requester.Channel = string(domain.ChannelPortal)
	item, _, err := h.handler.Execute(t.Context(), requester, taskCommand())
	if err != nil {
		t.Fatalf("raising failed: %v", err)
	}
	if item.CreatedBy != requester.AccountID {
		t.Errorf("raised in the name of %s", item.CreatedBy)
	}
	if asked := h.authorizer.requests[0]; asked.Permission != service.PermissionRequest || asked.On.Does != "" {
		t.Errorf("the portal's creation asked %+v", asked)
	}

	app := newItemHarness()
	if _, _, err := app.handler.Execute(t.Context(), itemActor(), taskCommand()); err != nil {
		t.Fatal(err)
	}
	if asked := app.authorizer.requests[0]; asked.Permission != service.PermissionWriteItems {
		t.Errorf("the app's creation asked %+v", asked)
	}
}
