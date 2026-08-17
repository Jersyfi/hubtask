// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// AccessTokenRepository looks up personal access tokens.
//
// It is the only place that knows how a token becomes a hash, because the pepper is a secret of
// this layer (security.md §8) - which is also why the port takes the presented token whole rather
// than a hash the application layer would have had to compute.
type AccessTokenRepository struct {
	hasher security.TokenHasher
}

func NewAccessTokenRepository(hasher security.TokenHasher) AccessTokenRepository {
	return AccessTokenRepository{hasher: hasher}
}

var _ repository.AccessTokens = AccessTokenRepository{}

// FindByToken reads the credential inside the caller's transaction, which is what bounds it to a
// tenant: the query itself carries no tenant condition, because row level security applies one
// that no query can forget (ADR-0010). A token quoting the wrong tenant therefore finds nothing.
func (r AccessTokenRepository) FindByToken(
	ctx context.Context,
	token identity.Token,
) (repository.Credential, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Credential{}, err
	}

	row, err := queries.FindAccessTokenByHash(ctx, r.hasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			return repository.Credential{}, shared.ErrNotFound.WithDetail("access.token_unknown")
		}
		return repository.Credential{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the access token: %w", err))
	}

	tokenID, err := idFrom(row.ID)
	if err != nil {
		return repository.Credential{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return repository.Credential{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return repository.Credential{}, err
	}

	return repository.Credential{
		Token: identity.AccessToken{
			ID:         tokenID,
			TenantID:   tenantID,
			AccountID:  accountID,
			Scopes:     row.Scopes,
			ExpiresAt:  timeFrom(row.ExpiresAt),
			RevokedAt:  timeFrom(row.RevokedAt),
			LastUsedAt: timeFrom(row.LastUsedAt),
		},
		Account: identity.Account{
			ID:       accountID,
			Kind:     identity.AccountKind(row.AccountKind),
			Status:   identity.AccountStatus(row.AccountStatus),
			Locale:   stringFrom(row.AccountLocale),
			TimeZone: stringFrom(row.AccountTimeZone),
		},
		TenantLocale:   row.DefaultLocale,
		TenantTimeZone: row.DefaultTimeZone,
	}, nil
}

// TouchLastUsed writes the bookkeeping column. The row is reachable only within the tenant's own
// transaction, so no tenant condition is needed here either.
func (r AccessTokenRepository) TouchLastUsed(ctx context.Context, tokenID shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(tokenID)
	if err != nil {
		return err
	}
	if err := queries.TouchAccessToken(ctx, sqlc.TouchAccessTokenParams{
		ID:         id,
		LastUsedAt: pgtype.Timestamptz{Time: at, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the last use: %w", err))
	}
	return nil
}

// queriesFrom binds the generated queries to the transaction in the context. There is no path
// that binds them to the pool: a query outside a unit of work would run without a tenant context
// (CLAUDE.md rule 3).
func queriesFrom(ctx context.Context) (*sqlc.Queries, error) {
	tx, err := FromContext(ctx)
	if err != nil {
		return nil, err
	}
	return sqlc.New(tx), nil
}

// idFrom converts a stored UUID to the domain identifier. A row that cannot produce one is a
// defect rather than a bad request - the column is typed uuid, so the only way here is a driver
// disagreeing with itself.
func idFrom(value pgtype.UUID) (shared.ID, error) {
	if !value.Valid {
		return "", shared.ErrInternal.WithDetail("postgres.uuid_null")
	}
	text, err := value.MarshalJSON()
	if err != nil || len(text) < 2 {
		return "", shared.ErrInternal.
			WithDetail("postgres.uuid_unreadable").
			WithCause(errors.Join(err, fmt.Errorf("unreadable uuid")))
	}
	// MarshalJSON quotes the canonical form; the quotes are the two bytes trimmed here.
	return shared.ParseID(string(text[1 : len(text)-1]))
}

func uuidOf(id shared.ID) (pgtype.UUID, error) {
	var out pgtype.UUID
	if err := out.Scan(id.String()); err != nil {
		return out, shared.ErrInternal.
			WithDetail("postgres.uuid_unreadable").
			WithCause(fmt.Errorf("converting %q: %w", id, err))
	}
	return out, nil
}

func timeFrom(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func stringFrom(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
