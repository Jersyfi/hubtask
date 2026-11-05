// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// MaxHubFigureCollections is how many collections of one hub a hub's figure counts on a read. A hub
// holding more answers its figure as unavailable rather than as a sum of some of them.
const MaxHubFigureCollections = 500

// HubFigureReader counts a hub's figure on its read and on the hub level of the list (UC-WRK-29,
// user-experience.md §2.5): the open entries directly under its collections that are neither
// archived nor trashed - what the tree shows under it - each counted as its own OPEN figure, so
// that what its structure hides counts nowhere and a kept count answers as it does for the
// collection. Called after the permission question: a reader of the hub reaches all of it.
type HubFigureReader struct {
	Collections repository.HubCollections
	Structure   StructureResolver
	Figures     FigureReader
}

// Wired reports whether the reader can count. A read built without one answers no hub figure.
func (r HubFigureReader) Wired() bool {
	return r.Collections != nil && r.Structure.Wired() && r.Figures.Wired()
}

// Count answers the figures of the hubs among containers, keyed by hub. No figure for a trashed
// hub or one whose own structure counts nothing; an unavailable one where its collections could not
// be read or counted, or where it holds more than MaxHubFigureCollections.
func (r HubFigureReader) Count(
	ctx context.Context, actor appshared.ActorContext, uow persistence.UnitOfWork,
	containers []domain.Container, structures map[shared.ID]domain.EffectiveStructure,
) map[shared.ID]domain.CollectionFigure {
	figures := map[shared.ID]domain.CollectionFigure{}
	if !r.Wired() || uow == nil {
		return figures
	}
	var hubs []shared.ID
	for _, container := range containers {
		effective, resolved := structures[container.ID]
		if container.Type != domain.ContainerHub || container.DeletedAt != nil || !resolved {
			continue
		}
		if _, counted := domain.FigureRequestFor(container.ID, effective); counted {
			hubs = append(hubs, container.ID)
		}
	}
	if len(hubs) == 0 {
		return figures
	}
	window := r.Figures.window(actor)

	var children map[shared.ID][]domain.Container
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		children, err = r.Collections.CollectionsOfHubs(ctx, hubs, MaxHubFigureCollections+1)
		return err
	})
	var collections []domain.Container
	if err == nil {
		for _, hub := range hubs {
			if len(children[hub]) <= MaxHubFigureCollections {
				collections = append(collections, children[hub]...)
			}
		}
	}
	var childStructures map[shared.ID]domain.EffectiveStructure
	if err == nil {
		childStructures, err = r.Structure.Resolve(ctx, actor, uow, collections)
	}
	if err != nil {
		for _, hub := range hubs {
			figures[hub] = domain.HubUnavailableFigure(window)
		}
		return figures
	}

	requests := make([]domain.FigureRequest, 0, len(collections))
	for _, collection := range collections {
		requests = append(requests, domain.FigureRequest{CollectionID: collection.ID, Kind: domain.FigureOpen})
	}
	var counted map[shared.ID]domain.CollectionFigure
	if len(requests) > 0 {
		counted = r.Figures.countRequests(ctx, actor, uow, requests, childStructures, window)
	}
	for _, hub := range hubs {
		if len(children[hub]) > MaxHubFigureCollections {
			figures[hub] = domain.HubUnavailableFigure(window)
			continue
		}
		each := make([]domain.CollectionFigure, 0, len(children[hub]))
		for _, collection := range children[hub] {
			figure, answered := counted[collection.ID]
			if !answered {
				figure = domain.HubUnavailableFigure(window)
			}
			each = append(each, figure)
		}
		figures[hub] = domain.HubFigure(window, each)
	}
	return figures
}
