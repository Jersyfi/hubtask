// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// wayInCeiling holds a new way into somebody else's account - a start password, an address, an
// invitation link, a service account's token - to the ceiling a grant is held to (identity.md
// §13, domain-model.md §3.2): whoever issues it can act as the account, so it is as if they had
// handed themselves every role the account holds. An owner issues one for anybody, an
// administrator for nobody holding OWNER; refused with `access.role_above_own` and recorded, by
// the authoriser, which is where every grant's ceiling is decided (rule 2).
//
// The account's strongest role anywhere in the workspace is measured against the actor's own on
// the workspace's path - where the door's own permission is asked - and not each membership along
// its own path as a grant is. A credential reaches every scope the account holds at once; and a
// private hub's path discounts the workspace's roles, so measured there a workspace owner would be
// refused for every member of a private hub, against "an owner issues one for anybody".
//
// The roles are read before, and the answer asked after, the read-only transaction: a refusal's
// trail entry must not be rolled back with it.
type wayInCeiling struct {
	Held       repository.HeldRoles
	Authorizer Authorizer
}

// heldBy reads what the account holds, inside the caller's transaction.
func (c wayInCeiling) heldBy(ctx context.Context, accountID shared.ID) ([]domain.Role, error) {
	return c.Held.HeldAnywhere(ctx, accountID)
}

// hold asks the authoriser the door's own question again, now naming the strongest of the roles
// as what is at stake. An account that holds none is held to nothing more than the permission.
func (c wayInCeiling) hold(
	ctx context.Context, actor appshared.ActorContext, request access.Request, held []domain.Role,
) error {
	strongest, found := domain.Strongest(held)
	if !found {
		return nil
	}
	request.Grants = strongest
	return c.Authorizer.Authorize(ctx, actor, request)
}
