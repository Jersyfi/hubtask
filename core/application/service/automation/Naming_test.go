// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Only switched-on rules count, and a step inside a branch names a role as one at the top does.
func TestRulesNamingCountsTheSwitchedOnRulesThatNameABlock(t *testing.T) {
	scope := domain.Scope{Type: domain.ScopeTenant}
	autoClose := ruleAt(scope, shared.ID("01936f2a-7c1e-7000-8000-0000000000c1"))
	autoClose.Enabled = true
	autoClose.Trigger = domain.Trigger{Kind: domain.TriggerRelativeDate, Calendar: "office"}
	paused := ruleAt(scope, shared.ID("01936f2a-7c1e-7000-8000-0000000000c2"))
	paused.Enabled = false
	paused.Trigger = domain.Trigger{Kind: domain.TriggerRelativeDate, Calendar: "office"}
	notify := ruleAt(scope, shared.ID("01936f2a-7c1e-7000-8000-0000000000c3"))
	notify.Enabled = true
	notify.Actions = []domain.Action{{Kind: domain.ActionBranch, Params: map[string]any{
		"condition": "item.type == 'TASK'",
		"then": []any{map[string]any{"kind": "NOTIFY_ROLE", "params": map[string]any{
			"role": "change_manager", "item_id": "x",
		}}},
	}}}
	naming := RulesNaming{Rules: newRuleStore(autoClose, paused, notify)}

	calendars, err := naming.Calendar(t.Context(), "office")
	if err != nil || calendars != 1 {
		t.Fatalf("calendar office named by %d rules (%v), want 1", calendars, err)
	}
	if other, _ := naming.Calendar(t.Context(), "weekend"); other != 0 {
		t.Fatalf("calendar weekend named by %d rules, want 0", other)
	}
	roles, err := naming.Role(t.Context(), "change_manager")
	if err != nil || roles != 1 {
		t.Fatalf("role named by %d rules (%v), want 1", roles, err)
	}
}
