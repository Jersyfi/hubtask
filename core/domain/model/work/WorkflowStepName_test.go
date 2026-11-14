// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "testing"

// A step is named as the team named it: its own name, else the name of the status it leads into;
// a key is never a name, so a step named neither way answers none.
func TestAStepIsNamedByItsNameElseItsTargetsName(t *testing.T) {
	requests := Workflow{Key: "requests", Published: WorkflowShape{
		Statuses: []WorkflowStatus{{Key: "new"}, {Key: "approved", Name: "Freigegeben"}, {Key: "done"}},
		Steps: []WorkflowStep{
			{Key: "approve", Name: "Freigabe einholen", To: "approved"},
			{Key: "grant", To: "approved"},
			{Key: "finish", To: "done"},
		},
	}}
	set := NewWorkflows([]Workflow{requests})
	for _, tc := range []struct{ workflow, step, want string }{
		{"requests", "approve", "Freigabe einholen"},
		{"requests", "grant", "Freigegeben"},
		{"requests", "finish", ""},
		{"requests", "gone", ""},
		{"elsewhere", "approve", ""},
	} {
		if got := set.StepName(tc.workflow, tc.step); got != tc.want {
			t.Errorf("StepName(%q, %q) = %q, want %q", tc.workflow, tc.step, got, tc.want)
		}
	}
}
