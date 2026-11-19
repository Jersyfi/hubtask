// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// StateModes answers a collection's level per entry type it offers - a list, columns or a
// workflow (domain-model.md §3.5a) - derived from its board and the schemes, never stored.
type StateModes struct {
	Boards   Boards
	Workflow WorkflowGate
	// EntryTypes adds the workspace's own types to the levels' system types. Optional.
	EntryTypes repository.EntryTypes
	// Entries narrows the types to those the collection's structure offers. Optional.
	Entries EntryGate
}

// Wired reports whether the build reads boards; one without answers no level.
func (s StateModes) Wired() bool { return s.Boards.Wired() }

// Of answers the tasks' level and the level of every type the collection offers, inside the
// caller's transaction. The tasks' level is the system task type's, or - where the collection
// does not offer it - its first offered task type's.
func (s StateModes) Of(ctx context.Context, collection domain.Container) (domain.StateMode, map[string]domain.StateMode, error) {
	board, err := s.Boards.Of(ctx, collection.ID)
	if err != nil {
		return "", nil, err
	}
	types, err := s.offered(ctx, collection)
	if err != nil {
		return "", nil, err
	}
	var schemes []domain.WorkflowScheme
	var workflows domain.Workflows
	if s.Workflow.Wired() {
		if schemes, err = s.Workflow.Workflows.Schemes(ctx); err != nil {
			return "", nil, err
		}
		rows, err := s.Workflow.Workflows.List(ctx)
		if err != nil {
			return "", nil, err
		}
		workflows = domain.NewWorkflows(rows)
	}
	modes := make(map[string]domain.StateMode, len(types))
	var task domain.StateMode
	for _, entryType := range types {
		key, runs := domain.ResolveWorkflow(schemes, collection.ID, collection.ParentID, entryType.Key)
		if runs {
			_, runs = workflows.Find(key)
		}
		mode := domain.StateModeOf(entryType.Level, runs, len(board.Columns))
		modes[entryType.Key] = mode
		if entryType.Level == domain.ItemTask && (task == "" || entryType.Key == domain.SystemEntryTypeKey(domain.ItemTask)) {
			task = mode
		}
	}
	if task == "" {
		task = domain.StateModeOf(domain.ItemTask, false, len(board.Columns))
	}
	return task, modes, nil
}

// offered is the live types usable in the collection's hub that its structure offers on their
// level; a level the structure leaves open offers every one.
func (s StateModes) offered(ctx context.Context, collection domain.Container) ([]domain.EntryType, error) {
	var rows []domain.EntryType
	if s.EntryTypes != nil {
		var err error
		if rows, err = s.EntryTypes.List(ctx); err != nil {
			return nil, err
		}
	}
	all := domain.NewEntryTypes(rows).Within(collection.ParentID)
	offer := entryOffer{}
	if s.Entries.Wired() {
		offered, err := s.Entries.offeredIn(ctx, collection)
		if err != nil {
			return nil, err
		}
		offer = entryOffer{offered: offered, wired: true}
	}
	var types []domain.EntryType
	for _, entryType := range all {
		if entryType.IsArchived() {
			continue
		}
		if keys := offer.entryTypesOn(entryType.Level); keys != nil && !slices.Contains(keys, entryType.Key) {
			continue
		}
		types = append(types, entryType)
	}
	return types, nil
}
