// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A collection's level is derived per entry type, never a setting (§5): a list without columns,
// columns for tasks only, and a workflow wherever one runs for the type.
func TestACollectionsLevelIsDerivedPerEntryType(t *testing.T) {
	collection := domain.Container{ID: collectionID, ParentID: hubID, Type: domain.ContainerCollection}
	ticket := domain.EntryType{Key: "ticket", Level: domain.ItemTask}
	elsewhere := domain.EntryType{Key: "story", Level: domain.ItemTask, HubID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")}
	cases := []struct {
		name    string
		columns bool
		schemes []domain.WorkflowScheme
		task    domain.StateMode
		modes   map[string]domain.StateMode
	}{
		{"a list", false, nil, domain.StateModeNone, map[string]domain.StateMode{
			"task": domain.StateModeNone, "work_package": domain.StateModeNone,
			"activity": domain.StateModeNone, "ticket": domain.StateModeNone,
		}},
		{"columns hold tasks only", true, nil, domain.StateModeColumns, map[string]domain.StateMode{
			"task": domain.StateModeColumns, "work_package": domain.StateModeNone,
			"activity": domain.StateModeNone, "ticket": domain.StateModeColumns,
		}},
		{"a workflow for one type wins over the columns", true,
			[]domain.WorkflowScheme{{ContainerID: hubID, EntryType: "ticket", WorkflowKey: "review"}},
			domain.StateModeColumns, map[string]domain.StateMode{
				"task": domain.StateModeColumns, "work_package": domain.StateModeNone,
				"activity": domain.StateModeNone, "ticket": domain.StateModeWorkflow,
			}},
		{"a workflow for every type", false,
			[]domain.WorkflowScheme{{ContainerID: collectionID, WorkflowKey: "review"}},
			domain.StateModeWorkflow, map[string]domain.StateMode{
				"task": domain.StateModeWorkflow, "work_package": domain.StateModeWorkflow,
				"activity": domain.StateModeWorkflow, "ticket": domain.StateModeWorkflow,
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			board := &buckets{stored: map[shared.ID]domain.Bucket{}}
			if c.columns {
				threeColumnBoard(board)
			}
			modes := StateModes{
				Boards:     Boards{Buckets: board},
				Workflow:   WorkflowGate{Workflows: &workflowStore{rows: []domain.Workflow{reviewWorkflow()}, schemes: c.schemes}},
				EntryTypes: &entryTypes{rows: []domain.EntryType{ticket, elsewhere}},
			}
			task, all, err := modes.Of(t.Context(), collection)
			if err != nil {
				t.Fatalf("reading the levels: %v", err)
			}
			if task != c.task {
				t.Errorf("the tasks' level is %s, want %s", task, c.task)
			}
			if len(all) != len(c.modes) {
				t.Errorf("levels %v, want %v", all, c.modes)
			}
			for key, want := range c.modes {
				if all[key] != want {
					t.Errorf("%s is %s, want %s", key, all[key], want)
				}
			}
		})
	}
}

// Reading one collection answers its levels; a hub answers none.
func TestReadingACollectionAnswersItsLevels(t *testing.T) {
	store := &containers{stored: map[shared.ID]domain.Container{
		hubID: {ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Home", OrderKey: "a0", Version: 1},
		collectionID: {
			ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
			Name: "Shopping", OrderKey: "a0", Version: 1,
		},
	}}
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	get := GetContainer{
		Containers: store, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
		Levels: StateModes{Boards: Boards{Buckets: board}},
	}

	out, err := get.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"container_id": collectionID.String()})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	modes, _ := out["state_modes"].(map[string]any)
	if out["state_mode"] != "COLUMNS" || modes["task"] != "COLUMNS" || modes["activity"] != "NONE" {
		t.Errorf("levels %v / %v", out["state_mode"], modes)
	}
	out, err = get.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"container_id": hubID.String()})
	if err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	if _, held := out["state_mode"]; held {
		t.Errorf("a hub answered a level: %v", out["state_mode"])
	}
}
