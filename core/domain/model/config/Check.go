// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"slices"
	"strings"
)

// Severity is how much a finding of the configuration check weighs (configuration/ui.md A5).
type Severity string

const (
	// SeverityError is a configuration that refuses or strands the people who meet it.
	SeverityError Severity = "ERROR"
	// SeverityWarning is one that works, but not as it was most likely meant.
	SeverityWarning Severity = "WARNING"
)

// Finding is one thing the configuration check found: a message code, the block it is about as a
// key path (`workflows.incident`), and the parameters its message names.
type Finding struct {
	Severity Severity
	Code     string
	Path     string
	Params   map[string]string
}

// Checked is what the check reads: the workspace's document and, for a hub, the hub's own. Both
// are read as JSON reads them - objects, arrays, strings, booleans - so a check is a pure function
// of what any door would export.
type Checked struct {
	Workspace Document
	// Hub is nil when the workspace alone is checked.
	Hub Document
	// BuiltInRoles are the roles every workspace has; a step naming one may be taken.
	BuiltInRoles []string
}

// CheckDocument answers what the configuration check finds, errors first, each kind in key order.
//
// It finds before a person or an import meets the refusal (ui.md A5): a workflow whose entries can
// never be completed, a status no step leads to, a step only roles that do not exist may take, a
// rule that answers its own change, a hub's scheme naming a workflow the workspace does not hold.
// It never refuses anything itself: each finding is advice with the block that needs a change.
func CheckDocument(in Checked) []Finding {
	var findings []Finding
	roles := map[string]bool{}
	for _, role := range in.BuiltInRoles {
		roles[strings.ToUpper(role)] = true
	}
	for key := range in.Workspace["roles"] {
		roles[key] = true
	}
	workflows := in.Workspace["workflows"]
	for _, key := range sortedKeys(workflows) {
		findings = append(findings, checkWorkflow(key, workflows[key], roles)...)
	}
	findings = append(findings, checkRules("rules", in.Workspace["rules"])...)
	if in.Hub != nil {
		findings = append(findings, checkRules("rules", in.Hub["rules"])...)
		scheme := in.Hub["scheme"]
		for _, key := range sortedKeys(scheme) {
			named := stringOf(scheme[key]["workflow"])
			if named == "" || workflows == nil {
				continue
			}
			if _, held := workflows[named]; !held {
				findings = append(findings, Finding{Severity: SeverityError, Code: "config.check.scheme_workflow_unknown",
					Path: "scheme." + key, Params: map[string]string{"workflow": named}})
			}
		}
	}
	slices.SortStableFunc(findings, func(a, b Finding) int {
		if a.Severity != b.Severity {
			if a.Severity == SeverityError {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	return findings
}

// checkWorkflow walks a workflow's steps from its initial status.
func checkWorkflow(key string, block Block, roles map[string]bool) []Finding {
	shape, _ := block["shape"].(map[string]any)
	if shape == nil {
		return nil
	}
	path := "workflows." + key
	initial := stringOf(shape["initial"])
	type status struct{ key, category string }
	var statuses []status
	for _, raw := range listOf(shape["statuses"]) {
		each, _ := raw.(map[string]any)
		statuses = append(statuses, status{stringOf(each["key"]), stringOf(each["category"])})
	}
	type step struct {
		key, to string
		from    []string
		who     map[string]any
	}
	var steps []step
	for _, raw := range listOf(shape["steps"]) {
		each, _ := raw.(map[string]any)
		s := step{key: stringOf(each["key"]), to: stringOf(each["to"]), who: mapOf(each["who"])}
		for _, from := range listOf(each["from"]) {
			s.from = append(s.from, stringOf(from))
		}
		steps = append(steps, s)
	}

	var findings []Finding
	takeable := func(s step) bool {
		if s.who == nil {
			return true
		}
		if len(listOf(s.who["groups"])) > 0 || len(listOf(s.who["person_fields"])) > 0 || stringOf(s.who["condition"]) != "" {
			return true
		}
		named := listOf(s.who["roles"])
		if len(named) == 0 {
			return true
		}
		for _, role := range named {
			if roles[stringOf(role)] || roles[strings.ToUpper(stringOf(role))] {
				return true
			}
		}
		return false
	}
	for _, s := range steps {
		if !takeable(s) {
			findings = append(findings, Finding{Severity: SeverityError, Code: "config.check.step_nobody",
				Path: path, Params: map[string]string{"step": s.key}})
		}
	}

	// Reached from the initial status by the steps somebody may take.
	reached := map[string]bool{initial: true}
	for grew := true; grew; {
		grew = false
		for _, s := range steps {
			if reached[s.to] || !takeable(s) {
				continue
			}
			leaves := len(s.from) == 0
			for _, from := range s.from {
				leaves = leaves || reached[from]
			}
			if leaves {
				reached[s.to], grew = true, true
			}
		}
	}
	done := false
	for _, st := range statuses {
		if st.category == "DONE" && reached[st.key] {
			done = true
		}
	}
	if !done && len(statuses) > 0 {
		findings = append(findings, Finding{Severity: SeverityError, Code: "config.check.done_unreachable", Path: path})
	}
	for _, st := range statuses {
		if !reached[st.key] {
			findings = append(findings, Finding{Severity: SeverityWarning, Code: "config.check.status_unreachable",
				Path: path, Params: map[string]string{"status": st.key}})
		}
	}
	return findings
}

// entryChanging are the actions that change the entry a rule runs on, so that an `item.updated`
// rule taking one answers its own change until the depth limit stops it.
var entryChanging = []string{"UPDATE_WORK_ITEM", "TAKE_STEP", "ADD_LABEL", "REMOVE_LABEL", "ASSIGN_WORK_ITEM", "MOVE_WORK_ITEM"}

// checkRules finds a rule that reacts to every change of an entry by changing it again, with no
// condition and no narrowing to the fields that moved.
func checkRules(section string, rules Section) []Finding {
	var findings []Finding
	for _, key := range sortedKeys(rules) {
		rule := rules[key]
		if enabled, set := rule["enabled"].(bool); set && !enabled {
			continue
		}
		trigger := mapOf(rule["trigger"])
		if stringOf(trigger["kind"]) != "EVENT" || stringOf(trigger["event_type"]) != "item.updated" ||
			len(listOf(trigger["changed_fields"])) > 0 || len(listOf(rule["conditions"])) > 0 {
			continue
		}
		for _, raw := range listOf(rule["actions"]) {
			if slices.Contains(entryChanging, stringOf(mapOf(raw)["kind"])) {
				findings = append(findings, Finding{Severity: SeverityWarning, Code: "config.check.rule_may_loop",
					Path: section + "." + key, Params: map[string]string{"action": stringOf(mapOf(raw)["kind"])}})
				break
			}
		}
	}
	return findings
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func listOf(v any) []any {
	l, _ := v.([]any)
	return l
}

func mapOf(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
