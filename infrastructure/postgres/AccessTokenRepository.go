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
			ID:          accountID,
			Kind:        identity.AccountKind(row.AccountKind),
			DisplayName: row.AccountDisplayName,
			Status:      identity.AccountStatus(row.AccountStatus),
			Locale:      stringFrom(row.AccountLocale),
			TimeZone:    stringFrom(row.AccountTimeZone),
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

// Insert writes the minted row. The hash is computed here and nowhere else - the application
// layer handed over the presented token whole, precisely so that it never held the stored value.
func (r AccessTokenRepository) Insert(
	ctx context.Context, token identity.AccessToken, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(token.ID)
	if err != nil {
		return err
	}
	accountID, err := uuidOf(token.AccountID)
	if err != nil {
		return err
	}

	if err := queries.InsertAccessToken(ctx, sqlc.InsertAccessTokenParams{
		ID:        id,
		AccountID: accountID,
		Name:      token.Name,
		TokenHash: r.hasher.Hash(presented.Secret()),
		// The prefix is stored so that an operator can tell one kind of credential from another
		// in a row they are looking at. It is public by design (security.md §5).
		TokenPrefix: identity.TokenPrefix,
		Scopes:      token.Scopes,
		ExpiresAt:   pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: token.CreatedAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the access token: %w", err))
	}
	return nil
}

// Find reads one token. A token of another tenant is not found rather than forbidden, because row
// level security makes it invisible before any comparison could be made (multi-tenancy.md §2).
func (r AccessTokenRepository) Find(
	ctx context.Context, tokenID shared.ID,
) (identity.AccessToken, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.AccessToken{}, err
	}

	id, err := uuidOf(tokenID)
	if err != nil {
		return identity.AccessToken{}, err
	}

	row, err := queries.FindAccessToken(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.AccessToken{}, shared.ErrNotFound.WithDetail("access.token_unknown")
		}
		return identity.AccessToken{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the access token: %w", err))
	}
	return tokenFrom(sqlc.AccessTokensForAccountRow(row))
}

// ListForAccount answers one account's own credentials, newest first.
func (r AccessTokenRepository) ListForAccount(
	ctx context.Context, accountID shared.ID,
) ([]identity.AccessToken, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.AccessTokensForAccount(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the access tokens: %w", err))
	}

	tokens := make([]identity.AccessToken, 0, len(rows))
	for _, row := range rows {
		token, err := tokenFrom(row)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// Revoke stamps the row and reports whether it changed anything. Nothing changed means it was
// already revoked, which is not an error - and the moment it was first pulled is kept, because
// that is the one an auditor asks about.
func (r AccessTokenRepository) Revoke(
	ctx context.Context, tokenID shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	id, err := uuidOf(tokenID)
	if err != nil {
		return false, err
	}

	changed, err := queries.RevokeAccessToken(ctx, sqlc.RevokeAccessTokenParams{
		ID:        id,
		RevokedAt: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking the access token: %w", err))
	}
	return changed > 0, nil
}

// tokenFrom maps a stored row. The two listings and the single read select the same columns in
// the same order, so one mapping serves all three - a second copy is how two of them come to
// disagree about what a null expiry means.
func tokenFrom(row sqlc.AccessTokensForAccountRow) (identity.AccessToken, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.AccessToken{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return identity.AccessToken{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return identity.AccessToken{}, err
	}

	return identity.AccessToken{
		ID: id, TenantID: tenantID, AccountID: accountID,
		Name:       row.Name,
		Scopes:     row.Scopes,
		ExpiresAt:  timeFrom(row.ExpiresAt),
		RevokedAt:  timeFrom(row.RevokedAt),
		LastUsedAt: timeFrom(row.LastUsedAt),
		CreatedAt:  timeFrom(row.CreatedAt),
	}, nil
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
