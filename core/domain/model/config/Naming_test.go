// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import "testing"

func notifyRole(role string) any {
	return map[string]any{"kind": "NOTIFY_ROLE", "params": map[string]any{"role": role}}
}

// configuration.md §2: DeleteRole refuses a role a switched-on rule's NOTIFY_ROLE step addresses,
// in a branch or not; the plan counts the rules of the document it sets the same way.
func TestRulesAddressingARoleAreThoseSwitchedOnThatNotifyIt(t *testing.T) {
	rules := Section{
		"direct": {"enabled": true, "actions": []any{notifyRole("desk_agent")}},
		"branched": {"enabled": true, "actions": []any{map[string]any{"kind": "BRANCH", "params": map[string]any{
			"condition": "true", "then": []any{map[string]any{"kind": "LOG"}},
			"else": []any{notifyRole("desk_agent")},
		}}}},
		// Switched off, or never switched on: DeleteRole does not count it.
		"off":     {"enabled": false, "actions": []any{notifyRole("desk_agent")}},
		"unset":   {"actions": []any{notifyRole("desk_agent")}},
		"other":   {"enabled": true, "actions": []any{notifyRole("approver")}},
		"removed": nil,
	}
	if got := RulesAddressingRole(rules, "desk_agent"); got != 2 {
		t.Errorf("counted %d rules, want 2", got)
	}
	if got := RulesAddressingRole(rules, "nobody"); got != 0 {
		t.Errorf("counted %d rules for an unaddressed role", got)
	}
	if got := RulesAddressingRole(nil, "desk_agent"); got != 0 {
		t.Errorf("counted %d rules in no section", got)
	}
}
