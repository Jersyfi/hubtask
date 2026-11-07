// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"
)

// A flow metric counts work as begun in every status of the IN_PROGRESS category of any workflow,
// once each.
func TestTheStartedStatusesAreEveryWorkflowsInProgress(t *testing.T) {
	support := Workflow{Key: "support", Published: WorkflowShape{Statuses: []WorkflowStatus{
		{Key: "new", Category: CategoryTodo},
		{Key: "triage", Category: CategoryInProgress},
		{Key: "in_progress", Category: CategoryInProgress},
		{Key: "closed", Category: CategoryDone},
	}}}
	got := NewWorkflows([]Workflow{support}).Started()
	if want := []string{"in_progress", "triage"}; !slices.Equal(got, want) {
		t.Fatalf("started = %v, want %v", got, want)
	}
}
