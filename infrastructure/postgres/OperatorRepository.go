// SPDX-License-Identifier: Apache-2.0
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

// OperatorRepository is the register of ADR-0070 §1, reached through its narrow functions.
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

// Roster answers the register with names, and nothing to a caller the register does not name.
func (OperatorRepository) Roster(ctx context.Context, caller shared.ID) ([]repository.Operator, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	callerID, err := uuidOf(caller)
	if err != nil {
		return nil, err
	}

	rows, err := queries.OperatorRoster(ctx, callerID)
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
			DisplayName: row.DisplayName, Email: row.Email, SignInName: row.SignInName,
			Kind:          identity.AccountKind(row.Kind),
			WorkspaceSlug: row.WorkspaceSlug, WorkspaceName: row.WorkspaceName,
			AddedByName: row.AddedByName,
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

// Resolve answers the account an address names inside one workspace.
func (OperatorRepository) Resolve(ctx context.Context, slug, email string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return shared.ID(""), err
	}
	found, err := queries.ResolveOperatorAccount(ctx, sqlc.ResolveOperatorAccountParams{
		Slug: slug, Email: email,
	})
	if err != nil {
		return shared.ID(""), shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("resolving an operator's account: %w", err))
	}
	if !found.Valid {
		// Nothing matched. Not an error here: the caller turns it into the one refusal, which is
		// the same one a wrong identifier produced before.
		return shared.ID(""), nil
	}
	return idFrom(found)
}

// Enlist puts an active account in, and the adder too where the register was empty.
func (OperatorRepository) Enlist(
	ctx context.Context, accountID, by shared.ID,
) (repository.EnlistOutcome, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return "", err
	}

	adder := pgtype.UUID{}
	if !by.IsZero() {
		if adder, err = uuidOf(by); err != nil {
			return "", err
		}
	}

	outcome, err := queries.EnlistOperator(ctx, sqlc.EnlistOperatorParams{
		AccountID: account, AddedBy: adder,
	})
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("adding an operator: %w", err))
	}
	return repository.EnlistOutcome(outcome), nil
}

// Dismiss takes one out, unless it is the last.
func (OperatorRepository) Dismiss(
	ctx context.Context, accountID shared.ID,
) (repository.DismissOutcome, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return "", err
	}

	outcome, err := queries.DismissOperator(ctx, account)
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing an operator: %w", err))
	}
	return repository.DismissOutcome(outcome), nil
}

// ResolveSingle answers the account an address names in a single-mode installation's one
// workspace, and zero where there is not exactly one workspace or the address matches nothing
// (migration 0128).
func (OperatorRepository) ResolveSingle(ctx context.Context, email string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return shared.ID(""), err
	}
	found, err := queries.ResolveSingleOperatorAccount(ctx, email)
	if err != nil {
		return shared.ID(""), shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("resolving an operator's account in the one workspace: %w", err))
	}
	if !found.Valid {
		return shared.ID(""), nil
	}
	return idFrom(found)
}

// State answers how many workspaces there are and how many operators, counted as `is_operator`
// counts (migration 0128).
func (OperatorRepository) State(ctx context.Context) (workspaces, operators int64, err error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, 0, err
	}
	row, err := queries.OperatorRegisterState(ctx)
	if err != nil {
		return 0, 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the operator register's state: %w", err))
	}
	return row.Workspaces, row.Operators, nil
}
