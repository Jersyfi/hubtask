// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// MaxGroupPeople bounds the people one group word becomes: they are bound as one list, and a
// query over more people than this belongs to a hub or a role, not a group word.
const MaxGroupPeople = 1000

// GroupPeople answers a query's group words - `my-groups` and `members("…")` - with the asker's
// rights: the question ListGroups asks (any member, the token's members scope), then the people.
type GroupPeople struct {
	// Members asks whether the asker is a member of the workspace; nil refuses everybody.
	Members    Authorizer
	Groups     repository.GroupPeople
	UnitOfWork persistence.UnitOfWork
}

// People reads each group the query names, keyed by view.Placeholder.GroupKey.
func (g GroupPeople) People(ctx context.Context, actor appshared.ActorContext, wanted []view.Placeholder) (map[string][]shared.ID, error) {
	if err := authorizeGroupRead(ctx, actor, g.Members); err != nil {
		return nil, err
	}
	people := make(map[string][]shared.ID, len(wanted))
	err := g.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for _, p := range wanted {
			var (
				read []shared.ID
				err  error
			)
			if p.Kind == view.PlaceholderMyGroups {
				read, err = g.Groups.PeopleOfGroupsOf(ctx, actor.AccountID, MaxGroupPeople+1)
			} else {
				read, err = g.Groups.PeopleOfGroup(ctx, p.Group, MaxGroupPeople+1)
			}
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrValidation.WithDetail("query.group_unknown").
					WithParams(map[string]string{"group": p.Group})
			}
			if err != nil {
				return err
			}
			if len(read) > MaxGroupPeople {
				return shared.ErrValidation.WithDetail("query.group_too_large").
					WithParams(map[string]string{"maximum": strconv.Itoa(MaxGroupPeople)})
			}
			people[p.GroupKey()] = read
		}
		return nil
	})
	return people, err
}
