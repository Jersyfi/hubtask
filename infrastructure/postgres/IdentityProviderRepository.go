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

// ExternalAccountRepository is the seam `account.external_subject` cut in phase 0 (H-04).
//
// Its own type rather than a method on AccountRepository, for the reason every slice here has
// one: the sign-in flow needs two statements about a column nothing else touches, and a
// repository that could write the subject from anywhere is one that eventually does.
type ExternalAccountRepository struct{}

func NewExternalAccountRepository() ExternalAccountRepository { return ExternalAccountRepository{} }

var _ repository.ExternalAccounts = ExternalAccountRepository{}

func (ExternalAccountRepository) FindBySubject(
	ctx context.Context, subject string,
) (identity.Account, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Account{}, err
	}
	row, err := queries.FindAccountByExternalSubject(ctx, &subject)
	if err != nil {
		if IsNoRows(err) {
			return identity.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return identity.Account{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			// The subject stays out of the message: it identifies a person at their provider.
			WithCause(fmt.Errorf("reading an account by its provider subject: %w", err))
	}
	return accountFrom(row.ID, row.Kind, row.Email, row.DisplayName, row.Status,
		row.Locale, row.TimeZone, row.WeekStart)
}

func (ExternalAccountRepository) LinkSubject(
	ctx context.Context, accountID shared.ID, subject string, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	linked, err := queries.LinkAccountExternalSubject(ctx, sqlc.LinkAccountExternalSubjectParams{
		ID:              id,
		ExternalSubject: &subject,
		Now:             pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("linking an account to its provider subject: %w", err))
	}
	return linked > 0, nil
}

var _ repository.IdentityProviderSealing = IdentityProviderRepository{}

func (IdentityProviderRepository) RewrapSecret(
	ctx context.Context, sealed crypto.Sealed, expectedKeyID string,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	changed, err := queries.RewrapIdentityProviderSecret(ctx, sqlc.RewrapIdentityProviderSecretParams{
		ClientSecretEnc: sealed.Ciphertext, ClientSecretKeyID: sealed.KeyID,
		ExpectedKeyID: expectedKeyID,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("re-sealing the identity provider's secret: %w", err))
	}
	return changed > 0, nil
}
