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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckDocument(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}
