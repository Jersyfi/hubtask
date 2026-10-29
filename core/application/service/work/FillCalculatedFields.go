// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// DefaultFillBatch is how many entries one pass of the fill recalculates, each in its own
// transaction, so one pass stays short and a busy entry costs nobody else a retry.
const DefaultFillBatch = 100

// FillCalculatedFields writes a new CALCULATED field on the entries defined before it
// (domain-model.md §3.5): a page of the scope's entries after a cursor, each recalculated as a
// write of one of its inputs would. The system's own act, seeded by the definition; each pass
// enqueues the next page's job with its cursor, since a repeated job keeps its payload.
type FillCalculatedFields struct {
	Walk   repository.EntryWalk
	Setter SetCustomField
	Jobs   queue.Queue
	// BatchSize is how many entries a pass walks; zero is DefaultFillBatch.
	BatchSize int
}

// FillCalculatedFieldsCommand names the scope - a collection, or the workspace for zero - and the
// cursor to continue after.
type FillCalculatedFieldsCommand struct {
	DefinitionID shared.ID
	CollectionID shared.ID
	After        shared.ID
}

// FillOutcome is one pass: the cursor to continue after, how many entries moved, and whether the
// walk reached the end.
type FillOutcome struct {
	Next   shared.ID
	Filled int
	Done   bool
}

// Execute walks one page.
func (f FillCalculatedFields) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd FillCalculatedFieldsCommand,
) (FillOutcome, error) {
	size := f.BatchSize
	if size <= 0 {
		size = DefaultFillBatch
	}
	var ids []shared.ID
	err := f.Setter.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		ids, err = f.Walk.IDsAfter(ctx, cmd.CollectionID, cmd.After, size)
		return err
	})
	if err != nil {
		return FillOutcome{}, err
	}
	outcome := FillOutcome{Next: cmd.After, Done: len(ids) < size}
	for _, id := range ids {
		moved, err := f.fill(ctx, actor, id)
		if err != nil {
			return FillOutcome{}, err
		}
		if moved {
			outcome.Filled++
		}
		outcome.Next = id
	}
	if outcome.Done || f.Jobs == nil {
		return outcome, nil
	}
	next := queue.FieldFill(actor.TenantID, cmd.DefinitionID, cmd.CollectionID, outcome.Next.String())
	err = f.Setter.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, err := f.Jobs.Enqueue(ctx, next)
		return err
	})
	return outcome, err
}

// fill recalculates one entry inside its own transaction. An entry that may not change now - its
// collection archived, its type carrying no custom fields, a write in between - is passed over:
// its next write of an input recalculates it.
func (f FillCalculatedFields) fill(ctx context.Context, actor appshared.ActorContext, id shared.ID) (bool, error) {
	h := f.Setter
	moved := false
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, h.Items, id)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, h.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		profile, err := h.Gate.gatedProfile(ctx, h.Profiles, collection, item)
		if err != nil {
			return err
		}
		if err := item.EnsureCustomisable(profile); err != nil {
			return err
		}
		written, err := h.recalculate(ctx, actor, item, profile, h.Clock.Now())
		moved = written.Version != item.Version
		return err
	})
	if err == nil {
		return moved, nil
	}
	if passedOver(err) {
		return false, nil
	}
	return false, err
}

// passedOver is a refusal the entry's own state gives, rather than a failure to retry.
func passedOver(err error) bool {
	var coded *shared.Error
	if !errors.As(err, &coded) {
		return false
	}
	switch coded.Category {
	case shared.CategoryValidation, shared.CategoryConflict, shared.CategoryNotFound, shared.CategoryGone:
		return true
	}
	return false
}
