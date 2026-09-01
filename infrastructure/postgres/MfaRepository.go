// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// MfaRepository maintains the second factor (H-02): the sealed enrolment, the recovery codes,
// the pending credential, and the tenant's enforcement switch. It is the only place that knows
// how a pending token or a recovery code becomes a hash, AccessTokenRepository's reasoning. No
// method takes a tenant - row level security bounds every statement (ADR-0010).
type MfaRepository struct {
	pendingHasher  security.PendingTokenHasher
	recoveryHasher security.RecoveryCodeHasher
}

func NewMfaRepository(
	pendingHasher security.PendingTokenHasher,
	recoveryHasher security.RecoveryCodeHasher,
) MfaRepository {
	return MfaRepository{pendingHasher: pendingHasher, recoveryHasher: recoveryHasher}
}

var (
	_ repository.MfaEnrollments     = MfaRepository{}
	_ repository.RecoveryCodes      = MfaRepository{}
	_ repository.PendingCredentials = MfaRepository{}
	_ repository.TenantPolicy       = MfaRepository{}
)

func (r MfaRepository) Upsert(
	ctx context.Context, accountID shared.ID, sealed cryptoport.Sealed, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.UpsertMfaEnrollment(ctx, sqlc.UpsertMfaEnrollmentParams{
		AccountID:   id,
		SecretEnc:   sealed.Ciphertext,
		SecretKeyID: sealed.KeyID,
		Now:         pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the enrolment: %w", err))
	}
	return changed > 0, nil
}

func (r MfaRepository) Find(
	ctx context.Context, accountID shared.ID,
) (repository.MfaEnrollment, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.MfaEnrollment{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return repository.MfaEnrollment{}, err
	}
	row, err := queries.FindMfaEnrollment(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return repository.MfaEnrollment{}, shared.ErrNotFound.WithDetail("auth.mfa_not_enrolled")
		}
		return repository.MfaEnrollment{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the enrolment: %w", err))
	}
	enrollment := repository.MfaEnrollment{
		AccountID:   accountID,
		Secret:      cryptoport.Sealed{KeyID: row.SecretKeyID, Ciphertext: row.SecretEnc},
		ConfirmedAt: timeFrom(row.ConfirmedAt),
	}
	if row.LastStep != nil {
		enrollment.LastStep = *row.LastStep
	}
	return enrollment, nil
}

func (r MfaRepository) Confirm(
	ctx context.Context, accountID shared.ID, step int64, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.ConfirmMfaEnrollment(ctx, sqlc.ConfirmMfaEnrollmentParams{
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		Step:      &step,
		AccountID: id,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("arming the enrolment: %w", err))
	}
	return changed > 0, nil
}

func (r MfaRepository) RecordStep(
	ctx context.Context, accountID shared.ID, step int64, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.RecordMfaStep(ctx, sqlc.RecordMfaStepParams{
		Step:      &step,
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		AccountID: id,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the step: %w", err))
	}
	return changed > 0, nil
}

func (r MfaRepository) Disable(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.DisableMfa(ctx, id)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the enrolment: %w", err))
	}
	// The codes go in the same transaction: half a disable is worse than none.
	if _, err := queries.DeleteRecoveryCodes(ctx, id); err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the recovery codes: %w", err))
	}
	return changed > 0, nil
}

func (r MfaRepository) Replace(
	ctx context.Context, accountID shared.ID, ids []shared.ID, presented []string, now time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if len(ids) != len(presented) {
		return shared.ErrInternal.WithDetail("auth.session_unmintable")
	}
	if _, err := queries.DeleteRecoveryCodes(ctx, account); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("burning the old codes: %w", err))
	}
	for index, code := range presented {
		id, err := uuidOf(ids[index])
		if err != nil {
			return err
		}
		if err := queries.InsertRecoveryCode(ctx, sqlc.InsertRecoveryCodeParams{
			ID:        id,
			AccountID: account,
			CodeHash:  r.recoveryHasher.Hash(identity.NormalizeRecoveryCode(code)),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		}); err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("writing a recovery code: %w", err))
		}
	}
	return nil
}

func (r MfaRepository) Burn(
	ctx context.Context, accountID shared.ID, presented string, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.BurnRecoveryCode(ctx, sqlc.BurnRecoveryCodeParams{
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		AccountID: account,
		CodeHash:  r.recoveryHasher.Hash(identity.NormalizeRecoveryCode(presented)),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("burning the recovery code: %w", err))
	}
	return changed > 0, nil
}

func (r MfaRepository) Remaining(ctx context.Context, accountID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	left, err := queries.CountRecoveryCodes(ctx, account)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the recovery codes: %w", err))
	}
	return int(left), nil
}

func (r MfaRepository) Insert(
	ctx context.Context, credential identity.PendingCredential, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(credential.ID)
	if err != nil {
		return err
	}
	account, err := uuidOf(credential.AccountID)
	if err != nil {
		return err
	}
	if err := queries.InsertPendingCredential(ctx, sqlc.InsertPendingCredentialParams{
		ID:        id,
		AccountID: account,
		TokenHash: r.pendingHasher.Hash(presented.Secret()),
		Purpose:   string(credential.Purpose),
		UserAgent: nullableString(credential.UserAgent),
		IpClass:   nullableString(credential.IPClass),
		CreatedAt: pgtype.Timestamptz{Time: credential.CreatedAt, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: credential.ExpiresAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the pending credential: %w", err))
	}
	return nil
}

func (r MfaRepository) FindByToken(
	ctx context.Context, token identity.Token,
) (repository.PendingLookup, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.PendingLookup{}, err
	}
	row, err := queries.FindPendingByHash(ctx, r.pendingHasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			return repository.PendingLookup{}, shared.ErrNotFound.WithDetail("auth.mfa_challenge_failed")
		}
		return repository.PendingLookup{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the pending credential: %w", err))
	}

	credentialID, err := idFrom(row.ID)
	if err != nil {
		return repository.PendingLookup{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return repository.PendingLookup{}, err
	}

	return repository.PendingLookup{
		Credential: identity.PendingCredential{
			ID:         credentialID,
			TenantID:   token.TenantID(),
			AccountID:  accountID,
			Purpose:    identity.PendingPurpose(row.Purpose),
			UserAgent:  stringFrom(row.UserAgent),
			IPClass:    stringFrom(row.IpClass),
			CreatedAt:  timeFrom(row.CreatedAt),
			ExpiresAt:  timeFrom(row.ExpiresAt),
			ConsumedAt: timeFrom(row.ConsumedAt),
		},
		Account: identity.Account{
			ID:          accountID,
			TenantID:    token.TenantID(),
			Kind:        identity.AccountKind(row.AccountKind),
			DisplayName: row.AccountDisplayName,
			Status:      identity.AccountStatus(row.AccountStatus),
			Locale:      stringFrom(row.AccountLocale),
			TimeZone:    stringFrom(row.AccountTimeZone),
		},
		TenantLocale:   row.DefaultLocale,
		TenantTimeZone: row.DefaultTimeZone,
		TenantSlug:     row.TenantSlug,
		TenantStatus:   identity.TenantStatus(row.TenantStatus),
	}, nil
}

func (r MfaRepository) Consume(
	ctx context.Context, credentialID shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(credentialID)
	if err != nil {
		return false, err
	}
	changed, err := queries.ConsumePendingCredential(ctx, sqlc.ConsumePendingCredentialParams{
		Now: pgtype.Timestamptz{Time: at, Valid: true},
		ID:  id,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("consuming the pending credential: %w", err))
	}
	return changed > 0, nil
}

// RequireAdminTotp reads the enforcement switch off the tenant's settings document. The document
// is the adapter's shape: the application layer asks the question and never sees the JSON.
func (r MfaRepository) RequireAdminTotp(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	raw, err := queries.TenantSettings(ctx)
	if err != nil {
		if IsNoRows(err) {
			// No tenant row in scope means no tenant to enforce anything.
			return false, nil
		}
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the tenant settings: %w", err))
	}

	var settings struct {
		RequireAdminTotp bool `json:"require_admin_totp"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			// A settings document this build cannot read must not quietly switch enforcement
			// off - or on. Refusing is the fail-closed answer that names the defect.
			return false, shared.ErrInternal.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("parsing the tenant settings: %w", err))
		}
	}
	return settings.RequireAdminTotp, nil
}
