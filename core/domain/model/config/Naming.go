// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

// maxBranchNesting bounds the walk through a rule's branches; the rule's own validation holds
// them to fewer.
const maxBranchNesting = 8

// RulesAddressingRole counts a rules section's switched-on rules with a NOTIFY_ROLE step, in a
// branch or not, addressing the role - what DeleteRole counts. Only `enabled: true` counts: a
// rule the apply creates without it stays off.
func RulesAddressingRole(rules Section, role string) int {
	count := 0
	for _, rule := range rules {
		if enabled, _ := rule["enabled"].(bool); enabled && notifiesRole(listOf(rule["actions"]), role, 0) {
			count++
		}
	}
	return count
}

// PoliciesNamingCalendar counts the SLA policies of a document's policy section that name the
// calendar: a calendar they name may not go (DeleteBusinessCalendar refuses it).
func PoliciesNamingCalendar(policies Section, calendar string) int {
	count := 0
	for _, policy := range policies {
		if stringOf(policy["calendar"]) == calendar {
			count++
		}
	}
	return count
}

func notifiesRole(actions []any, role string, depth int) bool {
	if depth > maxBranchNesting {
		return false
	}
	for _, raw := range actions {
		action := mapOf(raw)
		params := mapOf(action["params"])
		switch stringOf(action["kind"]) {
		case "BRANCH":
			if notifiesRole(listOf(params["then"]), role, depth+1) || notifiesRole(listOf(params["else"]), role, depth+1) {
				return true
			}
		case "NOTIFY_ROLE":
			if stringOf(params["role"]) == role {
				return true
			}
		}
	}
	return false
}
