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
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The relying party's two stores (H-04). No method takes a tenant: row level security bounds
// every statement, and the workspace is the provider row's key (ADR-0010).

type IdentityProviderRepository struct{}

func NewIdentityProviderRepository() IdentityProviderRepository {
	return IdentityProviderRepository{}
}

// OidcFlowRepository is the only place that knows how a presented state becomes a hash,
// OauthCodeRepository's reasoning.
type OidcFlowRepository struct {
	stateHasher security.OidcFlowHasher
}

func NewOidcFlowRepository(stateHasher security.OidcFlowHasher) OidcFlowRepository {
	return OidcFlowRepository{stateHasher: stateHasher}
}

var (
	_ repository.IdentityProviders = IdentityProviderRepository{}
	_ repository.OidcFlows         = OidcFlowRepository{}
)

func (IdentityProviderRepository) Upsert(
	ctx context.Context, configured identity.IdentityProvider, sealed crypto.Sealed, now time.Time,
) (identity.IdentityProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	row, err := queries.UpsertIdentityProvider(ctx, sqlc.UpsertIdentityProviderParams{
		Issuer:              configured.Issuer,
		ClientID:            configured.ClientID,
		ClientSecretEnc:     sealed.Ciphertext,
		ClientSecretKeyID:   sealed.KeyID,
		AllowedEmailDomains: configured.AllowedEmailDomains,
		Enabled:             configured.Enabled,
		Now:                 pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return identity.IdentityProvider{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the identity provider: %w", err))
	}
	return identity.IdentityProvider{
		TenantID:            configured.TenantID,
		Issuer:              row.Issuer,
		ClientID:            row.ClientID,
		AllowedEmailDomains: row.AllowedEmailDomains,
		Enabled:             row.Enabled,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
		Version:             int(row.Version),
	}, nil
}

func (IdentityProviderRepository) Find(ctx context.Context) (identity.IdentityProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	row, err := queries.FindIdentityProvider(ctx)
	if err != nil {
		if IsNoRows(err) {
			return identity.IdentityProvider{}, shared.ErrNotFound.
				WithDetail("identity_provider.not_configured")
		}
		return identity.IdentityProvider{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the identity provider: %w", err))
	}
	return identity.IdentityProvider{
		Issuer:              row.Issuer,
		ClientID:            row.ClientID,
		AllowedEmailDomains: row.AllowedEmailDomains,
		Enabled:             row.Enabled,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
		Version:             int(row.Version),
	}, nil
}

func (IdentityProviderRepository) FindWithSecret(
	ctx context.Context,
) (identity.IdentityProvider, crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, crypto.Sealed{}, err
	}
	row, err := queries.FindIdentityProviderSecret(ctx)
	if err != nil {
		if IsNoRows(err) {
			return identity.IdentityProvider{}, crypto.Sealed{}, shared.ErrNotFound.
				WithDetail("identity_provider.not_configured")
		}
		return identity.IdentityProvider{}, crypto.Sealed{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the identity provider: %w", err))
	}
	return identity.IdentityProvider{
			Issuer:              row.Issuer,
			ClientID:            row.ClientID,
			AllowedEmailDomains: row.AllowedEmailDomains,
			Enabled:             row.Enabled,
		}, crypto.Sealed{
			KeyID: row.ClientSecretKeyID, Ciphertext: row.ClientSecretEnc,
		}, nil
}

func (IdentityProviderRepository) Delete(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	removed, err := queries.DeleteIdentityProvider(ctx)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the identity provider: %w", err))
	}
	return removed > 0, nil
}

func (r OidcFlowRepository) Insert(
	ctx context.Context, flow identity.OidcFlow, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(flow.ID)
	if err != nil {
		return err
	}
	if err := queries.InsertOidcFlow(ctx, sqlc.InsertOidcFlowParams{
		ID:           id,
		StateHash:    r.stateHasher.Hash(presented.Secret()),
		CodeVerifier: flow.Verifier,
		Nonce:        flow.Nonce,
		CreatedAt:    pgtype.Timestamptz{Time: flow.CreatedAt, Valid: true},
		ExpiresAt:    pgtype.Timestamptz{Time: flow.ExpiresAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the sign-in flow: %w", err))
	}
	return nil
}

func (r OidcFlowRepository) Consume(
	ctx context.Context, presented identity.Token, now time.Time,
) (identity.OidcFlow, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	row, err := queries.ConsumeOidcFlow(ctx, sqlc.ConsumeOidcFlowParams{
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		StateHash: r.stateHasher.Hash(presented.Secret()),
	})
	if err != nil {
		if IsNoRows(err) {
			// Unknown, expired or already spent - one answer, because which of the three it was
			// is not for a presenter to learn.
			return identity.OidcFlow{}, false, nil
		}
		return identity.OidcFlow{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("consuming the sign-in flow: %w", err))
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	return identity.OidcFlow{
		ID: id, TenantID: presented.TenantID(),
		Nonce: row.Nonce, Verifier: row.CodeVerifier,
	}, true, nil
}
