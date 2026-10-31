// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MembershipRepository reads what an account holds.
type MembershipRepository struct{}

func NewMembershipRepository() MembershipRepository { return MembershipRepository{} }

var _ repository.Memberships = MembershipRepository{}

// Along returns the memberships that could apply to the path, held directly or through a group.
//
// The tenant is not a parameter: row level security bounds the query to the tenant of the running
// transaction, so a membership from another tenant cannot reach the resolution even if its
// identifier were guessed correctly (ADR-0010).
func (r MembershipRepository) Along(
	ctx context.Context, accountID shared.ID, path []identity.Scope,
) ([]identity.Membership, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}

	scopeIDs := make([]pgtype.UUID, 0, len(path))
	for _, scope := range path {
		if scope.ID.IsZero() {
			// The tenant scope carries no identifier; the query matches it by its type.
			continue
		}
		id, err := uuidOf(scope.ID)
		if err != nil {
			return nil, err
		}
		scopeIDs = append(scopeIDs, id)
	}

	rows, err := queries.MembershipsAlongPath(ctx, sqlc.MembershipsAlongPathParams{
		AccountID: account,
		ScopeIds:  scopeIDs,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the memberships: %w", err))
	}

	memberships := make([]identity.Membership, 0, len(rows))
	for _, row := range rows {
		scopeID, err := optionalID(row.ScopeID)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, identity.Membership{
			AccountID: accountID,
			Scope:     identity.Scope{Type: identity.ScopeType(row.ScopeType), ID: scopeID},
			Role:      identity.Role(row.Role),
		})
	}
	custom, err := customRolesAlong(ctx, queries, accountID, account, scopeIDs)
	if err != nil {
		return nil, err
	}
	return append(memberships, custom...), nil
}

// SharedItemsIn returns the entries of one collection the account holds a membership on, directly
// or through a group.
//
// The tenant is not a parameter: row level security bounds the query to the tenant of the running
// transaction, so an entry of another tenant cannot reach the answer even if its identifier were
// guessed correctly (ADR-0010).
func (r MembershipRepository) SharedItemsIn(
	ctx context.Context, accountID, collectionID shared.ID,
) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.SharedItemsInCollection(ctx, sqlc.SharedItemsInCollectionParams{
		AccountID:    account,
		CollectionID: collection,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the shared entries: %w", err))
	}

	items := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := optionalID(row)
		if err != nil {
			return nil, err
		}
		items = append(items, id)
	}
	return items, nil
}

var _ repository.SharedEntries = MembershipRepository{}

// SharedAcross answers the entries shared with the account anywhere in the tenant of the running
// transaction (row level security, ADR-0010), at most limit of them.
func (r MembershipRepository) SharedAcross(
	ctx context.Context, accountID shared.ID, limit int,
) ([]repository.SharedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SharedItemsAcross(ctx, sqlc.SharedItemsAcrossParams{
		AccountID: account, RowLimit: int32(min(max(limit, 0), 1<<20)),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the entries shared across the workspace: %w", err))
	}
	entries := make([]repository.SharedEntry, 0, len(rows))
	for _, row := range rows {
		item, err := optionalID(row.ID)
		if err != nil {
			return nil, err
		}
		collection, err := optionalID(row.CollectionID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, repository.SharedEntry{ItemID: item, CollectionID: collection})
	}
	return entries, nil
}

// administratorRoles are the roles the retention warning treats as administrators
// (data-retention.md §6).
//
// Named here rather than passed in, because "who can answer a warning about work that is about to
// be deleted" is a property of the role matrix (domain-model.md §3.2): the two roles that shape
// the workspace. A caller that could choose would be a caller choosing its own audience.
var administratorRoles = []string{string(identity.RoleOwner), string(identity.RoleAdmin)}

// Administrators answers who administers anywhere on the path.
func (r MembershipRepository) Administrators(
	ctx context.Context, path []identity.Scope,
) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	scopeIDs := make([]pgtype.UUID, 0, len(path))
	for _, scope := range path {
		if scope.ID.IsZero() {
			// The tenant scope carries no identifier; the query matches it by its type.
			continue
		}
		id, err := uuidOf(scope.ID)
		if err != nil {
			return nil, err
		}
		scopeIDs = append(scopeIDs, id)
	}

	rows, err := queries.AdministratorsAlongPath(ctx, sqlc.AdministratorsAlongPathParams{
		Roles:    administratorRoles,
		ScopeIds: scopeIDs,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the administrators: %w", err))
	}

	administrators := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		administrators = append(administrators, id)
	}
	return administrators, nil
}

// HubsOpenedFor answers, of the named hubs, those the account holds a running emergency access to.
func (r MembershipRepository) HubsOpenedFor(
	ctx context.Context, accountID shared.ID, hubIDs []shared.ID, now time.Time,
) ([]shared.ID, error) {
	if len(hubIDs) == 0 || accountID.IsZero() {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	hubs, err := uuidsOf(hubIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.HubsOpenedFor(ctx, sqlc.HubsOpenedForParams{
		AccountID: account, HubIds: hubs, Now: timestampOf(now),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the emergency accesses: %w", err))
	}
	return idsFrom(rows)
}

// HubsOf answers the hub each named container or entry sits under.
//
// The tenant is not a parameter: row level security bounds the query to the tenant of the running
// transaction, so another tenant's container answers nothing even when its identifier is known
// (ADR-0010).
func (r MembershipRepository) HubsOf(
	ctx context.Context, ids []shared.ID,
) (map[shared.ID]repository.Hub, error) {
	if len(ids) == 0 {
		return map[shared.ID]repository.Hub{}, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	named := make([]pgtype.UUID, 0, len(ids))
	for _, each := range ids {
		id, err := uuidOf(each)
		if err != nil {
			return nil, err
		}
		named = append(named, id)
	}

	rows, err := queries.HubsOf(ctx, named)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the hubs: %w", err))
	}

	hubs := make(map[shared.ID]repository.Hub, len(rows))
	for _, row := range rows {
		namedID, err := idFrom(row.NamedID)
		if err != nil {
			return nil, err
		}
		hubID, err := idFrom(row.HubID)
		if err != nil {
			return nil, err
		}
		hub := repository.Hub{ID: hubID, Private: row.Private}
		if row.CollectionID.Valid {
			if hub.Collection, err = idFrom(row.CollectionID); err != nil {
				return nil, err
			}
		}
		hubs[namedID] = hub
	}
	return hubs, nil
}

var _ repository.HeldRoles = MembershipRepository{}

// HeldAnywhere answers each role the account holds anywhere in the transaction's tenant, once.
func (r MembershipRepository) HeldAnywhere(ctx context.Context, accountID shared.ID) ([]identity.Role, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.RolesHeldAnywhere(ctx, account)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the roles an account holds: %w", err))
	}
	roles := make([]identity.Role, 0, len(rows))
	for _, role := range rows {
		roles = append(roles, identity.Role(role))
	}
	return roles, nil
}

// HoldsAny answers whether the account holds a role anywhere in the transaction's tenant.
func (r MembershipRepository) HoldsAny(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	held, err := queries.HoldsAnyMembership(ctx, account)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading whether the account holds a role: %w", err))
	}
	return held, nil
}

// PeopleReaching answers how many people hold a role in the transaction's tenant, or within one
// hub when hubID is not zero.
func (r MembershipRepository) PeopleReaching(ctx context.Context, hubID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	var hub pgtype.UUID
	if !hubID.IsZero() {
		if hub, err = uuidOf(hubID); err != nil {
			return 0, err
		}
	}
	people, err := queries.CountPeopleReaching(ctx, hub)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the people a place reaches: %w", err))
	}
	return int(people), nil
}
