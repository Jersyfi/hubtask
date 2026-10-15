// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PasskeyRepository keeps the WebAuthn credentials (identity.md §23, migration 0161).
type PasskeyRepository struct{}

var _ repository.PasskeyCredentials = PasskeyRepository{}

// NewPasskeyRepository builds it.
func NewPasskeyRepository() PasskeyRepository { return PasskeyRepository{} }

func (PasskeyRepository) ListOf(
	ctx context.Context, accountID shared.ID,
) ([]identity.PasskeyCredential, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPasskeys(ctx, account)
	if err != nil {
		return nil, passkeyQueryFailed("reading an account's passkeys", err)
	}
	credentials := make([]identity.PasskeyCredential, 0, len(rows))
	for _, row := range rows {
		credential, err := passkeyOf(row)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (PasskeyRepository) CountOf(ctx context.Context, accountID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountPasskeys(ctx, account)
	if err != nil {
		return 0, passkeyQueryFailed("counting an account's passkeys", err)
	}
	return int(count), nil
}

func (PasskeyRepository) LockAccount(ctx context.Context, accountID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if _, err := queries.LockAccountForPasskeys(ctx, account); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return passkeyQueryFailed("locking an account for its passkeys", err)
	}
	return nil
}

func (PasskeyRepository) Insert(ctx context.Context, credential identity.PasskeyCredential) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(credential.ID)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(credential.AccountID)
	if err != nil {
		return false, err
	}
	if credential.Algorithm < math.MinInt32 || credential.Algorithm > math.MaxInt32 {
		return false, shared.ErrValidation.WithDetail("auth.passkey_registration_invalid")
	}
	transports := credential.Transports
	if transports == nil {
		transports = []string{}
	}
	written, err := queries.InsertPasskey(ctx, sqlc.InsertPasskeyParams{
		ID:             id,
		AccountID:      account,
		CredentialID:   credential.CredentialID,
		PublicKey:      credential.PublicKey,
		Algorithm:      int32(credential.Algorithm),
		SignCount:      int64(credential.SignCount),
		UserHandle:     credential.UserHandle,
		BackupEligible: credential.BackupEligible,
		BackedUp:       credential.BackedUp,
		Transports:     transports,
		Name:           credential.Name,
		CreatedAt:      timestampOf(credential.CreatedAt),
	})
	if err != nil {
		return false, passkeyQueryFailed("keeping a passkey", err)
	}
	return written > 0, nil
}

func passkeyOf(row sqlc.WebauthnCredential) (identity.PasskeyCredential, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	account, err := idFrom(row.AccountID)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	if row.SignCount < 0 || row.SignCount > math.MaxUint32 {
		return identity.PasskeyCredential{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}
	return identity.PasskeyCredential{
		ID: id, TenantID: tenant, AccountID: account,
		CredentialID: row.CredentialID, PublicKey: row.PublicKey,
		Algorithm: int64(row.Algorithm), SignCount: uint32(row.SignCount),
		UserHandle: row.UserHandle, BackupEligible: row.BackupEligible, BackedUp: row.BackedUp,
		Transports: row.Transports, Name: row.Name,
		CreatedAt: timeFrom(row.CreatedAt), LastUsedAt: timeFrom(row.LastUsedAt),
	}, nil
}

// passkeyQueryFailed carries the statement's purpose to the log, never a credential or a name.
func passkeyQueryFailed(doing string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", doing, err))
}
