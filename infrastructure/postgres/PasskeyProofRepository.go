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

// PasskeyProofRepository keeps the challenges a known account answers (migration 0176) where the
// pending credentials live, hashed with their pepper.
type PasskeyProofRepository struct {
	pendingHasher security.PendingTokenHasher
}

var _ repository.PasskeyProofs = PasskeyProofRepository{}

// NewPasskeyProofRepository builds it with the pending credentials' hasher.
func NewPasskeyProofRepository(pendingHasher security.PendingTokenHasher) PasskeyProofRepository {
	return PasskeyProofRepository{pendingHasher: pendingHasher}
}

func (r PasskeyProofRepository) Issue(
	ctx context.Context, challenge identity.PasskeyProofChallenge, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(challenge.ID)
	if err != nil {
		return err
	}
	account, err := uuidOf(challenge.AccountID)
	if err != nil {
		return err
	}
	session, err := optionalUUID(challenge.SessionID)
	if err != nil {
		return err
	}
	if err := queries.InsertPasskeyProofChallenge(ctx, sqlc.InsertPasskeyProofChallengeParams{
		ID: id, AccountID: account, TokenHash: r.pendingHasher.Hash(presented.Secret()),
		Purpose: string(challenge.Purpose), CreatedAt: timestampOf(challenge.CreatedAt),
		ExpiresAt: timestampOf(challenge.ExpiresAt), SessionID: session, UserHandle: challenge.UserHandle,
	}); err != nil {
		return passkeyQueryFailed("writing a passkey proof's challenge", err)
	}
	return nil
}

func (r PasskeyProofRepository) Spend(
	ctx context.Context, purpose identity.PendingPurpose, presented identity.Token,
	accountID, sessionID shared.ID, now time.Time,
) (identity.PasskeyProofChallenge, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.PasskeyProofChallenge{}, false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return identity.PasskeyProofChallenge{}, false, err
	}
	session, err := optionalUUID(sessionID)
	if err != nil {
		return identity.PasskeyProofChallenge{}, false, err
	}
	row, err := queries.ConsumePasskeyProofChallenge(ctx, sqlc.ConsumePasskeyProofChallengeParams{
		Now: timestampOf(now), TokenHash: r.pendingHasher.Hash(presented.Secret()),
		Purpose: string(purpose), AccountID: account, SessionID: session,
	})
	if err != nil {
		if IsNoRows(err) {
			return identity.PasskeyProofChallenge{}, false, nil
		}
		return identity.PasskeyProofChallenge{}, false, passkeyQueryFailed("spending a passkey proof's challenge", err)
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.PasskeyProofChallenge{}, false, err
	}
	return identity.PasskeyProofChallenge{
		ID: id, Purpose: purpose, AccountID: accountID, SessionID: sessionID, UserHandle: row.UserHandle,
		CreatedAt: timeFrom(row.CreatedAt), ExpiresAt: timeFrom(row.ExpiresAt),
	}, true, nil
}
