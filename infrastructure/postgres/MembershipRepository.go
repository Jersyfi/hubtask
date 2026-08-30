// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

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
	return memberships, nil
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

// administratorRoles are the roles the retention warning treats as administrators (R-1, G-12).
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
