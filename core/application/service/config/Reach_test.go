// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

func serviceShape(statuses ...string) map[string]any {
	rows := []any{}
	for _, key := range statuses {
		rows = append(rows, map[string]any{"key": key, "name": key, "category": "IN_PROGRESS"})
	}
	return map[string]any{"statuses": rows}
}

// UC-CFG-01 check 4, with the tests below.
// configuration.md §2: a plan says what a workflow's new shape reaches - the entries it moves,
// taken from the publish preview of the shape the plan sets, before anything is saved.
func TestAPlanNamesTheEntriesAWorkflowsNewShapeMoves(t *testing.T) {
	f := newFixture()
	f.world.workflows = map[string]map[string]any{
		"service": {"key": "service", "name": "Service", "published": serviceShape("todo", "doing")},
	}
	f.world.preview = usecase.Output{
		"entries": 7, "removed": []usecase.Output{},
		"moves": []usecase.Output{{"collection_id": "0190a8f0-0000-7000-8000-0000000000c1", "entry_type": "task", "from": "doing", "to": "todo", "count": 7}},
	}
	shape := serviceShape("todo", "review")
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": shape}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "workflows.service" || reached[0]["entries"] != 7 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewWorkflowPublish"]
	if len(asked) != 1 || asked[0]["key"] != "service" || asked[0]["shape"] == nil {
		t.Fatalf("the preview was not asked for the plan's shape: %v", asked)
	}
	if plan["plan_hash"] == nil || len(f.world.calls) != 0 {
		t.Errorf("plan %v wrote %v", plan["plan_hash"], f.world.calls)
	}
}

// S-09's gap: a reader the preview refuses - an agent's token with `config:plan` but not the
// preview's scope - gets a plan that says the reach was not counted, rather than one silent on it.
func TestAPlanSaysWhenItsReachWasWithheld(t *testing.T) {
	f := newFixture()
	f.world.workflows = map[string]map[string]any{
		"service": {"key": "service", "name": "Service", "published": serviceShape("todo", "doing")},
	}
	f.world.previewWithheld = true
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": serviceShape("todo", "review")}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "workflows.service" || reached[0]["withheld"] != true {
		t.Fatalf("reach %v", reached)
	}
	if _, counted := reached[0]["entries"]; counted {
		t.Errorf("a withheld reach was counted: %v", reached[0])
	}
	if plan["plan_hash"] == nil {
		t.Error("a withheld reach refused the plan")
	}
}

// A shape that removes a status entries are in is refused at the plan, with the status and its
// count, as its publish would refuse the apply.
func TestAPlanRefusesAShapeRemovingAStatusInUse(t *testing.T) {
	f := newFixture()
	f.world.workflows = map[string]map[string]any{
		"service": {"key": "service", "name": "Service", "published": serviceShape("todo", "waiting")},
	}
	f.world.preview = usecase.Output{"entries": 0, "moves": []usecase.Output{}, "removed": []usecase.Output{{"status": "waiting", "count": 12}}}
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": serviceShape("todo")}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	refusals := plan["refusals"].([]usecase.Output)
	if len(refusals) != 1 || refusals[0]["code"] != "workflows.mapping_required" || plan["plan_hash"] != nil {
		t.Fatalf("refusals %v, hash %v", refusals, plan["plan_hash"])
	}
	if params := refusals[0]["params"].(map[string]string); params["status"] != "waiting" || params["count"] != "12" {
		t.Errorf("the refusal does not name the status and its count: %v", params)
	}
}

// configuration.md §2: a plan says which people a role's change gives or takes a permission -
// how many hold it directly and as a group, and the permissions gained and lost.
func TestAPlanNamesThePeopleARolesChangeReaches(t *testing.T) {
	f := newFixture()
	f.world.roles["desk_agent"] = map[string]any{"key": "desk_agent", "name": "Desk agent", "permissions": []any{"READ", "COMMENT"}}
	f.world.grants = []any{
		map[string]any{"role": "desk_agent", "account_id": "0190a8f0-0000-7000-8000-0000000000d1"},
		map[string]any{"role": "desk_agent", "account_id": "0190a8f0-0000-7000-8000-0000000000d2"},
		map[string]any{"role": "desk_agent", "group_id": "0190a8f0-0000-7000-8000-0000000000e1"},
		map[string]any{"role": "other", "account_id": "0190a8f0-0000-7000-8000-0000000000d3"},
	}
	document := map[string]any{"roles": map[string]any{"desk_agent": map[string]any{"permissions": []any{"READ", "APPROVE"}}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["accounts"] != 2 || reached[0]["groups"] != 1 {
		t.Fatalf("reach %v", reached)
	}
	gained, lost := reached[0]["gained"].([]string), reached[0]["lost"].([]string)
	if len(gained) != 1 || gained[0] != "APPROVE" || len(lost) != 1 || lost[0] != "COMMENT" {
		t.Errorf("gained %v, lost %v", gained, lost)
	}
}

// configuration.md §2: a plan that removes a role somebody holds is refused at the plan, with the
// count of its grants, as DeleteRole refuses the apply - not first found out half-way.
func TestAPlanRefusesRemovingARoleSomebodyHolds(t *testing.T) {
	f := newFixture()
	f.world.roles["desk_agent"] = map[string]any{"key": "desk_agent", "name": "Desk agent", "permissions": []any{"READ"}}
	f.world.grants = []any{
		map[string]any{"role": "desk_agent", "account_id": "0190a8f0-0000-7000-8000-0000000000d1"},
		map[string]any{"role": "desk_agent", "group_id": "0190a8f0-0000-7000-8000-0000000000e1"},
	}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{"roles": map[string]any{"desk_agent": nil}}})

	refusals, _ := plan["refusals"].([]usecase.Output)
	if len(refusals) != 1 || refusals[0]["code"] != "roles.held" || refusals[0]["path"] != "/roles/desk_agent" || plan["plan_hash"] != nil {
		t.Fatalf("refusals %v, hash %v", refusals, plan["plan_hash"])
	}
	if params := refusals[0]["params"].(map[string]string); params["grants"] != "2" || params["key"] != "desk_agent" {
		t.Errorf("the refusal does not count the grants: %v", params)
	}

	// Nobody holds it: the removal plans.
	f.world.grants = []any{}
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{"roles": map[string]any{"desk_agent": nil}}})
	if plan["plan_hash"] == nil {
		t.Errorf("an unheld role's removal was refused: %v", plan["refusals"])
	}
}

// configuration.md §2: a plan that removes a role a switched-on rule it keeps still addresses is
// refused at the plan with the count of those rules, as DeleteRole refuses the apply; a plan that
// switches the rule off or removes it with the role plans, since the apply runs those first.
func TestAPlanRefusesRemovingARoleARuleItKeepsAddresses(t *testing.T) {
	f := newFixture()
	f.world.roles["desk_agent"] = map[string]any{"key": "desk_agent", "name": "Desk agent", "permissions": []any{"READ"}}
	f.world.grants = []any{}
	f.world.rules = map[string]map[string]any{
		"tell_desk": {"id": existingRuleID, "key": "tell_desk", "scope": map[string]any{"type": "TENANT"}, "name": "Tell the desk",
			"enabled": true, "run_as": "0190a8f0-0000-7000-8000-00000000cccc", "trigger": map[string]any{"kind": "MANUAL"},
			"conditions": []any{}, "throttle": map[string]any{}, "on_error": "STOP",
			"actions": []any{map[string]any{"kind": "NOTIFY_ROLE", "params": map[string]any{"role": "desk_agent"}}}},
	}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{"roles": map[string]any{"desk_agent": nil}}})

	refusals, _ := plan["refusals"].([]usecase.Output)
	if len(refusals) != 1 || refusals[0]["code"] != "roles.named_by_rules" || refusals[0]["path"] != "/roles/desk_agent" || plan["plan_hash"] != nil {
		t.Fatalf("refusals %v, hash %v", refusals, plan["plan_hash"])
	}
	if params := refusals[0]["params"].(map[string]string); params["rules"] != "1" || params["key"] != "desk_agent" {
		t.Errorf("the refusal does not count the rules: %v", params)
	}

	// The rule goes with the role: the removal plans.
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"roles": map[string]any{"desk_agent": nil}, "rules": map[string]any{"tell_desk": nil},
	}})
	if plan["plan_hash"] == nil {
		t.Errorf("a removal taking the rule along was refused: %v", plan["refusals"])
	}
}

// S-09's gap: a plan that removes a field, or hides it from readers, says how many entries hold a
// value under it - what would stop being shown - asked by the field's identifier.
func TestAPlanCountsTheEntriesAFieldItRemovesHolds(t *testing.T) {
	f := newFixture()
	f.world.fields = map[string]map[string]any{
		"severity": {"id": "0190a8f0-0000-7000-8000-0000000000f1", "key": "severity", "kind": "TEXT", "applies_to": []any{"TASK"}},
	}
	f.world.fieldReach = 4
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"fields": map[string]any{}}, "mode": "REPLACE",
	})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "fields.severity" || reached[0]["hidden_values"] != 4 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewCustomFieldReach"]
	if len(asked) != 1 || asked[0]["field_id"] != "0190a8f0-0000-7000-8000-0000000000f1" {
		t.Fatalf("the count was asked %v", asked)
	}
}

// S-09's gap: a plan that changes the workspace's own structure says what it would stop showing in
// every hub and collection that follows the workspace, from the workspace's structure preview.
func TestAPlanCountsWhatTheWorkspacesStructureWouldStopShowing(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{"completion": "MANUAL"}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"structure": map[string]any{"capture": map[string]any{"value": "INBOX"}}},
	})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "structure" ||
		reached[0]["hidden_entries"] != 3 || reached[0]["hidden_values"] != 5 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewWorkspaceStructure"]
	if len(asked) != 1 || asked[0]["container_id"] != nil {
		t.Fatalf("the workspace's preview was asked %v", asked)
	}
	if settings, _ := asked[0]["settings"].(map[string]any); settings["capture"] != "INBOX" {
		t.Errorf("the preview was asked for %v", asked[0])
	}
	if f.world.inputs["PreviewContainerStructure"] != nil {
		t.Errorf("a hub's preview was asked for the workspace: %v", f.world.inputs["PreviewContainerStructure"])
	}
}

// S-09's gap: a plan that switches a module off says how many people it leaves without it and how
// many switched-on rules go quiet, asked at the plan's place; a switch on is not counted.
func TestAPlanCountsThePeopleAModuleSwitchedOffLeaves(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"modules": map[string]any{"workflows": map[string]any{"switch": "OFF"}}},
	})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "modules.workflows" || reached[0]["people"] != 7 ||
		reached[0]["rules"] != 2 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewModuleReach"]
	if len(asked) != 1 || asked[0]["module"] != "WORKFLOWS" || asked[0]["hub_id"] != nil || asked[0]["switch"] != "OFF" {
		t.Fatalf("the count was asked %v", asked)
	}

	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"modules": map[string]any{"workflows": map[string]any{"switch": "INHERIT"}}},
	})
	asked = f.world.inputs["PreviewModuleReach"]
	if reached, _ := plan["reach"].([]usecase.Output); len(reached) != 1 || len(asked) != 2 || asked[1]["switch"] != "INHERIT" {
		t.Fatalf("a module left to the level above was not counted: %v, asked %v", reached, asked)
	}

	f.world.previewWithheld = true
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"modules": map[string]any{"workflows": map[string]any{"switch": "OFF"}}},
	})
	if reached, _ := plan["reach"].([]usecase.Output); len(reached) != 1 || reached[0]["withheld"] != true {
		t.Errorf("a withheld count was answered %v", reached)
	}
}

// configuration.md §2: a form that newly requires a value the unattended doors do not send - a
// rule's action, an import's row, a mail - is said at the plan, per door and field, before those
// doors start refusing every creation of the type.
func TestAPlanNamesTheDoorsANewlyRequiredFieldCloses(t *testing.T) {
	f := newFixture()
	f.world.fields = map[string]map[string]any{"severity": {
		"id": "0190a8f0-0000-7000-8000-0000000000c1", "key": "severity", "kind": "TEXT", "options": []any{},
		"applies_to": []any{"TASK"},
	}}
	f.world.types["incident"] = map[string]any{"key": "incident", "level": "TASK", "name": "Incident",
		"form": map[string]any{"sections": []any{}, "fields": []any{}, "validations": []any{}, "channels": []any{}}}
	form := map[string]any{"sections": []any{}, "validations": []any{},
		"fields":   []any{map[string]any{"key": "custom_fields.severity", "required": true}},
		"channels": []any{map[string]any{"channel": "MAIL", "closed": true, "hidden": []any{}, "presets": map[string]any{}}}}
	document := map[string]any{"types": map[string]any{"incident": map[string]any{"form": form}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "types.incident" {
		t.Fatalf("reach %v, refusals %v", reached, plan["refusals"])
	}
	intake, _ := reached[0]["intake"].([]usecase.Output)
	if len(intake) != 2 || intake[0]["channel"] != "RULE" || intake[1]["channel"] != "IMPORT" {
		t.Fatalf("intake %v, want the rule and the import door, mail being closed", intake)
	}
	if fields := intake[0]["fields"].([]string); len(fields) != 1 || fields[0] != "custom_fields.severity" {
		t.Errorf("fields %v", fields)
	}
	if plan["plan_hash"] == nil {
		t.Errorf("a door's gap refused the plan: %v", plan["refusals"])
	}
}

// configuration.md §2: each door a form change leaves refusing says how many standing doors of
// its channel create the type now - the switched-on rules naming it, the document's as the plan
// sets them and every other scope's as they are, and for the task level's own type the
// switched-on mailboxes; mail creates no other type, and an import has no standing door.
func TestAPlanCountsTheDoorsANewlyRequiredFieldCloses(t *testing.T) {
	f := newFixture()
	f.world.fields = map[string]map[string]any{"severity": {
		"id": "0190a8f0-0000-7000-8000-0000000000c1", "key": "severity", "kind": "TEXT", "options": []any{},
		"applies_to": []any{"TASK"},
	}}
	empty := map[string]any{"sections": []any{}, "fields": []any{}, "validations": []any{}, "channels": []any{}}
	f.world.types["task"]["form"] = empty
	f.world.types["incident"] = map[string]any{"key": "incident", "level": "TASK", "name": "Incident", "form": empty}
	creates := func(id, key string, scope map[string]any, enabled bool, params map[string]any) map[string]any {
		return map[string]any{"id": id, "key": key, "scope": scope, "name": key, "enabled": enabled,
			"run_as": "0190a8f0-0000-7000-8000-00000000cccc", "trigger": map[string]any{"kind": "MANUAL"},
			"conditions": []any{}, "throttle": map[string]any{}, "on_error": "STOP",
			"actions": []any{map[string]any{"kind": "CREATE_WORK_ITEM", "params": params}}}
	}
	f.world.rules = map[string]map[string]any{
		"own":      creates("0190a8f0-0000-7000-8000-0000000000f1", "own", map[string]any{"type": "TENANT"}, true, map[string]any{"type": "TASK"}),
		"hubs":     creates("0190a8f0-0000-7000-8000-0000000000f2", "hubs", map[string]any{"type": "HUB", "id": deskHub}, true, map[string]any{"type": "TASK"}),
		"off":      creates("0190a8f0-0000-7000-8000-0000000000f3", "off", map[string]any{"type": "TENANT"}, false, map[string]any{"type": "TASK"}),
		"incident": creates("0190a8f0-0000-7000-8000-0000000000f4", "incident", map[string]any{"type": "HUB", "id": deskHub}, true, map[string]any{"type": "TASK", "entry_type": "incident"}),
	}
	f.world.mailboxes = []any{map[string]any{"enabled": true}, map[string]any{"enabled": true}, map[string]any{"enabled": false}}
	required := map[string]any{"sections": []any{}, "validations": []any{}, "channels": []any{},
		"fields": []any{map[string]any{"key": "custom_fields.severity", "required": true}}}
	doorsOf := func(plan usecase.Output, path string) map[string]any {
		t.Helper()
		out := map[string]any{}
		for _, row := range plan["reach"].([]usecase.Output) {
			if row["path"] != path {
				continue
			}
			for _, door := range row["intake"].([]usecase.Output) {
				out[door["channel"].(string)] = door["doors"]
			}
		}
		return out
	}

	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"types": map[string]any{"task": map[string]any{"form": required}},
	}})
	if got := doorsOf(plan, "types.task"); got["RULE"] != 2 || got["MAIL"] != 2 || got["IMPORT"] != nil {
		t.Fatalf("doors %v, want 2 rules, 2 mailboxes and no count for the import (refusals %v)", got, plan["refusals"])
	}

	// The plan switches its own rule off: it no longer counts, the hub's still does.
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"types": map[string]any{"task": map[string]any{"form": required}},
		"rules": map[string]any{"own": map[string]any{"enabled": false}},
	}})
	if got := doorsOf(plan, "types.task"); got["RULE"] != 1 {
		t.Errorf("doors %v, want the hub's rule alone", got)
	}

	// Another type: the rule naming it; mail never creates it, and the mailboxes are not read.
	f.world.inputs["ListMailboxes"] = nil
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"types": map[string]any{"incident": map[string]any{"form": required}},
	}})
	if got := doorsOf(plan, "types.incident"); got["RULE"] != 1 || got["MAIL"] != 0 {
		t.Errorf("doors %v, want the one rule and no mailbox", got)
	}
	if asked := f.world.inputs["ListMailboxes"]; len(asked) != 0 {
		t.Errorf("the mailboxes were read for a type mail does not create")
	}

	// A reader who may not list the rules or the mailboxes is told no count.
	f.world.rules, f.world.mailboxes = nil, nil
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"types": map[string]any{"task": map[string]any{"form": required}},
	}})
	if got := doorsOf(plan, "types.task"); got["RULE"] != nil || got["MAIL"] != nil {
		t.Errorf("doors %v counted for a reader who may not list them", got)
	}
}

// A template a workspace cannot take because a module is off says which module, so the person
// is told the switch to turn rather than that a section is withheld (UC-CFG-01).
func TestAPlanNamesTheModuleThatIsOff(t *testing.T) {
	f := newFixture()
	f.world.workflows = nil
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": serviceShape("todo", "review")}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	refusals := plan["refusals"].([]usecase.Output)
	if len(refusals) != 1 {
		t.Fatalf("refusals %v", refusals)
	}
	params, _ := refusals[0]["params"].(map[string]string)
	if refusals[0]["code"] != "module.off" || params["module"] != "WORKFLOWS" || refusals[0]["path"] != "/workflows" {
		t.Errorf("the refusal does not name the module: %v", refusals[0])
	}
}
