// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// BoardTypesOf answers the entry types whose entries stand on a board (domain.BoardTypes), from
// the registry where one is wired.
func BoardTypesOf(ctx context.Context, types repository.EntryTypes, schemes []domain.WorkflowScheme) ([]string, error) {
	var registry domain.EntryTypes
	if types != nil {
		rows, err := types.List(ctx)
		if err != nil {
			return nil, err
		}
		registry = domain.NewEntryTypes(rows)
	}
	return domain.BoardTypes(registry, schemes), nil
}

// OnBoard answers what a collection's board shows: the workflows in force for every entry type on
// it as one shape (domain.Workflows.OnBoard); nil where none runs. Types is the registry, nil for
// the scheme's types alone.
func (g WorkflowGate) OnBoard(ctx context.Context, collection domain.Container, types repository.EntryTypes) (*domain.WorkflowShape, error) {
	if !g.Wired() {
		return nil, nil
	}
	schemes, err := g.Workflows.Schemes(ctx)
	if err != nil || len(schemes) == 0 {
		return nil, err
	}
	onBoard, err := BoardTypesOf(ctx, types, schemes)
	if err != nil {
		return nil, err
	}
	rows, err := g.Workflows.List(ctx)
	if err != nil {
		return nil, err
	}
	shape, _ := domain.NewWorkflows(rows).OnBoard(schemes, collection.ID, collection.ParentID, onBoard)
	return shape, nil
}
