// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// PasskeySignInRepository keeps a passkey sign-in's challenge where the pending credentials live,
// hashed with their pepper, and reads and marks the credential a browser names (migration 0168).
type PasskeySignInRepository struct {
	pendingHasher security.PendingTokenHasher
}

var _ repository.PasskeySignIns = PasskeySignInRepository{}

// NewPasskeySignInRepository builds it with the pending credentials' hasher.
func NewPasskeySignInRepository(pendingHasher security.PendingTokenHasher) PasskeySignInRepository {
	return PasskeySignInRepository{pendingHasher: pendingHasher}
}

func (r PasskeySignInRepository) IssueChallenge(
	ctx context.Context, challenge identity.PendingCredential, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(challenge.ID)
	if err != nil {
		return err
	}
	if err := queries.InsertPasskeySignInChallenge(ctx, sqlc.InsertPasskeySignInChallengeParams{
		ID: id, TokenHash: r.pendingHasher.Hash(presented.Secret()),
		UserAgent: nullableString(challenge.UserAgent), IpClass: nullableString(challenge.IPClass),
		CreatedAt: timestampOf(challenge.CreatedAt), ExpiresAt: timestampOf(challenge.ExpiresAt),
	}); err != nil {
		return passkeyQueryFailed("writing a passkey sign-in's challenge", err)
	}
	return nil
}

func (r PasskeySignInRepository) SpendChallenge(
	ctx context.Context, presented identity.Token, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	spent, err := queries.ConsumePasskeySignInChallenge(ctx, sqlc.ConsumePasskeySignInChallengeParams{
		Now: timestampOf(now), TokenHash: r.pendingHasher.Hash(presented.Secret()),
	})
	if err != nil {
		return false, passkeyQueryFailed("spending a passkey sign-in's challenge", err)
	}
	return spent > 0, nil
}

func (PasskeySignInRepository) HolderOf(
	ctx context.Context, credentialID []byte,
) (repository.PasskeyHolder, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.PasskeyHolder{}, err
	}
	row, err := queries.FindPasskeyHolder(ctx, credentialID)
	if err != nil {
		if IsNoRows(err) {
			return repository.PasskeyHolder{}, shared.ErrNotFound.WithDetail("auth.sign_in_failed")
		}
		return repository.PasskeyHolder{}, passkeyQueryFailed("reading a passkey's holder", err)
	}
	credential, err := passkeyOf(sqlc.WebauthnCredential{
		ID: row.ID, TenantID: row.TenantID, AccountID: row.AccountID, CredentialID: row.CredentialID,
		PublicKey: row.PublicKey, Algorithm: row.Algorithm, SignCount: row.SignCount,
		UserHandle: row.UserHandle, BackupEligible: row.BackupEligible, BackedUp: row.BackedUp,
		Transports: row.Transports, Name: row.Name, CreatedAt: row.CreatedAt,
		LastUsedAt: row.LastUsedAt, RpID: row.RpID,
	})
	if err != nil {
		return repository.PasskeyHolder{}, err
	}
	return repository.PasskeyHolder{
		Credential: credential,
		Account: identity.Account{
			ID: credential.AccountID, TenantID: credential.TenantID,
			Kind:  identity.AccountKind(row.Kind),
			Email: stringFrom(row.Email), SignInName: stringFrom(row.SignInName),
			DisplayName: row.DisplayName, Status: identity.AccountStatus(row.Status),
			Locale: stringFrom(row.AccountLocale), TimeZone: stringFrom(row.AccountTimeZone),
		},
		TenantStatus:    identity.TenantStatus(row.TenantStatus),
		StartPasswordAt: timeFrom(row.StartPasswordAt),
	}, nil
}

func (PasskeySignInRepository) MarkUsed(
	ctx context.Context, id shared.ID, signCount uint32, backedUp bool, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	credential, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	marked, err := queries.MarkPasskeyUsed(ctx, sqlc.MarkPasskeyUsedParams{
		SignCount: int64(signCount), UsedAt: timestampOf(at), BackedUp: backedUp, ID: credential,
	})
	if err != nil {
		return false, passkeyQueryFailed("recording a passkey's use", err)
	}
	return marked > 0, nil
}
