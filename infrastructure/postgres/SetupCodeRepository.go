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
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// SetupCodeRepository holds the first start's codes (migration 0128) through their three narrow
// functions; the table has no policy and no grant to the application role. A code is stored only as
// its hash under the setup code's own purpose label.
type SetupCodeRepository struct {
	hasher security.InboundTokenHasher
}

func NewSetupCodeRepository(hasher security.InboundTokenHasher) SetupCodeRepository {
	return SetupCodeRepository{hasher: hasher}
}

var _ repository.SetupCodes = SetupCodeRepository{}

// Unset reports whether the installation has no workspace and nobody in its register.
func (SetupCodeRepository) Unset(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	unset, err := queries.InstallationUnset(ctx)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading whether the installation is set up: %w", err))
	}
	return unset, nil
}

// Issue stores the code's hash, on an empty installation only.
func (r SetupCodeRepository) Issue(
	ctx context.Context, id shared.ID, code identity.SetupCode, issuedAt, expiresAt time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	row, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	issued, err := queries.IssueSetupCode(ctx, sqlc.IssueSetupCodeParams{
		ID: row, CodeHash: r.hasher.Hash(code.Canonical()),
		IssuedAt:  pgtype.Timestamptz{Time: issuedAt, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("issuing a setup code: %w", err))
	}
	return issued, nil
}

// Claim spends a code inside the caller's transaction.
func (r SetupCodeRepository) Claim(
	ctx context.Context, code identity.SetupCode, now time.Time,
) (repository.SetupClaim, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	answer, err := queries.ClaimSetupCode(ctx, sqlc.ClaimSetupCodeParams{
		CodeHash: r.hasher.Hash(code.Canonical()),
		Now:      pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming a setup code: %w", err))
	}
	return repository.SetupClaim(answer), nil
}
