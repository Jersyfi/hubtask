// SPDX-License-Identifier: BUSL-1.1
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
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PasswordRepository is the store side of a password's life (ADR-0068 §5): the hash with the moment
// it was written, and the few hashes before it.
//
// No method names a tenant: row level security has bound the transaction to one, and that is what
// makes another workspace's account invisible rather than forbidden (ADR-0010).
type PasswordRepository struct{}

func NewPasswordRepository() PasswordRepository { return PasswordRepository{} }

var (
	_ repository.PasswordAccounts  = PasswordRepository{}
	_ repository.PasswordHistories = PasswordRepository{}
)

// FindForPassword answers one account with its hash and the moment.
func (PasswordRepository) FindForPassword(
	ctx context.Context, accountID shared.ID,
) (repository.PasswordAccount, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.PasswordAccount{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return repository.PasswordAccount{}, err
	}

	row, err := queries.FindAccountForPassword(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return repository.PasswordAccount{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return repository.PasswordAccount{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an account for its password: %w", err))
	}

	return passwordAccountFrom(
		row.ID, row.TenantID, row.Kind, row.Email, row.DisplayName, row.Status,
		row.Locale, row.TimeZone, row.WeekStart, row.PasswordHash, row.PasswordSetAt)
}

// FindByEmailForPassword answers the account an address names.
func (PasswordRepository) FindByEmailForPassword(
	ctx context.Context, email string,
) (repository.PasswordAccount, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.PasswordAccount{}, err
	}

	row, err := queries.FindAccountByEmailForPassword(ctx, email)
	if err != nil {
		if IsNoRows(err) {
			return repository.PasswordAccount{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return repository.PasswordAccount{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an account by address for its password: %w", err))
	}

	return passwordAccountFrom(
		row.ID, row.TenantID, row.Kind, row.Email, row.DisplayName, row.Status,
		row.Locale, row.TimeZone, row.WeekStart, row.PasswordHash, row.PasswordSetAt)
}

// passwordAccountFrom rebuilds the account. The tenant is carried deliberately: a mapper that drops
// it answers a row nothing can write back, and three of them once did.
func passwordAccountFrom(
	id, tenantID pgtype.UUID, kind sqlc.AccountKind, email *string, displayName string,
	status sqlc.AccountStatus, locale, timeZone, weekStart *string,
	passwordHash *string, passwordSetAt pgtype.Timestamptz,
) (repository.PasswordAccount, error) {
	account, err := accountFrom(
		id, kind, email, displayName, status, locale, timeZone, weekStart, nil,
		pgtype.Timestamptz{})
	if err != nil {
		return repository.PasswordAccount{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return repository.PasswordAccount{}, err
	}
	account.TenantID = tenant

	held := repository.PasswordAccount{Account: account}
	if passwordHash != nil && *passwordHash != "" {
		held.PasswordHash = secret.New(*passwordHash)
	}
	if passwordSetAt.Valid {
		held.PasswordSetAt = timeFrom(passwordSetAt)
	}
	return held, nil
}

// SetPassword writes the hash and the moment together.
func (PasswordRepository) SetPassword(
	ctx context.Context, accountID shared.ID, hash string, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}

	rows, err := queries.SetAccountPassword(ctx, sqlc.SetAccountPasswordParams{
		AccountID:    id,
		PasswordHash: &hash,
		At:           pgtype.Timestamptz{Time: at.UTC(), Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing a password: %w", err))
	}
	return rows > 0, nil
}

// Recent answers the newest hashes, newest first.
func (PasswordRepository) Recent(
	ctx context.Context, accountID shared.ID, count int,
) ([]secret.Secret, error) {
	if count <= 0 {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}

	//nolint:gosec // G115: bounded by identity.CeilingHistoryCount long before it reaches here
	rows, err := queries.RecentPasswordHashes(ctx, sqlc.RecentPasswordHashesParams{
		AccountID: id, Depth: int32(count),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the password history: %w", err))
	}

	hashes := make([]secret.Secret, 0, len(rows))
	for _, hash := range rows {
		hashes = append(hashes, secret.New(hash))
	}
	return hashes, nil
}

// Append records a hash that has just stopped being current.
func (PasswordRepository) Append(
	ctx context.Context, entryID, accountID shared.ID, hash string, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(entryID)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}

	if err := queries.AppendPasswordHistory(ctx, sqlc.AppendPasswordHistoryParams{
		ID: id, AccountID: account, PasswordHash: hash,
		SetAt: pgtype.Timestamptz{Time: at.UTC(), Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording a password in the history: %w", err))
	}
	return nil
}

// Trim drops everything past the newest `keep`.
func (PasswordRepository) Trim(ctx context.Context, accountID shared.ID, keep int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if keep < 0 {
		keep = 0
	}
	if keep > identity.CeilingHistoryCount {
		keep = identity.CeilingHistoryCount
	}

	if err := queries.TrimPasswordHistory(ctx, sqlc.TrimPasswordHistoryParams{
		AccountID: account, Keep: int32(keep),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("trimming the password history: %w", err))
	}
	return nil
}
