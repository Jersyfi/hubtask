// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// linkedWorkflows runs one workflow in listA, whose "doing" status is named.
type linkedWorkflows struct{ workrepo.Workflows }

func (linkedWorkflows) Schemes(context.Context) ([]work.WorkflowScheme, error) {
	return []work.WorkflowScheme{{ContainerID: listA, WorkflowKey: "story"}}, nil
}

func (linkedWorkflows) List(context.Context) ([]work.Workflow, error) {
	return []work.Workflow{{Key: "story", Published: work.WorkflowShape{Statuses: []work.WorkflowStatus{
		{Key: "doing", Name: "In progress", Category: work.CategoryInProgress},
	}}}}, nil
}

// The work-surface concept (goals): an entry linked to a goal reads its status as the workflow in
// force names it - *In Arbeit*, not *Offen* - and neither where no workflow runs.
func TestALinkedEntryCarriesItsNamedStatus(t *testing.T) {
	s, _, _, _ := newSteward(t)
	refs := s.References.(refs)
	doing := refs.entries[entry2]
	doing.Status = "doing"
	refs.entries[entry2] = doing
	s.Workflows = linkedWorkflows{}
	p := period(t, s)
	goal := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "hub_id": hubA.String(), "title": "Ship"})
	ok(t, LinkGoalEntries{s}.Descriptor(), ana, usecase.Input{"goal_id": goal.String("id"), "entry_ids": []any{entry1.String(), entry2.String()}})
	read := ok(t, ReadGoal{s}.Descriptor(), ana, usecase.Input{"goal_id": goal.String("id")})
	byID := map[string]usecase.Output{}
	for _, e := range read["linked_entries"].([]usecase.Output) {
		byID[e.String("id")] = e
	}
	if got := byID[entry2.String()]; got["status_name"] != "In progress" || got["status_category"] != "IN_PROGRESS" {
		t.Errorf("the entry in progress reads %+v", got)
	}
	if got := byID[entry1.String()]; got["status_name"] != nil || got["status_category"] != nil {
		t.Errorf("the entry without a status reads %+v", got)
	}
}
