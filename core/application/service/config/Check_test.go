// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// configuration.md §11, UC-CFG-01 check 10: the check reads the configuration in force, the
// workspace's and a hub's, and answers what people would meet - errors first, each with the block
// to change - before anybody meets it.
func TestTheConfigurationCheckFindsWhatPeopleWouldMeet(t *testing.T) {
	f := hubFixture()
	// The workflow has statuses but no step and no done status reached from where it starts.
	f.world.workflows = map[string]map[string]any{"service": {"key": "service", "name": "Service", "published": map[string]any{
		"initial":  "todo",
		"statuses": []any{map[string]any{"key": "todo", "category": "TODO"}, map[string]any{"key": "done", "category": "DONE"}},
		"steps":    []any{map[string]any{"key": "finish", "from": []any{"todo"}, "to": "done", "who": map[string]any{"roles": []any{"cab_member"}}}},
	}}}
	f.world.scheme = []any{map[string]any{"container_id": supportHub, "entry_type": "incident", "workflow": "gone"}}

	out := invoke(t, CheckConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	findings := out["findings"].([]usecase.Output)
	var codes []string
	for _, finding := range findings {
		codes = append(codes, finding["severity"].(string)+" "+finding["code"].(string)+" "+finding["path"].(string))
	}
	want := []string{
		"ERROR config.check.scheme_workflow_unknown scheme.incident",
		"ERROR config.check.step_nobody workflows.service",
		"ERROR config.check.done_unreachable workflows.service",
		"WARNING config.check.status_unreachable workflows.service",
	}
	if len(codes) != len(want) {
		t.Fatalf("findings %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("findings %v, want %v", codes, want)
		}
	}

	f.world.workflows["service"]["published"].(map[string]any)["steps"] = []any{map[string]any{"key": "finish", "from": []any{"todo"}, "to": "done"}}
	f.world.scheme = []any{map[string]any{"container_id": supportHub, "entry_type": "incident", "workflow": "service"}}
	if out := invoke(t, CheckConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub}); len(out["findings"].([]usecase.Output)) != 0 {
		t.Errorf("a sound configuration found %v", out["findings"])
	}
}

// UC-CFG-01 check 10: the check knows every door an entry arrives through, so a required field
// each of them hides is found before an entry of the type stalls in its first column.
func TestTheConfigurationCheckKnowsEveryChannel(t *testing.T) {
	f := newFixture()
	f.world.fields = map[string]map[string]any{"severity": {
		"id": "0190a8f0-0000-7000-8000-0000000000c1", "key": "severity", "kind": "TEXT", "options": []any{},
		"is_required": true, "applies_to": []any{"TASK"},
	}}
	var rules []any
	for _, channel := range workmodel.FormChannels() {
		rules = append(rules, map[string]any{"channel": string(channel), "closed": false,
			"hidden": []any{"custom_fields.severity"}, "presets": map[string]any{}})
	}
	f.world.types["incident"] = map[string]any{"key": "incident", "level": "TASK", "name": "Incident",
		"form": map[string]any{"sections": []any{}, "fields": []any{}, "validations": []any{}, "channels": rules}}

	out := invoke(t, CheckConfig{f.desk}.Descriptor(), person, usecase.Input{})
	findings := out["findings"].([]usecase.Output)
	if len(findings) != 1 || findings[0]["code"] != "config.check.required_unfillable" || findings[0]["path"] != "types.incident" {
		t.Fatalf("findings %v, want the severity field nobody can fill", findings)
	}

	rules[0].(map[string]any)["hidden"] = []any{}
	if out := invoke(t, CheckConfig{f.desk}.Descriptor(), person, usecase.Input{}); len(out["findings"].([]usecase.Output)) != 0 {
		t.Errorf("a field one channel shows found %v", out["findings"])
	}
}

// configuration.md §11, UC-CFG-01 check 10: a workspace template whose blocks the check finds an
// error in is refused when it is published, each finding naming its block, so a broken template is
// never offered; a warning alone publishes.
func TestATemplateTheCheckFindsBrokenIsNotPublished(t *testing.T) {
	f := newFixture()
	stranded := map[string]any{"workflows": map[string]any{"service": map[string]any{
		"name": "Service", "shape": map[string]any{
			"initial":  "todo",
			"statuses": []any{map[string]any{"key": "todo", "category": "TODO"}, map[string]any{"key": "done", "category": "DONE"}},
			"steps":    []any{map[string]any{"key": "finish", "from": []any{"todo"}, "to": "done", "who": map[string]any{"roles": []any{"cab_member"}}}},
		},
	}}}
	_, err := PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "desk", "name": "Desk", "document": stranded,
	})
	refusals := refusalsIn(err)
	if len(refusals) == 0 {
		t.Fatalf("a template whose workflow strands every entry was published: %v", err)
	}
	for _, refusal := range refusals {
		if refusal.Code != "config.template_check_failed" || refusal.Path != "/document/workflows/service" || refusal.Params["finding"] == "" {
			t.Errorf("refusal %+v, want config.template_check_failed at the workflow with its finding", refusal)
		}
	}
}
