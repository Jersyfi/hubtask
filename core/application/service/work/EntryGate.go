// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// EntryGate answers what an entry's collection offers it: the gate every item writer passes, and
// the mark every item read carries (domain-model.md §2, "Narrowing hides, it never deletes"). One
// answer for both, so that what a read says is hidden is what a write is refused for (P-05).
//
// A build without it - a test of something else - gates by the installation's profile alone, which
// is the behaviour from before collections had a structure (CompletionWriter's precedent). Wired,
// it never falls back: a structure that cannot be read refuses the write.
type EntryGate struct {
	Structure StructureResolver
	Types     repository.ItemTypes
	// EntryTypes narrows the profile by the entry's type: a record has no completion, a type that
	// is not schedulable no due date, reminder or recurrence (UC-FLW-01). Optional, and apart from
	// Wired, because a workspace without a type of its own narrows nothing.
	EntryTypes repository.EntryTypes
	// Forms holds every write to the entry's type's form (FormGate). Optional, for the reason
	// EntryTypes is.
	Forms FormGate
	// Sight takes the fields hidden from a reader out of every read and every answer (UC-ID-27
	// check 3). Optional, for the reason EntryTypes is.
	Sight FieldSight
}

// narrow takes from entries the fields hidden from the reader.
func (g EntryGate) narrow(
	ctx context.Context, uow persistence.UnitOfWork, actor appshared.ActorContext, items []domain.WorkItem,
) ([]domain.WorkItem, error) {
	if !g.Sight.Wired() || len(items) == 0 {
		return items, nil
	}
	lens, err := g.Sight.Lens(ctx, uow, actor)
	if err != nil {
		return nil, err
	}
	return lens.Items(ctx, uow, items)
}

// Wired reports whether the gate has what it needs.
func (g EntryGate) Wired() bool { return g.Structure.Wired() && g.Types != nil }

func (g EntryGate) chainsOf(ctx context.Context, items []domain.WorkItem) (map[shared.ID][]domain.ItemType, error) {
	chains := make(map[shared.ID][]domain.ItemType, len(items))
	above := make(map[shared.ID][]shared.ID, len(items))
	wanted := []shared.ID{}
	for _, item := range items {
		ids := domain.PathIDs(item.Path)
		if len(ids) > 0 {
			ids = ids[:len(ids)-1]
		}
		slices.Reverse(ids)
		above[item.ID] = ids
		for _, id := range ids {
			if !slices.Contains(wanted, id) {
				wanted = append(wanted, id)
			}
		}
	}
	if len(wanted) == 0 {
		return chains, nil
	}
	types, err := g.Types.TypesOf(ctx, wanted)
	if err != nil {
		return nil, err
	}
	for id, ids := range above {
		chain := make([]domain.ItemType, 0, len(ids))
		for _, ancestor := range ids {
			if itemType, held := types[ancestor]; held {
				chain = append(chain, itemType)
			}
		}
		chains[id] = chain
	}
	return chains, nil
}

// marksIn answers the mark of every entry inside the caller's transaction: one resolution per
// distinct collection, one statement for every level above every entry.
func (g EntryGate) marksIn(ctx context.Context, items []domain.WorkItem) (map[shared.ID]domain.EntryHidden, error) {
	marks := make(map[shared.ID]domain.EntryHidden, len(items))
	if !g.Wired() || len(items) == 0 {
		return marks, nil
	}
	reading := g.Structure.within(repository.LayerRead)
	bound, err := reading.Bound(ctx)
	if err != nil {
		return nil, err
	}
	offers := map[shared.ID]domain.Offered{}
	needChains := false
	for _, item := range items {
		if _, resolved := offers[item.CollectionID]; resolved {
			continue
		}
		collection, err := g.Structure.Containers.Find(ctx, item.CollectionID)
		if err != nil {
			return nil, err
		}
		effective, err := reading.Of(ctx, collection)
		if err != nil {
			return nil, err
		}
		offered := domain.NewOffered(bound, effective)
		offers[item.CollectionID] = offered
		needChains = needChains || offered.HidesALevel()
	}
	chains := map[shared.ID][]domain.ItemType{}
	if needChains {
		chains, err = g.chainsOf(ctx, items)
		if err != nil {
			return nil, err
		}
	}
	shown := []shared.ID{}
	for _, item := range items {
		offered := offers[item.CollectionID]
		marks[item.ID] = offered.Hidden(item.Type, chains[item.ID])
		if offered.HidesALevel() && !marks[item.ID].Level {
			shown = append(shown, item.ID)
		}
	}
	if err := g.countBelow(ctx, items, offers, shown, marks); err != nil {
		return nil, err
	}
	return marks, nil
}

// countBelow adds to each shown entry's mark how many entries under it its collection hides: one
// statement for every shown entry, asked only where a collection hides a level at all.
func (g EntryGate) countBelow(
	ctx context.Context, items []domain.WorkItem, offers map[shared.ID]domain.Offered, shown []shared.ID,
	marks map[shared.ID]domain.EntryHidden,
) error {
	if len(shown) == 0 {
		return nil
	}
	below, err := g.Types.EntriesBelow(ctx, shown)
	if err != nil {
		return err
	}
	collections := make(map[shared.ID]shared.ID, len(items))
	for _, item := range items {
		collections[item.ID] = item.CollectionID
	}
	for _, group := range below {
		mark, held := marks[group.Under]
		if !held || !offers[collections[group.Under]].HidesAnyOf(group.Seen) {
			continue
		}
		mark.Below += group.Entries
		marks[group.Under] = mark
	}
	return nil
}

// marksOf answers the marks in a read-only transaction of its own: after the permission question,
// so a refused caller pays for nothing.
func (g EntryGate) marksOf(
	ctx context.Context, uow persistence.UnitOfWork, actor appshared.ActorContext, items []domain.WorkItem,
) (map[shared.ID]domain.EntryHidden, error) {
	if !g.Wired() || len(items) == 0 || uow == nil {
		return map[shared.ID]domain.EntryHidden{}, nil
	}
	var marks map[shared.ID]domain.EntryHidden
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		marks, err = g.marksIn(ctx, items)
		return err
	})
	return marks, err
}

// Answer is an entry's projection with its mark, as every write to an entry answers.
func (g EntryGate) Answer(
	ctx context.Context, uow persistence.UnitOfWork, actor appshared.ActorContext, item domain.WorkItem,
) (usecase.Output, error) {
	marks, err := g.marksOf(ctx, uow, actor, []domain.WorkItem{item})
	if err != nil {
		return nil, err
	}
	narrowed, err := g.narrow(ctx, uow, actor, []domain.WorkItem{item})
	if err != nil {
		return nil, err
	}
	return withHidden(ItemOutput(narrowed[0]), marks, item.ID), nil
}

// withHidden puts an entry's mark into its projection, where one was answered.
func withHidden(out usecase.Output, marks map[shared.ID]domain.EntryHidden, id shared.ID) usecase.Output {
	mark, marked := marks[id]
	if !marked {
		return out
	}
	capabilities := make([]string, 0, len(mark.Capabilities))
	for _, capability := range mark.Capabilities {
		capabilities = append(capabilities, string(capability))
	}
	out["hidden"] = map[string]any{"level": mark.Level, "capabilities": capabilities, "below": mark.Below}
	return out
}
