// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var copiedWorkflowID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e3")

func serviceWorkflow(t *testing.T, hub shared.ID) work.Workflow {
	t.Helper()
	w, err := work.NewWorkflow(work.NewWorkflowInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e4"), TenantID: tenant,
		Key: "service", Name: "Service", Shape: serviceShape(), HubID: hub, Now: workflowNow,
		Text: text.Composing{},
	})
	if err != nil {
		t.Fatalf("NewWorkflow: %v", err)
	}
	return w
}

// UC-FLW-02 check 9: a hub's own workflow runs only inside that hub; a workspace's everywhere.
func TestAHubsWorkflowRunsOnlyInsideTheHub(t *testing.T) {
	common := serviceWorkflow(t, "")
	own := serviceWorkflow(t, supportHub)
	own.Key, own.ID = "incidents", copiedWorkflowID
	set := work.NewWorkflows([]work.Workflow{common, own})

	for _, tc := range []struct {
		name string
		hub  shared.ID
		want []string
	}{
		{"inside the hub", supportHub, []string{"default", "service", "incidents"}},
		{"another hub", salesHub, []string{"default", "service"}},
		{"no hub", "", []string{"default", "service"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			for _, w := range set.Within(tc.hub) {
				keys = append(keys, w.Key)
			}
			if len(keys) != len(tc.want) {
				t.Fatalf("within %v, want %v", keys, tc.want)
			}
			for i := range keys {
				if keys[i] != tc.want[i] {
					t.Errorf("within %v, want %v", keys, tc.want)
				}
			}
			if own.UsableIn(tc.hub) != (tc.hub == supportHub) {
				t.Errorf("UsableIn(%q) = %v", tc.hub, own.UsableIn(tc.hub))
			}
		})
	}
}

// A copy takes the source's shape in force, never its draft, belongs to the hub, and names the
// key it came from; the source is untouched.
func TestCopyingAWorkflowToAHub(t *testing.T) {
	source := serviceWorkflow(t, "")
	draft := serviceShape()
	draft.Statuses = draft.Statuses[:4]
	draft.Steps = draft.Steps[:2]
	source, err := source.WithDraft(draft, workflowNow, text.Composing{})
	if err != nil {
		t.Fatalf("WithDraft: %v", err)
	}
	renamed := "Support service"
	copied, err := source.CopyAt(work.WorkflowCopy{
		ID: copiedWorkflowID, HubID: supportHub, Key: "support_service", Name: &renamed,
		By: tenant, Now: workflowNow, Text: text.Composing{},
	})
	if err != nil {
		t.Fatalf("CopyAt: %v", err)
	}
	if copied.HubID != supportHub || copied.CopiedFrom != "service" || copied.Key != "support_service" ||
		copied.Name != renamed || copied.ID != copiedWorkflowID || copied.TenantID != tenant {
		t.Errorf("copy = %+v", copied)
	}
	if copied.Draft != nil || len(copied.Published.Statuses) != len(source.Published.Statuses) {
		t.Errorf("the copy took the draft: %+v", copied)
	}
	if source.HubID != "" || source.CopiedFrom != "" || source.Draft == nil {
		t.Errorf("the source changed: %+v", source)
	}
	kept, err := source.CopyAt(work.WorkflowCopy{ID: copiedWorkflowID, HubID: supportHub, Key: "kept", Now: workflowNow, Text: text.Composing{}})
	if err != nil || kept.Name != source.Name {
		t.Errorf("a copy without a name keeps the source's: %q, %v", kept.Name, err)
	}
}

func TestCopyingAWorkflowRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source work.Workflow
		in     work.WorkflowCopy
		code   string
	}{
		{"no hub", serviceWorkflow(t, ""), work.WorkflowCopy{Key: "x"}, "workflows.hub_required"},
		{"the reserved key", serviceWorkflow(t, ""), work.WorkflowCopy{HubID: supportHub, Key: "default"}, "workflows.key_reserved"},
		{"the shipped one nobody named", work.SystemWorkflow(), work.WorkflowCopy{HubID: supportHub, Key: "mine"}, "workflows.name_empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.ID, tc.in.TenantID, tc.in.Now, tc.in.Text = copiedWorkflowID, tenant, workflowNow, text.Composing{}
			_, err := tc.source.CopyAt(tc.in)
			var coded *shared.Error
			if !errors.As(err, &coded) || coded.DetailCode != tc.code {
				t.Errorf("err = %v, want %s", err, tc.code)
			}
		})
	}
}
