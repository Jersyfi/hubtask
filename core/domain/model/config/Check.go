// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"fmt"
	"slices"
	"strconv"
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
	// Channels are every door an entry arrives through; a field each of them hides is filled by
	// nobody.
	Channels []string
}

// CheckDocument answers what the configuration check finds, errors first, each kind in key order.
//
// It finds before a person or an import meets the refusal (ui.md A5): a workflow whose entries can
// never be completed, a status no step leads to, a step only roles that do not exist may take, a
// rule that answers its own change, a hub's scheme naming a workflow the workspace does not hold,
// a required field no channel can fill, two fields with one meaning, and a lock stated alike in
// several collections that the hub could state once.
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
	findings = append(findings, checkRequiredFields(in.Workspace, in.Channels)...)
	findings = append(findings, checkSameMeaning(in.Workspace["fields"])...)
	if in.Hub != nil {
		findings = append(findings, checkRules("rules", in.Hub["rules"])...)
		findings = append(findings, checkLocks(in.Hub)...)
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

// checkRequiredFields finds a required custom field of a type's level that every channel of the
// type's form hides, with no preset and no default to start it from: nobody can give it a value,
// so no entry of the type gets past its first column (MissingRequiredFields).
func checkRequiredFields(document Document, channels []string) []Finding {
	if len(channels) == 0 {
		return nil
	}
	var findings []Finding
	fields := document["fields"]
	types := document["types"]
	for _, typeKey := range sortedKeys(types) {
		entryType := types[typeKey]
		form := mapOf(entryType["form"])
		if form == nil {
			continue
		}
		level := stringOf(entryType["level"])
		rules := map[string]map[string]any{}
		for _, raw := range listOf(form["channels"]) {
			rule := mapOf(raw)
			rules[stringOf(rule["channel"])] = rule
		}
		defaults := map[string]bool{}
		for _, raw := range listOf(form["fields"]) {
			field := mapOf(raw)
			if field["default"] != nil {
				defaults[stringOf(field["key"])] = true
			}
		}
		for _, fieldKey := range sortedKeys(fields) {
			field := fields[fieldKey]
			if required, _ := field["is_required"].(bool); !required || !slices.Contains(listOf(field["applies_to"]), any(level)) {
				continue
			}
			formKey := "custom_fields." + fieldKey
			fillable := defaults[formKey]
			for _, channel := range channels {
				rule := rules[channel]
				_, preset := mapOf(rule["presets"])[formKey]
				if rule == nil || preset || !slices.Contains(listOf(rule["hidden"]), any(formKey)) {
					fillable = true
				}
			}
			if !fillable {
				findings = append(findings, Finding{Severity: SeverityError, Code: "config.check.required_unfillable",
					Path: "types." + typeKey, Params: map[string]string{"field": fieldKey}})
			}
		}
	}
	return findings
}

// checkSameMeaning finds two fields of one kind offering the same choices under different keys:
// the duplicate a local field becomes when a shared one of its meaning exists (model.md §6), which
// splits every report and filter over the meaning in two.
func checkSameMeaning(fields Section) []Finding {
	var findings []Finding
	first := map[string]string{}
	for _, key := range sortedKeys(fields) {
		field := fields[key]
		var options []string
		for _, option := range listOf(field["options"]) {
			options = append(options, strings.ToLower(strings.TrimSpace(stringOf(option))))
		}
		if len(options) == 0 {
			continue
		}
		slices.Sort(options)
		meaning := stringOf(field["kind"]) + "\x00" + strings.Join(options, "\x00")
		if held, seen := first[meaning]; seen {
			findings = append(findings, Finding{Severity: SeverityWarning, Code: "config.check.fields_same_meaning",
				Path: "fields." + held, Params: map[string]string{"field": held, "other": key}})
			continue
		}
		first[meaning] = key
	}
	return findings
}

// checkLocks finds one tolerance locked alike in several of a hub's collections while the hub
// leaves it open: one lock on the hub states it once and keeps the collections in step.
func checkLocks(hub Document) []Finding {
	for _, block := range hub["tolerance"] {
		if locked, _ := block["lock"].(bool); locked {
			return nil
		}
	}
	counts := map[string]int{}
	for key, block := range hub["collection_tolerance"] {
		if locked, _ := block["lock"].(bool); !locked || !strings.HasSuffix(key, ".time_tolerance_days") {
			continue
		}
		counts[valueText(block["value"])]++
	}
	var findings []Finding
	for _, value := range sortedKeys(counts) {
		if counts[value] < 2 {
			continue
		}
		findings = append(findings, Finding{Severity: SeverityWarning, Code: "config.check.lock_could_move_up",
			Path: "tolerance.time_tolerance_days", Params: map[string]string{"value": value, "collections": strconv.Itoa(counts[value])}})
	}
	return findings
}

func valueText(v any) string {
	if number, ok := v.(float64); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprint(v)
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
