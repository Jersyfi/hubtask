// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The provider's three stores (H-05), one type per port because two of them insert and one
// receiver cannot answer two spellings of the same verb. They are the only places that know how
// a code or a client secret becomes a hash, AccessTokenRepository's reasoning. No method takes
// a tenant - row level security bounds every statement (ADR-0010).

type OauthClientRepository struct {
	secretHasher security.OauthClientSecretHasher
}

func NewOauthClientRepository(secretHasher security.OauthClientSecretHasher) OauthClientRepository {
	return OauthClientRepository{secretHasher: secretHasher}
}

type OauthGrantRepository struct{}

func NewOauthGrantRepository() OauthGrantRepository { return OauthGrantRepository{} }

type OauthCodeRepository struct {
	codeHasher security.OauthCodeHasher
}

func NewOauthCodeRepository(codeHasher security.OauthCodeHasher) OauthCodeRepository {
	return OauthCodeRepository{codeHasher: codeHasher}
}

var (
	_ repository.OauthClients = OauthClientRepository{}
	_ repository.OauthGrants  = OauthGrantRepository{}
	_ repository.OauthCodes   = OauthCodeRepository{}
)

func (r OauthClientRepository) Insert(
	ctx context.Context, client identity.OauthClient, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(client.ID)
	if err != nil {
		return err
	}
	createdBy, err := optionalUUID(client.CreatedBy)
	if err != nil {
		return err
	}
	var secretHash []byte
	if client.Confidential {
		secretHash = r.secretHasher.Hash(presented.Secret())
	}
	if err := queries.InsertOauthClient(ctx, sqlc.InsertOauthClientParams{
		ID:           id,
		Name:         client.Name,
		Confidential: client.Confidential,
		SecretHash:   secretHash,
		RedirectUris: client.RedirectURIs,
		CreatedAt:    pgtype.Timestamptz{Time: client.CreatedAt, Valid: true},
		CreatedBy:    createdBy,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("registering the client: %w", err))
	}
	return nil
}

func (r OauthClientRepository) List(ctx context.Context) ([]identity.OauthClient, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOauthClients(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the clients: %w", err))
	}
	clients := make([]identity.OauthClient, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		clients = append(clients, identity.OauthClient{
			ID: id, Name: row.Name, Confidential: row.Confidential,
			RedirectURIs: row.RedirectUris, CreatedAt: timeFrom(row.CreatedAt),
		})
	}
	return clients, nil
}

func (r OauthClientRepository) Find(ctx context.Context, clientID shared.ID) (identity.OauthClient, error) {
	row, err := r.findRow(ctx, clientID)
	if err != nil {
		return identity.OauthClient{}, err
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.OauthClient{}, err
	}
	return identity.OauthClient{
		ID: id, Name: row.Name, Confidential: row.Confidential,
		RedirectURIs: row.RedirectUris, CreatedAt: timeFrom(row.CreatedAt),
	}, nil
}

// SecretMatches compares in constant time, in the one layer that holds the pepper. A public
// client answers false however right the guess: it has nothing to match.
func (r OauthClientRepository) SecretMatches(
	ctx context.Context, clientID shared.ID, presented secret.Secret,
) (bool, error) {
	row, err := r.findRow(ctx, clientID)
	if err != nil {
		return false, err
	}
	if len(row.SecretHash) == 0 || presented.IsEmpty() {
		return false, nil
	}
	derived := r.secretHasher.Hash(presented.Reveal())
	return subtle.ConstantTimeCompare(derived, row.SecretHash) == 1, nil
}

func (r OauthClientRepository) findRow(ctx context.Context, clientID shared.ID) (sqlc.FindOauthClientRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return sqlc.FindOauthClientRow{}, err
	}
	id, err := uuidOf(clientID)
	if err != nil {
		return sqlc.FindOauthClientRow{}, err
	}
	row, err := queries.FindOauthClient(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return sqlc.FindOauthClientRow{}, shared.ErrNotFound.WithDetail("oauth.client_not_found")
		}
		return sqlc.FindOauthClientRow{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the client: %w", err))
	}
	return row, nil
}

func (r OauthClientRepository) Delete(ctx context.Context, clientID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(clientID)
	if err != nil {
		return false, err
	}
	removed, err := queries.DeleteOauthClient(ctx, id)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the client: %w", err))
	}
	return removed > 0, nil
}

func (r OauthGrantRepository) Upsert(ctx context.Context, grant identity.OauthGrant) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := uuidOf(grant.ID)
	if err != nil {
		return "", err
	}
	accountID, err := uuidOf(grant.AccountID)
	if err != nil {
		return "", err
	}
	clientID, err := uuidOf(grant.ClientID)
	if err != nil {
		return "", err
	}
	liveID, err := queries.UpsertOauthGrant(ctx, sqlc.UpsertOauthGrantParams{
		ID:        id,
		AccountID: accountID,
		ClientID:  clientID,
		Scopes:    grant.Scopes,
		CreatedAt: pgtype.Timestamptz{Time: grant.CreatedAt, Valid: true},
	})
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the consent: %w", err))
	}
	return idFrom(liveID)
}

func (r OauthGrantRepository) Find(ctx context.Context, grantID shared.ID) (identity.OauthGrant, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.OauthGrant{}, err
	}
	id, err := uuidOf(grantID)
	if err != nil {
		return identity.OauthGrant{}, err
	}
	row, err := queries.FindOauthGrant(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.OauthGrant{}, shared.ErrNotFound.WithDetail("oauth.grant_not_found")
		}
		return identity.OauthGrant{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the grant: %w", err))
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return identity.OauthGrant{}, err
	}
	clientID, err := idFrom(row.ClientID)
	if err != nil {
		return identity.OauthGrant{}, err
	}
	return identity.OauthGrant{
		ID: grantID, AccountID: accountID, ClientID: clientID,
		Scopes: row.Scopes, CreatedAt: timeFrom(row.CreatedAt), RevokedAt: timeFrom(row.RevokedAt),
	}, nil
}

func (r OauthGrantRepository) ListForAccount(
	ctx context.Context, accountID shared.ID,
) ([]repository.GrantListing, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOauthGrants(ctx, account)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the grants: %w", err))
	}
	listings := make([]repository.GrantListing, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		clientID, err := idFrom(row.ClientID)
		if err != nil {
			return nil, err
		}
		listings = append(listings, repository.GrantListing{
			Grant: identity.OauthGrant{
				ID: id, AccountID: accountID, ClientID: clientID,
				Scopes: row.Scopes, CreatedAt: timeFrom(row.CreatedAt),
			},
			ClientName: row.ClientName,
			LastUsedAt: interfaceTime(row.LastUsedAt),
		})
	}
	return listings, nil
}

func (r OauthGrantRepository) Revoke(
	ctx context.Context, grantID, accountID shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(grantID)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.RevokeOauthGrant(ctx, sqlc.RevokeOauthGrantParams{
		RevokedAt: pgtype.Timestamptz{Time: at, Valid: true},
		ID:        id,
		AccountID: account,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking the grant: %w", err))
	}
	return changed > 0, nil
}

func (r OauthGrantRepository) RevokeSessions(
	ctx context.Context, grantID shared.ID, at time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(grantID)
	if err != nil {
		return 0, err
	}
	ended, err := queries.RevokeGrantSessions(ctx, sqlc.RevokeGrantSessionsParams{
		RevokedAt: pgtype.Timestamptz{Time: at, Valid: true},
		GrantID:   id,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("ending the grant's sessions: %w", err))
	}
	return int(ended), nil
}

func (r OauthCodeRepository) Insert(
	ctx context.Context, code identity.OauthCode, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(code.ID)
	if err != nil {
		return err
	}
	clientID, err := uuidOf(code.ClientID)
	if err != nil {
		return err
	}
	accountID, err := uuidOf(code.AccountID)
	if err != nil {
		return err
	}
	grantID, err := uuidOf(code.GrantID)
	if err != nil {
		return err
	}
	if err := queries.InsertOauthCode(ctx, sqlc.InsertOauthCodeParams{
		ID:            id,
		ClientID:      clientID,
		AccountID:     accountID,
		GrantID:       grantID,
		CodeHash:      r.codeHasher.Hash(presented.Secret()),
		CodeChallenge: code.Challenge,
		RedirectUri:   code.RedirectURI,
		CreatedAt:     pgtype.Timestamptz{Time: code.CreatedAt, Valid: true},
		ExpiresAt:     pgtype.Timestamptz{Time: code.ExpiresAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the code: %w", err))
	}
	return nil
}

func (r OauthCodeRepository) Consume(
	ctx context.Context, presented identity.Token, now time.Time,
) (identity.OauthCode, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.OauthCode{}, false, err
	}
	row, err := queries.ConsumeOauthCode(ctx, sqlc.ConsumeOauthCodeParams{
		Now:      pgtype.Timestamptz{Time: now, Valid: true},
		CodeHash: r.codeHasher.Hash(presented.Secret()),
	})
	if err != nil {
		if IsNoRows(err) {
			return identity.OauthCode{}, false, nil
		}
		return identity.OauthCode{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("consuming the code: %w", err))
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.OauthCode{}, false, err
	}
	clientID, err := idFrom(row.ClientID)
	if err != nil {
		return identity.OauthCode{}, false, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return identity.OauthCode{}, false, err
	}
	grantID, err := idFrom(row.GrantID)
	if err != nil {
		return identity.OauthCode{}, false, err
	}
	return identity.OauthCode{
		ID: id, ClientID: clientID, AccountID: accountID, GrantID: grantID,
		Challenge: row.CodeChallenge, RedirectURI: row.RedirectUri,
	}, true, nil
}

// interfaceTime reads the aggregate max() column, which sqlc types as interface{}.
func interfaceTime(value any) time.Time {
	if at, ok := value.(time.Time); ok {
		return at.UTC()
	}
	if at, ok := value.(pgtype.Timestamptz); ok && at.Valid {
		return at.Time.UTC()
	}
	return time.Time{}
}
