// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// AuthorizeOwnPrivateHub answers whether the actor may create a private hub for themselves
// (ADR-0073 §2): a person - never a service account - who holds a role anywhere in the workspace,
// whatever it is, with the token scope the request names. No STRUCTURE: the person who most
// needs a hub nobody else opens is the one without an administrator's role.
//
// Where the workspace allows private hubs is always, until the workspace's setting exists; that
// setting is read here when it does, so that the question stays one question.
//
// A refusal is recorded like every other, against the request's action.
func (s Service) AuthorizeOwnPrivateHub(
	ctx context.Context, actor appshared.ActorContext, request Request,
) error {
	if !actor.IsAuthenticated() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if request.TokenScope != "" {
		if err := actor.RequireScope(request.TokenScope); err != nil {
			s.recordRefusal(ctx, actor, request, "scope")
			return err
		}
	}
	request.Permission = service.PermissionRead
	if actor.Kind != appshared.ActorUser {
		s.recordRefusal(ctx, actor, request, "permission")
		return notPermitted(request)
	}

	var holds bool
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		holds, err = s.Memberships.HoldsAny(ctx, actor.AccountID)
		return err
	})
	if err != nil {
		return err
	}
	if !holds {
		s.recordRefusal(ctx, actor, request, "permission")
		return notPermitted(request)
	}
	return nil
}
