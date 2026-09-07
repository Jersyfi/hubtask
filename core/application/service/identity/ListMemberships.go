// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListMembershipsName = "ListMemberships"
	membersRead         = "members:read"

	// MembershipReadAction is the audit code of an attempted read. Declared even though an
	// ordinary read writes no entry: a refused one at the workspace scope does, recorded against
	// the action that was refused (audit.md §4).
	MembershipReadAction audit.Action = "membership.read"
)

// ContainerFinder is the one question this package asks the container repository: which hub or
// collection is this, and what sits above it. Narrower than repository/work.Containers so that a
// test can answer it in five lines.
type ContainerFinder interface {
	Find(ctx context.Context, id shared.ID) (work.Container, error)
}

// ItemFinder is the same question about an entry.
type ItemFinder interface {
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
}

// Permitter answers a permission question without recording a refusal. It is how a read decides
// to say "not found" rather than "forbidden": the trail is not filled with denials nobody acted
// on, and the answer discloses nothing (T-04).
type Permitter interface {
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// ListMembershipsQuery is the input, typed.
type ListMembershipsQuery struct {
	ScopeType domain.ScopeType
	ScopeID   shared.ID
	Cursor    string
	Size      int
}

// ListMemberships answers who holds a role at a scope (F3-01).
//
// What is granted *at* the scope named and nothing granted elsewhere. A membership applies
// downwards, so what is in force at a collection is this list plus what its hub and the workspace
// grant - and the client composes that path itself, because it knows the path it is on. An
// `effective` listing that walked it server-side is deliberately not built until a second caller
// wants one.
//
// Who may read is `READ` at the scope. A member of a hub seeing who else is in it is what makes a
// shared workspace legible, `data-protection.md` §9 already limits what a name reveals to the
// minimum, and `AUDITOR` holds no `READ` and is refused by the ordinary rule rather than by a
// special case. A scope the caller holds nothing on is not found, in the words a missing one
// produces (T-04): a hub they cannot see and a hub that does not exist must be the same answer,
// or the difference is an oracle for which identifiers exist. The workspace itself is the one
// scope whose existence is no secret, so a refusal there is a refusal, and it is audited.
//
// Read-only throughout: the transaction may be served by a replica (multi-tenancy.md §7).
type ListMemberships struct {
	Grants     repository.MembershipGrants
	Containers ContainerFinder
	Items      ItemFinder
	Authorizer Authorizer
	Permits    Permitter
	UnitOfWork persistence.UnitOfWork
}

// Execute returns one page of the memberships granted at the scope.
func (h ListMemberships) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListMembershipsQuery,
) (repository.GrantPage, error) {
	scope, err := scopeOf(query.ScopeType, query.ScopeID)
	if err != nil {
		return repository.GrantPage{}, err
	}
	if err := h.mayRead(ctx, actor, scope); err != nil {
		return repository.GrantPage{}, err
	}

	var page repository.GrantPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Grants.ListAt(ctx, scope, repository.Page{
			Cursor: query.Cursor, Size: pageSize(query.Size),
		})
		return err
	})
	if err != nil {
		return repository.GrantPage{}, err
	}
	return page, nil
}

// scopeOf is the scope as the caller named it, checked before anything is read: a type that is not
// a level, a workspace scope carrying an identifier, or a container scope without one are the
// caller's mistake and are answered as such rather than as "not found".
func scopeOf(scopeType domain.ScopeType, id shared.ID) (domain.Scope, error) {
	switch scopeType {
	case domain.ScopeTenant:
		if !id.IsZero() {
			return domain.Scope{}, shared.ErrValidation.
				WithDetail("memberships.scope_identifier_unexpected").
				WithFields(shared.FieldError{Path: "/scope_id", Code: "memberships.scope_identifier_unexpected"})
		}
	case domain.ScopeHub, domain.ScopeCollection, domain.ScopeItem:
		if id.IsZero() {
			return domain.Scope{}, shared.ErrValidation.
				WithDetail("memberships.scope_identifier_required").
				WithFields(shared.FieldError{Path: "/scope_id", Code: "memberships.scope_identifier_required"})
		}
	default:
		return domain.Scope{}, shared.ErrValidation.
			WithDetail("memberships.scope_invalid").
			WithParams(map[string]string{"value": string(scopeType)}).
			WithFields(shared.FieldError{Path: "/scope_type", Code: "memberships.scope_invalid"})
	}
	return domain.Scope{Type: scopeType, ID: id}, nil
}

// mayRead decides the read. The workspace scope goes through the authorisation service, so that a
// refusal is recorded; every other scope is resolved to its path and asked without a record, so
// that the refusal can be the not-found answer T-04 asks for.
func (h ListMemberships) mayRead(ctx context.Context, actor appshared.ActorContext, scope domain.Scope) error {
	if scope.Type == domain.ScopeTenant {
		return h.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionRead,
			Path:       []domain.Scope{domain.TenantScope()},
			Action:     MembershipReadAction,
			TokenScope: membersRead,
			TargetType: membershipTarget,
		})
	}

	// The token scope is the one bound Permits does not check, and a credential bounds its
	// holder before any path is walked (ADR-0005).
	if err := actor.RequireScope(membersRead); err != nil {
		return err
	}
	path, err := h.pathTo(ctx, actor, scope)
	if err != nil {
		return err
	}
	permitted, err := h.Permits.Permits(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       path,
		Action:     MembershipReadAction,
		TokenScope: membersRead,
		TargetType: membershipTarget,
		TargetID:   scope.ID,
	})
	if err != nil {
		return err
	}
	if !permitted {
		return notReachable(scope)
	}
	return nil
}

// pathTo is the authorisation path of the scope, from the tenant downwards: the hub a collection
// sits in, the collection an entry sits in. A membership held anywhere on it counts, which is what
// "the highest role along the path" means (domain-model.md §3.2) - a path that stopped at the
// collection would refuse somebody whose right sits on the hub.
//
// The row is read before the permission question can be asked, because the answer depends on it.
// Nothing is disclosed by the order: the tenant boundary is row level security, which has already
// applied, and what is read here is returned only after the check passes.
func (h ListMemberships) pathTo(
	ctx context.Context, actor appshared.ActorContext, scope domain.Scope,
) ([]domain.Scope, error) {
	var path []domain.Scope
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		switch scope.Type {
		case domain.ScopeHub, domain.ScopeCollection:
			container, err := h.Containers.Find(ctx, scope.ID)
			if err != nil {
				return notReachableOr(err, scope)
			}
			// A hub identifier presented as a collection scope, or the other way round, names
			// nothing that exists at that level.
			if (container.Type == work.ContainerHub) != (scope.Type == domain.ScopeHub) {
				return notReachable(scope)
			}
			path = service.ContainerScopes(container)
		case domain.ScopeItem:
			item, err := h.Items.Find(ctx, scope.ID)
			if err != nil {
				return notReachableOr(err, scope)
			}
			collection, err := h.Containers.Find(ctx, item.CollectionID)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return shared.ErrInternal.WithDetail("items.collection_missing").WithCause(err)
				}
				return err
			}
			path = append(service.ContainerScopes(collection), domain.ItemScope(item.ID))
		default:
			path = []domain.Scope{domain.TenantScope()}
		}
		return nil
	})
	return path, err
}

// notReachable is the one answer for a scope the actor cannot reach, whatever the reason - in the
// words the repository uses for a row that is not there, so that the two cannot be told apart
// (T-04, appshared.ItemNotFound).
func notReachable(scope domain.Scope) error {
	if scope.Type == domain.ScopeItem {
		return appshared.ItemNotFound(scope.ID)
	}
	return shared.ErrNotFound.WithDetail("containers.not_found")
}

func notReachableOr(err error, scope domain.Scope) error {
	if errors.Is(err, shared.ErrNotFound) {
		return notReachable(scope)
	}
	return err
}

// Descriptor registers the use case in all three channels.
func (h ListMemberships) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListMembershipsName,
		Summary: "Lists who holds a role at a scope: the memberships granted at exactly that " +
			"workspace, hub, collection or entry, newest first, and none granted elsewhere. A " +
			"role applies downwards, so what is in force at a collection is this list plus what " +
			"its hub and the workspace grant; walk the path yourself. A row carries an " +
			"identifier and no name - resolve it through getAccount or getGroup.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Enum: []string{string(domain.ScopeTenant), string(domain.ScopeHub),
					string(domain.ScopeCollection), string(domain.ScopeItem)},
				Description: "TENANT for the whole workspace; otherwise the level the scope identifier names.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "The hub, collection or entry. Omitted for the whole workspace, and only then.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the newest grant.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many memberships to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MembershipReadAction, TargetType: membershipTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListMemberships) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	scopeID, err := in.ID("scope_id")
	if err != nil {
		return nil, err
	}
	page, err := h.Execute(ctx, actor, ListMembershipsQuery{
		ScopeType: domain.ScopeType(in.String("scope_type")),
		ScopeID:   scopeID,
		Cursor:    in.String("cursor"),
		Size:      in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Grants))
	for _, grant := range page.Grants {
		rows = append(rows, grantOutput(grant))
	}
	return pageOutput(rows, page.Info), nil
}
