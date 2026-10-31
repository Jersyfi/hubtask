// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"reflect"
	"testing"
)

func status(key, category string) any { return map[string]any{"key": key, "category": category} }

func step(key string, from []any, to string, who map[string]any) any {
	out := map[string]any{"key": key, "from": from, "to": to}
	if who != nil {
		out["who"] = who
	}
	return out
}

func formOf(fields []any, channels ...any) map[string]any {
	return map[string]any{"fields": fields, "channels": channels}
}

func channelRule(channel string, hidden []any, presets map[string]any) any {
	return map[string]any{"channel": channel, "hidden": hidden, "presets": presets}
}

func workflowOf(steps ...any) Block {
	return Block{"shape": map[string]any{
		"initial":  "new",
		"statuses": []any{status("new", "TODO"), status("doing", "IN_PROGRESS"), status("done", "DONE")},
		"steps":    steps,
	}}
}

func TestTheConfigurationCheck(t *testing.T) {
	builtIn := []string{"OWNER", "ADMIN", "MEMBER"}
	cases := []struct {
		name string
		in   Checked
		want []Finding
	}{
		{
			name: "a workflow every status of which is reached finds nothing",
			in: Checked{Workspace: Document{"workflows": {"flow": workflowOf(
				step("start", []any{"new"}, "doing", nil), step("finish", []any{"doing"}, "done", nil))}}},
		},
		{
			name: "a status no step leads to, and no way to done",
			in: Checked{Workspace: Document{"workflows": {"flow": workflowOf(
				step("start", []any{"new"}, "doing", nil))}}},
			want: []Finding{
				{Severity: SeverityError, Code: "config.check.done_unreachable", Path: "workflows.flow"},
				{Severity: SeverityWarning, Code: "config.check.status_unreachable", Path: "workflows.flow", Params: map[string]string{"status": "done"}},
			},
		},
		{
			name: "a step only a role nobody holds may take strands the statuses behind it",
			in: Checked{BuiltInRoles: builtIn, Workspace: Document{"workflows": {"flow": workflowOf(
				step("start", []any{"new"}, "doing", nil),
				step("finish", []any{"doing"}, "done", map[string]any{"roles": []any{"cab_member"}, "groups": []any{}}))}}},
			want: []Finding{
				{Severity: SeverityError, Code: "config.check.step_nobody", Path: "workflows.flow", Params: map[string]string{"step": "finish"}},
				{Severity: SeverityError, Code: "config.check.done_unreachable", Path: "workflows.flow"},
				{Severity: SeverityWarning, Code: "config.check.status_unreachable", Path: "workflows.flow", Params: map[string]string{"status": "done"}},
			},
		},
		{
			name: "a custom role of the workspace and a built-in role may take a step",
			in: Checked{BuiltInRoles: builtIn, Workspace: Document{
				"roles": {"cab_member": Block{"name": "CAB"}},
				"workflows": {"flow": workflowOf(
					step("start", []any{"new"}, "doing", map[string]any{"roles": []any{"MEMBER"}}),
					step("finish", nil, "done", map[string]any{"roles": []any{"cab_member"}}))},
			}},
		},
		{
			name: "a rule changing the entry on every change of it may loop",
			in: Checked{Workspace: Document{"rules": {
				"loops": Block{"trigger": map[string]any{"kind": "EVENT", "event_type": "item.updated"},
					"actions": []any{map[string]any{"kind": "UPDATE_WORK_ITEM"}}},
				"narrowed": Block{"trigger": map[string]any{"kind": "EVENT", "event_type": "item.updated", "changed_fields": []any{"due_date"}},
					"actions": []any{map[string]any{"kind": "UPDATE_WORK_ITEM"}}},
				"off": Block{"enabled": false, "trigger": map[string]any{"kind": "EVENT", "event_type": "item.updated"},
					"actions": []any{map[string]any{"kind": "TAKE_STEP"}}},
			}}},
			want: []Finding{{Severity: SeverityWarning, Code: "config.check.rule_may_loop", Path: "rules.loops", Params: map[string]string{"action": "UPDATE_WORK_ITEM"}}},
		},
		{
			name: "a hub's scheme naming a workflow the workspace does not hold",
			in: Checked{
				Workspace: Document{"workflows": {"flow": workflowOf(step("finish", nil, "done", nil), step("start", nil, "doing", nil))}},
				Hub:       Document{"scheme": {"incident": Block{"workflow": "gone"}, "task": Block{"workflow": "flow"}}},
			},
			want: []Finding{{Severity: SeverityError, Code: "config.check.scheme_workflow_unknown", Path: "scheme.incident", Params: map[string]string{"workflow": "gone"}}},
		},
		{
			name: "a required field every channel hides with nothing to start it from",
			in: Checked{Channels: []string{"APP", "MCP"}, Workspace: Document{
				"fields": {
					"severity": Block{"kind": "SELECT", "is_required": true, "applies_to": []any{"TASK"}},
					"impact":   Block{"kind": "TEXT", "is_required": true, "applies_to": []any{"TASK"}},
					"cause":    Block{"kind": "TEXT", "is_required": true, "applies_to": []any{"TASK"}},
				},
				"types": {
					"incident": Block{"level": "TASK", "form": formOf(
						[]any{map[string]any{"key": "custom_fields.cause", "default": "unknown"}},
						channelRule("APP", []any{"custom_fields.severity", "custom_fields.impact", "custom_fields.cause"}, nil),
						channelRule("MCP", []any{"custom_fields.severity", "custom_fields.impact", "custom_fields.cause"}, map[string]any{"custom_fields.impact": "low"}))},
					"change": Block{"level": "TASK", "form": formOf(nil, channelRule("APP", []any{"custom_fields.severity"}, nil))},
					"story": Block{"level": "WORK_PACKAGE", "form": formOf(nil,
						channelRule("APP", []any{"custom_fields.severity"}, nil), channelRule("MCP", []any{"custom_fields.severity"}, nil))},
				},
			}},
			want: []Finding{{Severity: SeverityError, Code: "config.check.required_unfillable", Path: "types.incident",
				Params: map[string]string{"field": "severity"}}},
		},
		{
			name: "two fields of one kind offering the same choices",
			in: Checked{Workspace: Document{"fields": {
				"prio":     Block{"kind": "SELECT", "options": []any{"High", "Low"}},
				"priority": Block{"kind": "SELECT", "options": []any{"low", "high "}},
				"urgency":  Block{"kind": "MULTI_SELECT", "options": []any{"high", "low"}},
				"notes":    Block{"kind": "TEXT", "options": []any{}},
				"remarks":  Block{"kind": "TEXT", "options": []any{}},
			}}},
			want: []Finding{{Severity: SeverityWarning, Code: "config.check.fields_same_meaning", Path: "fields.prio",
				Params: map[string]string{"field": "prio", "other": "priority"}}},
		},
		{
			name: "collections each locking the same tolerance the hub leaves open",
			in: Checked{Workspace: Document{}, Hub: Document{"collection_tolerance": {
				"network.time_tolerance_days": Block{"value": float64(3), "lock": true},
				"servers.time_tolerance_days": Block{"value": float64(3), "lock": true},
				"desk.time_tolerance_days":    Block{"value": float64(5), "lock": true},
				"office.time_tolerance_days":  Block{"value": float64(3)},
			}}},
			want: []Finding{{Severity: SeverityWarning, Code: "config.check.lock_could_move_up", Path: "tolerance.time_tolerance_days",
				Params: map[string]string{"value": "3", "collections": "2"}}},
		},
		{
			name: "no lock moves up past the hub's own",
			in: Checked{Workspace: Document{}, Hub: Document{
				"tolerance": {"time_tolerance_days": Block{"value": float64(2), "lock": true}},
				"collection_tolerance": {
					"network.time_tolerance_days": Block{"value": float64(3), "lock": true},
					"servers.time_tolerance_days": Block{"value": float64(3), "lock": true},
				}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckDocument(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}
