// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// OperatorRepository is the register of ADR-0070 §1, reached through its four narrow functions.
//
// Not through the table: `operator` carries no row-level policy and no grant to this role, for the
// reason migration 0101 gives. The functions are `resolve_tenant`'s discipline applied to a table -
// a boolean, a listing the control plane alone reaches, and two writes that keep the register's one
// invariant in the statement rather than in a read somebody could race.
type OperatorRepository struct{}

func NewOperatorRepository() OperatorRepository { return OperatorRepository{} }

var _ repository.Operators = OperatorRepository{}

// Holds is the check, asked when the scope is minted and when it is exercised.
func (OperatorRepository) Holds(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}

	held, err := queries.IsOperator(ctx, account)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the operator register: %w", err))
	}
	return held, nil
}

// List answers the whole register.
func (OperatorRepository) List(ctx context.Context) ([]repository.Operator, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.OperatorRegister(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the operator register: %w", err))
	}

	operators := make([]repository.Operator, 0, len(rows))
	for _, row := range rows {
		tenantID, err := idFrom(row.TenantID)
		if err != nil {
			return nil, err
		}
		accountID, err := idFrom(row.AccountID)
		if err != nil {
			return nil, err
		}
		operator := repository.Operator{
			TenantID: tenantID, AccountID: accountID, AddedAt: timeFrom(row.AddedAt),
		}
		if row.AddedBy.Valid {
			if operator.AddedBy, err = idFrom(row.AddedBy); err != nil {
				return nil, err
			}
		}
		operators = append(operators, operator)
	}
	return operators, nil
}

// Add puts an account in.
func (OperatorRepository) Add(ctx context.Context, accountID, by shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}

	adder := pgtype.UUID{}
	if !by.IsZero() {
		if adder, err = uuidOf(by); err != nil {
			return false, err
		}
	}

	added, err := queries.AddOperator(ctx, sqlc.AddOperatorParams{
		AccountID: account, AddedBy: adder,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("adding an operator: %w", err))
	}
	return added, nil
}

// Remove takes one out, unless it is the last.
func (OperatorRepository) Remove(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}

	removed, err := queries.DropOperator(ctx, account)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing an operator: %w", err))
	}
	return removed, nil
}
