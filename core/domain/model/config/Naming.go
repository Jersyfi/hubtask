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
		if enabled, _ := rule["enabled"].(bool); enabled && takesStep(listOf(rule["actions"]), 0, func(kind string, params map[string]any) bool {
			return kind == "NOTIFY_ROLE" && stringOf(params["role"]) == role
		}) {
			count++
		}
	}
	return count
}

// RulesCreatingType counts a rules section's switched-on rules with a CREATE_WORK_ITEM step, in a
// branch or not, that creates an entry of the type: one naming its key as `entry_type`, or - for
// the level's own type (levelsOwn) - one of its level naming none, which CreateWorkItem gives the
// level's own. A step whose type a run fills in is not counted: what it creates is not known here.
func RulesCreatingType(rules Section, level, key string, levelsOwn bool) int {
	count := 0
	for _, rule := range rules {
		if enabled, _ := rule["enabled"].(bool); enabled && takesStep(listOf(rule["actions"]), 0, func(kind string, params map[string]any) bool {
			if kind != "CREATE_WORK_ITEM" {
				return false
			}
			named := stringOf(params["entry_type"])
			return named == key || (levelsOwn && named == "" && stringOf(params["type"]) == level)
		}) {
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

// takesStep says a step of the actions, in a branch or not, is one the predicate holds for.
func takesStep(actions []any, depth int, holds func(kind string, params map[string]any) bool) bool {
	if depth > maxBranchNesting {
		return false
	}
	for _, raw := range actions {
		action := mapOf(raw)
		params := mapOf(action["params"])
		kind := stringOf(action["kind"])
		if kind == "BRANCH" {
			if takesStep(listOf(params["then"]), depth+1, holds) || takesStep(listOf(params["else"]), depth+1, holds) {
				return true
			}
			continue
		}
		if holds(kind, params) {
			return true
		}
	}
	return false
}
