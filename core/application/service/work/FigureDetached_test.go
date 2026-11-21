// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// callersTransaction marks a context that carries the caller's transaction.
type callersTransaction struct{}

// detachingUnitOfWork is the double that can hand out a context without the caller's transaction.
type detachingUnitOfWork struct{ unitOfWork }

func (*detachingUnitOfWork) Detach(ctx context.Context) context.Context {
	return context.WithValue(ctx, callersTransaction{}, nil)
}

// joinedStore records whether a count ran in the caller's transaction.
type joinedStore struct {
	figureStore
	joined bool
}

func (s *joinedStore) Count(
	ctx context.Context, requests []domain.FigureRequest, window domain.FigureWindow,
) (map[shared.ID]domain.FigureCounts, error) {
	if ctx.Value(callersTransaction{}) != nil {
		s.joined = true
	}
	return s.figureStore.Count(ctx, requests, window)
}

// A container read made inside another use case's transaction - the configuration reading a hub
// while it applies a template - counts its figures on a connection of its own: a count cut off by
// its deadline, or one finished after the read, never cancels or overlaps a statement of the
// caller's transaction ("conn busy").
func TestAFigureIsNeverCountedInTheCallersTransaction(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureOpen, domain.ItemTask)}
	store := &joinedStore{figureStore: figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Open: 2}}}}
	reader := FigureReader{Figures: store, Clock: clock.Fixed(now), Profiles: &profiles{rows: seededProfiles()}}

	ctx := context.WithValue(context.Background(), callersTransaction{}, true)
	figures := reader.Count(ctx, actor(), &detachingUnitOfWork{}, []domain.Container{collection}, structures)
	if store.joined {
		t.Fatal("the figure was counted in the caller's transaction")
	}
	if figures[collection.ID].Counts.Open != 2 {
		t.Errorf("figures %+v", figures)
	}
}

// A hub's figure is the sum of its collections' counts, and they are counted as a collection's own
// are: a hub listed inside another use case's transaction never counts in it either.
func TestAHubFigureIsNeverCountedInTheCallersTransaction(t *testing.T) {
	h := newStructureHarness(t)
	store := &joinedStore{figureStore: figureStore{counts: map[shared.ID]domain.FigureCounts{
		collectionID: {Open: 4}, otherCollectionID: {Open: 3},
	}}}
	figures := FigureReader{Figures: store, Clock: clock.Fixed(now), Profiles: &profiles{rows: seededProfiles()}}
	reader := HubFigureReader{
		Collections: hubCollections{store: h.containers}, Structure: h.set.Structure, Figures: figures,
	}
	hub := h.containers.stored[hubID]
	uow := &detachingUnitOfWork{}
	ctx := context.WithValue(context.Background(), callersTransaction{}, true)
	structures, err := h.set.Structure.Resolve(ctx, actor(), uow, []domain.Container{hub})
	if err != nil {
		t.Fatalf("structure: %v", err)
	}

	counted := reader.Count(ctx, actor(), uow, []domain.Container{hub}, structures)
	if store.joined {
		t.Fatal("the hub's figure was counted in the caller's transaction")
	}
	if counted[hubID].Counts.Open != 7 {
		t.Errorf("figures %+v", counted)
	}
}
