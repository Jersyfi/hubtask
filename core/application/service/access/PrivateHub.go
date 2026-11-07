// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// AuthorizeOwnPrivateHub answers whether the actor may create a private hub for themselves
// (ADR-0073 §2): a person - never a service account - who holds a role anywhere in the workspace,
// whatever it is, with the token scope the request names. No STRUCTURE: the person who most
// needs a hub nobody else opens is the one without an administrator's role.
//
// Whether the workspace allows private hubs is not asked here: *Private hubs allowed* is a setting,
// not a permission, and the use case reads it after this answered (ADR-0073 §6).
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

	holds, err := s.holdsAny(ctx, actor)
	if err != nil {
		return err
	}
	if !holds {
		s.recordRefusal(ctx, actor, request, "permission")
		return notPermitted(request)
	}
	return nil
}

// MayOwnPrivateHub answers AuthorizeOwnPrivateHub's question without recording anything, for the
// manifest: a person - never a service account - who holds a role anywhere in the workspace. The
// token's scope is not asked, as `HeldOnWorkspace` does not ask it: the manifest says what the
// person may do, and a token bounds it further.
func (s Service) MayOwnPrivateHub(ctx context.Context, actor appshared.ActorContext) (bool, error) {
	if !actor.IsAuthenticated() || actor.Kind != appshared.ActorUser {
		return false, nil
	}
	return s.holdsAny(ctx, actor)
}

// holdsAny is whether the actor's account holds a role anywhere in the workspace - the one reading
// both the refusal and the manifest take, so the two cannot disagree.
func (s Service) holdsAny(ctx context.Context, actor appshared.ActorContext) (bool, error) {
	var holds bool
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		holds, err = s.Memberships.HoldsAny(ctx, actor.AccountID)
		return err
	})
	return holds, err
}

// authorizeMember is Authorize for a request that asks AnyMember, its token scope already judged.
func (s Service) authorizeMember(ctx context.Context, actor appshared.ActorContext, request Request) error {
	reason, err := s.anyMember(ctx, actor)
	if err != nil || reason == "" {
		return err
	}
	request.Permission = service.PermissionRead
	s.recordRefusal(ctx, actor, request, reason)
	return notPermitted(request)
}

// anyMember answers why the actor is no member of the workspace, empty when it is one: its account
// holds a built-in role anywhere or one of the workspace's own that carries READ (HoldsAny) - which
// a requester's REQUEST is not -, and so does the person an agent acts for or an administrator
// views as (alsoAsked). The system itself, acting for no account, reads as a member.
func (s Service) anyMember(ctx context.Context, actor appshared.ActorContext) (string, error) {
	if actor.AccountID.IsZero() {
		if actor.Kind == appshared.ActorSystem {
			return "", nil
		}
		return "permission", nil
	}
	holds, err := s.holdsAny(ctx, actor)
	if err != nil || !holds {
		return "permission", err
	}
	person, reason := alsoAsked(actor)
	if reason == "" {
		return "", nil
	}
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		holds, err = s.Memberships.HoldsAny(ctx, person)
		return err
	})
	if err != nil || !holds {
		return reason, err
	}
	return "", nil
}

// Hidden answers whether the path runs through a private hub the actor holds nothing in - a hub
// that, for them, is not there (ADR-0073 §1). Nothing is recorded: it is asked about somebody on
// whose behalf something would run - a rule's run_as, a subscription's creator - before anything
// is read for them, and nobody was refused anything.
//
// It answers only the private hub's question. A path the actor simply holds no role on is not
// hidden, so a reader that never asked before asks nothing new of a shared hub.
func (s Service) Hidden(
	ctx context.Context, actor appshared.ActorContext, path []identity.Scope,
) (bool, error) {
	if len(path) == 0 {
		return false, nil
	}
	memberships, at, err := s.heldAlong(ctx, actor.PersistenceScope(), actor.AccountID, path, false)
	if err != nil {
		return false, err
	}
	return hidden(memberships, at), nil
}

// Requester answers whether the actor follows the path as a requester - REQUEST without READ
// (service.IsRequester) -, who learns of work there only through the portal. Nothing is recorded:
// it is asked about the person a notification would go to, before anything is written for them.
func (s Service) Requester(
	ctx context.Context, actor appshared.ActorContext, path []identity.Scope,
) (bool, error) {
	if len(path) == 0 || actor.AccountID.IsZero() {
		return false, nil
	}
	memberships, at, err := s.heldAlong(ctx, actor.PersistenceScope(), actor.AccountID, path, false)
	if err != nil {
		return false, err
	}
	return service.IsRequester(memberships, at.path, at.privacy), nil
}
