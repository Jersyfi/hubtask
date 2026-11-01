// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

type hubWorkflows struct {
	repository.Workflows
	rows    []domain.Workflow
	schemes []domain.WorkflowScheme
}

func (h hubWorkflows) List(context.Context) ([]domain.Workflow, error) { return h.rows, nil }
func (h hubWorkflows) Schemes(context.Context) ([]domain.WorkflowScheme, error) {
	return h.schemes, nil
}

// UC-FLW-02 check 9: a collection whose own row runs its hub's workflow does not leave the hub -
// nothing outside may run it; one running a workspace's workflow, or none of its own, may.
func TestACollectionRunningItsHubsWorkflowStaysInTheHub(t *testing.T) {
	ctx := context.Background()
	list := shared.MustParseID("0192f000-0000-7000-8000-0000000000aa")
	workflows := hubWorkflows{
		rows: []domain.Workflow{{Key: "service"}, {Key: "support", HubID: homeHub}},
		schemes: []domain.WorkflowScheme{
			{ContainerID: homeHub, WorkflowKey: "support"},
			{ContainerID: list, EntryType: "incident", WorkflowKey: "support"},
		},
	}
	if err := keepsHubWorkflows(ctx, workflows, list, homeHub, otherHub); detail(err) != "workflows.hub_bound" {
		t.Errorf("leaving the hub with its workflow: %v, want workflows.hub_bound", err)
	}
	if err := keepsHubWorkflows(ctx, workflows, list, homeHub, homeHub); err != nil {
		t.Errorf("staying in the hub: %v", err)
	}
	workflows.schemes[1].WorkflowKey = "service"
	if err := keepsHubWorkflows(ctx, workflows, list, homeHub, otherHub); err != nil {
		t.Errorf("leaving with a workspace's workflow: %v", err)
	}
	if err := keepsHubWorkflows(ctx, nil, list, homeHub, otherHub); err != nil {
		t.Errorf("without a repository: %v", err)
	}
}
