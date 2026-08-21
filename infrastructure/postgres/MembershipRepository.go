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
