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

// The relying party's stores (H-04, SI-10). No method takes a tenant: row level security bounds
// every statement, and which level a row belongs to is the scope's answer rather than a
// parameter's (ADR-0010, migration 0103).

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

func (IdentityProviderRepository) List(ctx context.Context) ([]identity.IdentityProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListIdentityProviders(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the identity providers: %w", err))
	}
	providers := make([]identity.IdentityProvider, 0, len(rows))
	for _, row := range rows {
		configured, err := providerFrom(row.ID, row.TenantID, row.Issuer, row.ClientID,
			row.DisplayName, row.Kind, row.Provisioning, row.Position,
			row.AllowedEmailDomains, row.AllowedDirectories, row.Enabled, row.CreatedAt, row.UpdatedAt, row.Version)
		configured = withOffer(configured, row.WithdrawAt, row.OfferedWorkspaces)
		if err != nil {
			return nil, err
		}
		providers = append(providers, configured)
	}
	return providers, nil
}

func (IdentityProviderRepository) Count(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	counted, err := queries.CountIdentityProviders(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the identity providers: %w", err))
	}
	return int(counted), nil
}

func (IdentityProviderRepository) Find(
	ctx context.Context, id shared.ID,
) (identity.IdentityProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	row, err := queries.FindIdentityProviderByID(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return identity.IdentityProvider{}, shared.ErrNotFound.
				WithDetail("identity_provider.not_found")
		}
		return identity.IdentityProvider{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the identity provider: %w", err))
	}
	found, err := providerFrom(row.ID, row.TenantID, row.Issuer, row.ClientID,
		row.DisplayName, row.Kind, row.Provisioning, row.Position,
		row.AllowedEmailDomains, row.AllowedDirectories, row.Enabled, row.CreatedAt, row.UpdatedAt, row.Version)
	return withOffer(found, row.WithdrawAt, row.OfferedWorkspaces), err
}

func (IdentityProviderRepository) FindWithSecret(
	ctx context.Context, id shared.ID,
) (identity.IdentityProvider, crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, crypto.Sealed{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return identity.IdentityProvider{}, crypto.Sealed{}, err
	}
	row, err := queries.FindIdentityProviderSecret(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return identity.IdentityProvider{}, crypto.Sealed{}, shared.ErrNotFound.
				WithDetail("identity_provider.not_found")
		}
		return identity.IdentityProvider{}, crypto.Sealed{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the identity provider: %w", err))
	}
	configured, err := providerFrom(row.ID, row.TenantID, row.Issuer, row.ClientID,
		row.DisplayName, row.Kind, row.Provisioning, row.Position,
		row.AllowedEmailDomains, row.AllowedDirectories, row.Enabled,
		pgtype.Timestamptz{}, pgtype.Timestamptz{}, 0)
	configured.WithdrawAt = timeFrom(row.WithdrawAt)
	if err != nil {
		return identity.IdentityProvider{}, crypto.Sealed{}, err
	}
	return configured, crypto.Sealed{
		KeyID: row.ClientSecretKeyID, Ciphertext: row.ClientSecretEnc,
	}, nil
}

func (IdentityProviderRepository) Insert(
	ctx context.Context, configured identity.IdentityProvider, sealed crypto.Sealed,
) (identity.IdentityProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	key, err := uuidOf(configured.ID)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	row, err := queries.InsertIdentityProvider(ctx, sqlc.InsertIdentityProviderParams{
		ID:                key,
		Issuer:            configured.Issuer,
		ClientID:          configured.ClientID,
		ClientSecretEnc:   sealed.Ciphertext,
		ClientSecretKeyID: sealed.KeyID,
		DisplayName:       configured.DisplayName,
		Kind:              string(configured.Kind),
		Provisioning:      string(configured.Provisioning),
		// Bounded by the domain at MaxProviderPosition, two decimal digits.
		Position:            int32(configured.Position), //nolint:gosec // G115: 0..99 by construction
		AllowedEmailDomains: configured.AllowedEmailDomains,
		AllowedDirectories:  configured.AllowedDirectories,
		Enabled:             configured.Enabled,
		Now:                 pgtype.Timestamptz{Time: configured.CreatedAt, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return identity.IdentityProvider{}, shared.ErrConflict.
				WithDetail("identity_provider.issuer_taken").
				WithParams(map[string]string{"issuer": configured.Issuer})
		}
		return identity.IdentityProvider{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the identity provider: %w", err))
	}
	found, err := providerFrom(row.ID, row.TenantID, row.Issuer, row.ClientID,
		row.DisplayName, row.Kind, row.Provisioning, row.Position,
		row.AllowedEmailDomains, row.AllowedDirectories, row.Enabled, row.CreatedAt, row.UpdatedAt, row.Version)
	return withOffer(found, row.WithdrawAt, row.OfferedWorkspaces), err
}

func (r IdentityProviderRepository) Update(
	ctx context.Context, configured identity.IdentityProvider,
	sealed *crypto.Sealed, now time.Time,
) (identity.IdentityProvider, bool, error) {
	enabled := configured.Enabled
	return r.write(ctx, configured, sealed, &enabled, now)
}

// Reconfigure writes everything but the switch, which the statement's COALESCE keeps as the row
// holds it (ADR-0076 §5).
func (r IdentityProviderRepository) Reconfigure(
	ctx context.Context, configured identity.IdentityProvider,
	sealed *crypto.Sealed, now time.Time,
) (identity.IdentityProvider, bool, error) {
	return r.write(ctx, configured, sealed, nil, now)
}

// write is the one statement behind both: nil `enabled` is "leave the switch alone".
func (IdentityProviderRepository) write(
	ctx context.Context, configured identity.IdentityProvider,
	sealed *crypto.Sealed, enabled *bool, now time.Time,
) (identity.IdentityProvider, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, false, err
	}
	key, err := uuidOf(configured.ID)
	if err != nil {
		return identity.IdentityProvider{}, false, err
	}
	params := sqlc.UpdateIdentityProviderParams{
		ID:           key,
		Issuer:       configured.Issuer,
		ClientID:     configured.ClientID,
		DisplayName:  configured.DisplayName,
		Kind:         string(configured.Kind),
		Provisioning: string(configured.Provisioning),
		// Bounded by the domain at MaxProviderPosition, two decimal digits.
		Position:            int32(configured.Position), //nolint:gosec // G115: 0..99 by construction
		AllowedEmailDomains: configured.AllowedEmailDomains,
		AllowedDirectories:  configured.AllowedDirectories,
		Enabled:             enabled,
		Now:                 pgtype.Timestamptz{Time: now, Valid: true},
	}
	// Nil is "keep what is sealed", which the statement's COALESCE reads from these two being
	// absent. Both or neither: an envelope and the key it was wrapped under are one value.
	if sealed != nil {
		params.ClientSecretEnc = sealed.Ciphertext
		params.ClientSecretKeyID = &sealed.KeyID
	}
	row, err := queries.UpdateIdentityProvider(ctx, params)
	if err != nil {
		if IsNoRows(err) {
			// No such row *here*: gone, or the installation's own reached for by a workspace,
			// which the write policy refuses rather than this method.
			return identity.IdentityProvider{}, false, nil
		}
		if isUniqueViolation(err) {
			return identity.IdentityProvider{}, false, shared.ErrConflict.
				WithDetail("identity_provider.issuer_taken").
				WithParams(map[string]string{"issuer": configured.Issuer})
		}
		return identity.IdentityProvider{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the identity provider: %w", err))
	}
	stored, err := providerFrom(row.ID, row.TenantID, row.Issuer, row.ClientID,
		row.DisplayName, row.Kind, row.Provisioning, row.Position,
		row.AllowedEmailDomains, row.AllowedDirectories, row.Enabled, row.CreatedAt, row.UpdatedAt, row.Version)
	stored = withOffer(stored, row.WithdrawAt, row.OfferedWorkspaces)
	return stored, err == nil, err
}

// withOffer adds the two columns of ADR-0076 an installation row carries.
func withOffer(
	configured identity.IdentityProvider, withdrawAt pgtype.Timestamptz, offered int32,
) identity.IdentityProvider {
	configured.WithdrawAt = timeFrom(withdrawAt)
	configured.OfferedWorkspaces = int(offered)
	return configured
}

// SetWithdrawal sets or clears when an installation's offer ends (ADR-0076 §2).
func (IdentityProviderRepository) SetWithdrawal(
	ctx context.Context, providerID shared.ID, at, now time.Time,
) (identity.IdentityProvider, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.IdentityProvider{}, false, err
	}
	id, err := uuidOf(providerID)
	if err != nil {
		return identity.IdentityProvider{}, false, err
	}
	withdrawAt := pgtype.Timestamptz{}
	if !at.IsZero() {
		withdrawAt = pgtype.Timestamptz{Time: at, Valid: true}
	}
	row, err := queries.SetProviderWithdrawal(ctx, sqlc.SetProviderWithdrawalParams{
		WithdrawAt: withdrawAt, Now: pgtype.Timestamptz{Time: now, Valid: true}, ID: id,
	})
	if err != nil {
		if IsNoRows(err) {
			return identity.IdentityProvider{}, false, nil
		}
		return identity.IdentityProvider{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("setting the withdrawal: %w", err))
	}
	stored, err := providerFrom(row.ID, row.TenantID, row.Issuer, row.ClientID,
		row.DisplayName, row.Kind, row.Provisioning, row.Position,
		row.AllowedEmailDomains, row.AllowedDirectories, row.Enabled, row.CreatedAt, row.UpdatedAt, row.Version)
	if err != nil {
		return identity.IdentityProvider{}, false, err
	}
	return withOffer(stored, row.WithdrawAt, row.OfferedWorkspaces), true, nil
}

func (IdentityProviderRepository) Delete(ctx context.Context, id shared.ID, now time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	removed, err := queries.DeleteIdentityProvider(ctx, sqlc.DeleteIdentityProviderParams{
		ID: key, Now: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the identity provider: %w", err))
	}
	return removed > 0, nil
}

// providerFrom is the one mapper, and the tenant is in it deliberately: three mappers in this
// package once dropped `tenant_id` and every retry, replay and correction answered 500 for it. A
// NULL tenant is not an omission here - it is the installation's own row, which is what
// `Installation()` reads.
func providerFrom(
	id, tenantID pgtype.UUID, issuer, clientID, displayName, kind, provisioning string,
	position int32, domains, directories []string, enabled bool,
	createdAt, updatedAt pgtype.Timestamptz, version int32,
) (identity.IdentityProvider, error) {
	key, err := idFrom(id)
	if err != nil {
		return identity.IdentityProvider{}, err
	}
	var tenant shared.ID
	if tenantID.Valid {
		tenant, err = idFrom(tenantID)
		if err != nil {
			return identity.IdentityProvider{}, err
		}
	}
	return identity.IdentityProvider{
		ID:                  key,
		TenantID:            tenant,
		Issuer:              issuer,
		ClientID:            clientID,
		DisplayName:         displayName,
		Kind:                identity.ProviderKind(kind),
		Provisioning:        identity.Provisioning(provisioning),
		Position:            int(position),
		AllowedEmailDomains: domains,
		AllowedDirectories:  directories,
		Enabled:             enabled,
		CreatedAt:           createdAt.Time,
		UpdatedAt:           updatedAt.Time,
		Version:             int(version),
	}, nil
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
	provider, err := uuidOf(flow.ProviderID)
	if err != nil {
		return err
	}
	session, err := optionalUUID(flow.SessionID)
	if err != nil {
		return err
	}
	invited, err := optionalUUID(flow.InvitedAccountID)
	if err != nil {
		return err
	}
	pending, err := optionalUUID(flow.PendingID)
	if err != nil {
		return err
	}
	if err := queries.InsertOidcFlow(ctx, sqlc.InsertOidcFlowParams{
		SessionID:        session,
		InvitedAccountID: invited,
		PendingID:        pending,
		ID:               id,
		ProviderID:       provider,
		StateHash:        r.stateHasher.Hash(presented.Secret()),
		CodeVerifier:     flow.Verifier,
		Nonce:            flow.Nonce,
		CreatedAt:        pgtype.Timestamptz{Time: flow.CreatedAt, Valid: true},
		ExpiresAt:        pgtype.Timestamptz{Time: flow.ExpiresAt, Valid: true},
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
	// A flow the previous binary opened carries no provider, which the caller reads as "the one
	// this workspace had". Zero rather than an error: the row is valid, it is just older than the
	// column.
	providerID, err := optionalID(row.ProviderID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	// A flow opened by the previous binary, or by any sign-in that did not start from an
	// invitation, carries none.
	invitedAccountID, err := optionalID(row.InvitedAccountID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	// The same for the CONNECT link a sign-in started from (ADR-0078 §1).
	pendingID, err := optionalID(row.PendingID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	return identity.OidcFlow{
		ID: id, TenantID: presented.TenantID(), ProviderID: providerID,
		Nonce: row.Nonce, Verifier: row.CodeVerifier, InvitedAccountID: invitedAccountID,
		// When the flow left for the provider: a connection by mail counts only a sign-in made
		// after it (ADR-0078 §1).
		PendingID: pendingID, CreatedAt: timeFrom(row.CreatedAt),
	}, true, nil
}

// ConsumeForStepUp burns a step-up's flow, and only one bound to this session (ADR-0075 §2).
func (r OidcFlowRepository) ConsumeForStepUp(
	ctx context.Context, presented identity.Token, sessionID shared.ID, now time.Time,
) (identity.OidcFlow, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	session, err := uuidOf(sessionID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	row, err := queries.ConsumeStepUpOidcFlow(ctx, sqlc.ConsumeStepUpOidcFlowParams{
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		StateHash: r.stateHasher.Hash(presented.Secret()),
		SessionID: session,
	})
	if err != nil {
		if IsNoRows(err) {
			// Unknown, expired, spent, another session's or a sign-in's - one answer.
			return identity.OidcFlow{}, false, nil
		}
		return identity.OidcFlow{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("consuming the step-up flow: %w", err))
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	providerID, err := optionalID(row.ProviderID)
	if err != nil {
		return identity.OidcFlow{}, false, err
	}
	return identity.OidcFlow{
		ID: id, TenantID: presented.TenantID(), ProviderID: providerID, SessionID: sessionID,
		Nonce: row.Nonce, Verifier: row.CodeVerifier,
	}, true, nil
}

// ExternalAccountRepository is the link between a provider's subject and an account (H-04, SI-10).
//
// `account_identity` since the providers became plural: `account.external_subject` held one subject
// per account and could not say which provider vouched for it. The column is left where it is - a
// rolling update still reads it - and nothing here writes it any more, which is why an account's
// links live in the table and the column is a contract step for a later migration.
//
// Its own type rather than a method on AccountRepository, for the reason every slice here has
// one: the sign-in flow needs two statements about a table nothing else touches, and a
// repository that could write a link from anywhere is one that eventually does.
type ExternalAccountRepository struct{}

func NewExternalAccountRepository() ExternalAccountRepository { return ExternalAccountRepository{} }

var _ repository.ExternalAccounts = ExternalAccountRepository{}

func (ExternalAccountRepository) FindBySubject(
	ctx context.Context, providerID shared.ID, subject string,
) (identity.Account, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Account{}, err
	}
	provider, err := uuidOf(providerID)
	if err != nil {
		return identity.Account{}, err
	}
	row, err := queries.FindAccountByProviderSubject(ctx, sqlc.FindAccountByProviderSubjectParams{
		ProviderID: provider, Subject: subject,
	})
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
		row.Locale, row.TimeZone, row.WeekStart, row.Celebrations, row.OnboardingCompletedAt)
}

func (ExternalAccountRepository) LinkSubject(
	ctx context.Context, providerID, accountID shared.ID, subject string, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	provider, err := uuidOf(providerID)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	linked, err := queries.LinkAccountIdentity(ctx, sqlc.LinkAccountIdentityParams{
		ProviderID: provider,
		Subject:    subject,
		Now:        pgtype.Timestamptz{Time: now, Valid: true},
		AccountID:  account,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("linking an account to its provider subject: %w", err))
	}
	return linked > 0, nil
}

// ProvidersOf answers the providers the account is connected to (ADR-0075 §2).
func (ExternalAccountRepository) ProvidersOf(ctx context.Context, accountID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.AccountIdentityProviders(ctx, account)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an account's providers: %w", err))
	}
	providers := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		providers = append(providers, id)
	}
	return providers, nil
}

// HasIdentity answers whether the account already signs in through some provider - a credential of
// its own, which ADR-0071's addendum does not let another provider's word override.
func (ExternalAccountRepository) HasIdentity(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	held, err := queries.AccountHasIdentity(ctx, account)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading whether an account has a provider identity: %w", err))
	}
	return held, nil
}

// UnlinkAll drops every provider identity of the account (ADR-0078 §1).
func (ExternalAccountRepository) UnlinkAll(ctx context.Context, accountID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	removed, err := queries.UnlinkAccountIdentities(ctx, account)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing an account's provider identities: %w", err))
	}
	return int(removed), nil
}

// CountWithoutIdentityAt counts the workspace's active people no provider in the list signs in
// (ADR-0078 §1). The providers travel as one array parameter (rule 9).
func (ExternalAccountRepository) CountWithoutIdentityAt(
	ctx context.Context, providerIDs []shared.ID,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	providers := make([]pgtype.UUID, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		provider, err := uuidOf(providerID)
		if err != nil {
			return 0, err
		}
		providers = append(providers, provider)
	}
	counted, err := queries.CountAccountsWithoutIdentityAt(ctx, providers)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the accounts without a provider: %w", err))
	}
	return int(counted), nil
}

var _ repository.IdentityProviderSealing = IdentityProviderRepository{}

func (IdentityProviderRepository) ListSealed(
	ctx context.Context,
) ([]repository.SealedProviderSecret, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListIdentityProviderSecrets(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the identity providers' secrets: %w", err))
	}
	sealed := make([]repository.SealedProviderSecret, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		sealed = append(sealed, repository.SealedProviderSecret{
			ProviderID: id,
			Sealed: crypto.Sealed{
				KeyID: row.ClientSecretKeyID, Ciphertext: row.ClientSecretEnc,
			},
		})
	}
	return sealed, nil
}

func (IdentityProviderRepository) RewrapSecret(
	ctx context.Context, providerID shared.ID, sealed crypto.Sealed, expectedKeyID string,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(providerID)
	if err != nil {
		return false, err
	}
	changed, err := queries.RewrapIdentityProviderSecret(ctx, sqlc.RewrapIdentityProviderSecretParams{
		ClientSecretEnc: sealed.Ciphertext, ClientSecretKeyID: sealed.KeyID,
		ID: key, ExpectedKeyID: expectedKeyID,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("re-sealing the identity provider's secret: %w", err))
	}
	return changed > 0, nil
}
