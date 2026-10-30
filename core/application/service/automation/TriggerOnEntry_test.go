// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func macroHarness(scope domain.Scope) *triggerHarness {
	rule := manualRule()
	rule.Scope = scope
	h := newTrigger(rule)
	h.handler.Entries = entries{rows: map[shared.ID]work.WorkItem{
		itemID: {ID: itemID, TenantID: tenant, CollectionID: collectionID},
	}}
	h.handler.Containers = &containers{rows: map[shared.ID]work.Container{
		collectionID: {ID: collectionID, TenantID: tenant, Type: work.ContainerCollection, ParentID: hubID},
	}}
	return h
}

// UC-SVC-06 check 4: a macro is a MANUAL rule pressed on an entry - the run is about that entry,
// the presser must be allowed to change it, and an entry outside the rule's scope is refused.
func TestAMacroRunsAboutTheEntryItIsPressedOn(t *testing.T) {
	h := macroHarness(domain.Scope{Type: domain.ScopeHub, ID: hubID})
	if _, err := h.handler.ExecuteOn(context.Background(), presser(), ruleID, itemID); err != nil {
		t.Fatalf("pressing the macro: %v", err)
	}
	if got, _ := h.queued.queued[0].Payload["subject_id"].(string); got != itemID.String() {
		t.Errorf("the run is about %q, want the entry", got)
	}
	asked := h.auth.requests[len(h.auth.requests)-1]
	if asked.Permission != service.PermissionWriteItems || asked.TargetID != itemID || asked.On.Does != service.ItemChange {
		t.Errorf("asked %+v, want changing the entry", asked)
	}

	outside := macroHarness(domain.Scope{Type: domain.ScopeCollection, ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000ff")})
	_, err := outside.handler.ExecuteOn(context.Background(), presser(), ruleID, itemID)
	var refused *shared.Error
	if !errors.As(err, &refused) || refused.DetailCode != "automation.subject_outside_scope" {
		t.Errorf("an entry outside the rule's scope: %v", err)
	}
	if len(outside.queued.queued) != 0 {
		t.Error("a refused press queued a run")
	}

	refusing := macroHarness(domain.Scope{Type: domain.ScopeTenant})
	refusing.auth.refuse = true
	if _, err := refusing.handler.ExecuteOn(context.Background(), presser(), ruleID, itemID); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a presser who may not change the entry: %v", err)
	}
	missing := macroHarness(domain.Scope{Type: domain.ScopeTenant})
	other := shared.ID("01936f2a-7c1e-7000-8000-0000000000fe")
	if _, err := missing.handler.ExecuteOn(context.Background(), presser(), ruleID, other); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an entry that does not exist: %v", err)
	}
}
