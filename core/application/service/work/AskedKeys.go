// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// GroupPeople answers the people of each group a query names (`my-groups`, `members("…")`),
// keyed by view.Placeholder.GroupKey, read with the asker's rights to read groups.
type GroupPeople interface {
	People(ctx context.Context, actor appshared.ActorContext, wanted []view.Placeholder) (map[string][]shared.ID, error)
}

// CriticalPath answers the entries on a plan's critical path as the scheduler computes it for the
// asker - a hub or a collection, the entries the asker may read (PLN-03).
type CriticalPath interface {
	Critical(ctx context.Context, actor appshared.ActorContext, planID shared.ID) ([]shared.ID, error)
}

// askGroups reads the people of the groups a filter names; a door without the reader refuses the
// group words rather than answering them as nobody.
func askGroups(ctx context.Context, actor appshared.ActorContext, groups GroupPeople, filter *view.Node) (map[string][]shared.ID, error) {
	wanted := view.NeedsOf(filter).Groups
	if len(wanted) == 0 {
		return nil, nil
	}
	if groups == nil {
		return nil, shared.ErrValidation.WithDetail("query.groups_not_here")
	}
	return groups.People(ctx, actor, wanted)
}

// askedKeys answers the keys a statement cannot answer alone for a query anchored at a hub, a
// collection or an entry: whose time `time.logged` counts - the time module's TimeReach over
// the anchor's collections - and the critical path of the hub or collection the query reaches.
func (h QueryItems) askedKeys(
	ctx context.Context, actor appshared.ActorContext, filter *view.Node, anchor repository.Anchor, container domain.Container,
) (*view.Node, error) {
	needs := view.NeedsOf(filter)
	if !needs.Time && !needs.Critical {
		return filter, nil
	}
	var asked view.Asked
	if needs.Time {
		collections := []domain.Container{container}
		if anchor.Kind == repository.AnchorHub {
			collections = nil
		}
		sight, err := timeSightOf(ctx, actor, h, h.Time, anchor, collections)
		if err != nil {
			return nil, err
		}
		asked.Sight = sight
	}
	if needs.Critical {
		if h.Critical == nil {
			return nil, shared.ErrValidation.WithDetail("query.key_not_here").
				WithParams(map[string]string{"field": view.FieldCritical})
		}
		plan := anchor.CollectionID
		switch anchor.Kind {
		case repository.AnchorHub:
			plan = anchor.HubID
		case repository.AnchorCollection:
		default:
			// An entry's subtree is no plan; the scheduler plans a hub or a collection.
			return nil, shared.ErrValidation.WithDetail("query.critical_needs_plan")
		}
		critical, err := h.Critical.Critical(ctx, actor, plan)
		if err != nil {
			return nil, err
		}
		asked.Critical = critical
	}
	answered := filter.Answer(asked)
	return &answered, nil
}

// timeSightOf decides whose records count: the reader's own, and other people's on the entries
// of each collection where the time module says the reader sees them - a hub's collections read
// one by one. Where the workspace shows other people's time only as sums, Withhold says so.
func timeSightOf(
	ctx context.Context, actor appshared.ActorContext, q QueryItems, reach TimeReach,
	anchor repository.Anchor, collections []domain.Container,
) (view.TimeSight, error) {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return view.TimeSight{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	sight := view.TimeSight{Reader: actor.AccountID, Shown: []shared.ID{}}
	if reach == nil {
		return sight, nil
	}
	if anchor.Kind == repository.AnchorHub {
		err := q.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			page := repository.Page{Size: 200}
			for {
				read, err := q.Containers.List(ctx, repository.ContainerQuery{
					ParentID: anchor.HubID, Type: domain.ContainerCollection, IncludeArchived: true, Page: page,
				})
				if err != nil {
					return err
				}
				collections = append(collections, read.Containers...)
				if len(collections) > MaxSummaryCollections || !read.Info.HasMore {
					return nil
				}
				page.Cursor = read.Info.NextCursor
			}
		})
		if err != nil {
			return view.TimeSight{}, err
		}
	}
	shown, sumsOnly, err := reach.Reach(ctx, actor, collections)
	if err != nil {
		return view.TimeSight{}, err
	}
	for i, collection := range collections {
		if i < len(shown) && shown[i] {
			sight.Shown = append(sight.Shown, collection.ID)
		}
	}
	sight.Withhold = sumsOnly && len(sight.Shown) > 0
	return sight, nil
}
