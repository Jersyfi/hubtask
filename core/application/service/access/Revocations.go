// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Permitter answers a permission question without recording it - Service.Permits, or a fake.
type Permitter interface {
	Permits(ctx context.Context, actor appshared.ActorContext, request Request) (bool, error)
}

// Revocations tells the devices of a person that their read access has ended (N-08,
// offline-sync.md §6).
//
// A phone that holds a hub its owner was removed from keeps the hub until something says
// otherwise, and nothing did: `RevokeMembership` and the group changes wrote their audit entry and
// their event and told no device. This writes the one record that says it - an `ACCESS_REVOKED`
// entry at the container root, `actor_id` naming the account that lost access - and it is
// written where the *effective* access ends: after the removal, inside its transaction, the
// question "may this account still read here?" is put to the same resolution every request
// uses. A person who loses a group but keeps a direct grant loses nothing and is told nothing.
//
// The record is addressed to a person rather than filtered by permission, which is why it is the
// one record the stream's filter does not ask the container about (sync.StreamChanges).
type Revocations struct {
	Permits    Permitter
	Grants     GrantReader
	Groups     GroupMembers
	Containers ContainerReader
	Items      ItemReader
	Changes    changelog.ChangeLog
	HLC        clock.HLCSource
}

// The slices of the repositories this reads - named so that a test fakes four methods rather
// than thirty, and so that the file says what it touches.
type (
	GrantReader interface {
		ListAt(ctx context.Context, scope identity.Scope, page identityrepo.Page) (identityrepo.GrantPage, error)
		OfGroup(ctx context.Context, groupID shared.ID) ([]identity.Grant, error)
	}
	GroupMembers interface {
		Members(ctx context.Context, groupID shared.ID) ([]shared.ID, error)
	}
	ContainerReader interface {
		Find(ctx context.Context, id shared.ID) (work.Container, error)
		List(ctx context.Context, query workrepo.ContainerQuery) (workrepo.ContainerPage, error)
	}
	ItemReader interface {
		Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
	}
)

// Loss is one removal as the use case that made it describes it: the accounts that held
// something through what was removed, and the scope it was held at.
type Loss struct {
	Accounts []shared.ID
	Scope    identity.Scope
}

// AfterGrantRevoked describes the loss a revoked grant is: the account named, or every member of
// the group named, at the grant's scope. Resolved before the grant is gone, because a group's
// members are read from the group rather than from the grant.
func (r Revocations) AfterGrantRevoked(ctx context.Context, grant identity.Grant) (Loss, error) {
	if !grant.GroupID.IsZero() {
		members, err := r.Groups.Members(ctx, grant.GroupID)
		if err != nil {
			return Loss{}, err
		}
		return Loss{Accounts: members, Scope: grant.Scope}, nil
	}
	return Loss{Accounts: []shared.ID{grant.AccountID}, Scope: grant.Scope}, nil
}

// AfterGroupLoss describes what accounts taken out of a group - or every member of a group about
// to be deleted - stand to lose: one loss per grant the group holds. Read before the removal, for
// the same reason: a deleted group takes its grants with it.
func (r Revocations) AfterGroupLoss(
	ctx context.Context, groupID shared.ID, accounts []shared.ID,
) ([]Loss, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	grants, err := r.Grants.OfGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	losses := make([]Loss, 0, len(grants))
	for _, grant := range grants {
		losses = append(losses, Loss{Accounts: accounts, Scope: grant.Scope})
	}
	return losses, nil
}

// AfterContainerMoved describes who may have lost a collection by its move under another hub:
// everybody who held a role on the hub it left, directly or through a group. Whoever reads the
// collection some other way - a role on the collection itself, or on the workspace - is still
// asked and still keeps it; the list is who to ask, not who lost.
func (r Revocations) AfterContainerMoved(ctx context.Context, from, to work.Container) (Loss, error) {
	if from.ParentID.IsZero() || from.ParentID == to.ParentID {
		return Loss{}, nil
	}
	accounts, err := r.holdersAt(ctx, identity.HubScope(from.ParentID))
	if err != nil {
		return Loss{}, err
	}
	return Loss{Accounts: accounts, Scope: identity.CollectionScope(to.ID)}, nil
}

// holdersAt is every account with a role granted at exactly the scope, through a group or not.
func (r Revocations) holdersAt(ctx context.Context, scope identity.Scope) ([]shared.ID, error) {
	seen := map[shared.ID]bool{}
	var accounts []shared.ID
	add := func(account shared.ID) {
		if !seen[account] {
			seen[account] = true
			accounts = append(accounts, account)
		}
	}
	page := identityrepo.Page{Size: holdersPage}
	for {
		grants, err := r.Grants.ListAt(ctx, scope, page)
		if err != nil {
			return nil, err
		}
		for _, grant := range grants.Grants {
			if grant.GroupID.IsZero() {
				add(grant.AccountID)
				continue
			}
			members, err := r.Groups.Members(ctx, grant.GroupID)
			if err != nil {
				return nil, err
			}
			for _, member := range members {
				add(member)
			}
		}
		if !grants.Info.HasMore {
			return accounts, nil
		}
		page.Cursor = grants.Info.NextCursor
	}
}

// holdersPage is how many grants at a scope are read at once while collecting who holds one.
const holdersPage = 100

// Announce writes the record for every account of the loss that may no longer read what the scope
// names. It runs inside the transaction that made the removal, after it, so that the resolution
// sees the removal and nothing else that has not happened yet.
//
// A loss at the workspace is announced per hub: the device applies a revocation to the subtree
// under the root it names, and the workspace is not a root a device holds. A loss at an entry is
// announced for the entry, under its collection.
func (r Revocations) Announce(ctx context.Context, tenantID shared.ID, losses ...Loss) error {
	for _, loss := range losses {
		roots, err := r.rootsOf(ctx, loss.Scope)
		if err != nil {
			return err
		}
		for _, account := range loss.Accounts {
			for _, root := range roots {
				if err := r.announceTo(ctx, tenantID, account, root); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// root is one thing a device holds by path prefix, and the path the permission is decided along.
type root struct {
	entity      string
	entityID    shared.ID
	containerID shared.ID
	path        []identity.Scope
}

func (r Revocations) rootsOf(ctx context.Context, scope identity.Scope) ([]root, error) {
	switch scope.Type {
	case identity.ScopeHub, identity.ScopeCollection:
		container, err := r.Containers.Find(ctx, scope.ID)
		if err != nil {
			return nil, gone(err)
		}
		return []root{containerRoot(container)}, nil
	case identity.ScopeItem:
		item, err := r.Items.Find(ctx, scope.ID)
		if err != nil {
			return nil, gone(err)
		}
		collection, err := r.Containers.Find(ctx, item.CollectionID)
		if err != nil {
			return nil, gone(err)
		}
		return []root{{
			entity: entityItem, entityID: item.ID, containerID: collection.ID,
			path: append(service.ContainerScopes(collection), identity.ItemScope(item.ID)),
		}}, nil
	case identity.ScopeTenant:
		return r.hubs(ctx)
	default:
		return nil, nil
	}
}

// hubs is every hub of the workspace, archived ones included: a device holds an archived hub too.
func (r Revocations) hubs(ctx context.Context) ([]root, error) {
	var roots []root
	query := workrepo.ContainerQuery{
		Type: work.ContainerHub, IncludeArchived: true, Page: workrepo.Page{Size: holdersPage},
	}
	for {
		page, err := r.Containers.List(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, hub := range page.Containers {
			roots = append(roots, containerRoot(hub))
		}
		if !page.Info.HasMore {
			return roots, nil
		}
		query.Page.Cursor = page.Info.NextCursor
	}
}

func containerRoot(container work.Container) root {
	return root{
		entity: entityContainer, entityID: container.ID, containerID: container.ID,
		path: service.ContainerScopes(container),
	}
}

// gone turns a root that is no longer there into nothing to announce: a container in the trash
// is not one a revocation reaches, and its own deletion is what the device applies.
func gone(err error) error {
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	return err
}

func (r Revocations) announceTo(ctx context.Context, tenantID, account shared.ID, at root) error {
	still, err := r.Permits.Permits(ctx, appshared.ActorContext{
		TenantID: tenantID, AccountID: account, Kind: shared.ActorUser,
	}, Request{Permission: service.PermissionRead, Path: at.path})
	if err != nil {
		return err
	}
	if still {
		return nil
	}
	return r.Changes.Record(ctx, changelog.Change{
		TenantID:    tenantID,
		Entity:      at.entity,
		EntityID:    at.entityID,
		Op:          changelog.AccessRevoked,
		ContainerID: at.containerID,
		ActorID:     account,
		HLC:         r.HLC.Next(),
	})
}

// The entity names the change log records under, as the walk hands them out
// (sync.InitialSync); repeated here rather than imported, because sync reads this package's
// records and must not be read by it.
const (
	entityContainer = "container"
	entityItem      = "item"
)
