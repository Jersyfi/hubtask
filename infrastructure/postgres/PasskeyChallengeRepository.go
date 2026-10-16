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

// PasskeyChallengeRepository keeps a passkey registration's challenge (migration 0161) where the
// pending credentials live, hashed with their pepper; its own statements bind it to its purpose, its
// session and its account.
type PasskeyChallengeRepository struct {
	pendingHasher security.PendingTokenHasher
}

var _ repository.PasskeyChallenges = PasskeyChallengeRepository{}

// NewPasskeyChallengeRepository builds it with the pending credentials' hasher.
func NewPasskeyChallengeRepository(pendingHasher security.PendingTokenHasher) PasskeyChallengeRepository {
	return PasskeyChallengeRepository{pendingHasher: pendingHasher}
}

func (r PasskeyChallengeRepository) Issue(
	ctx context.Context, challenge identity.PasskeyChallenge, presented identity.Token,
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
	session, err := uuidOf(challenge.SessionID)
	if err != nil {
		return err
	}
	if err := queries.SupersedePasskeyChallenges(ctx, sqlc.SupersedePasskeyChallengesParams{
		Now: timestampOf(challenge.CreatedAt), AccountID: account,
	}); err != nil {
		return passkeyQueryFailed("superseding a passkey challenge", err)
	}
	if err := queries.InsertPasskeyChallenge(ctx, sqlc.InsertPasskeyChallengeParams{
		ID: id, AccountID: account, TokenHash: r.pendingHasher.Hash(presented.Secret()),
		CreatedAt: timestampOf(challenge.CreatedAt), ExpiresAt: timestampOf(challenge.ExpiresAt),
		SessionID: session, UserHandle: challenge.UserHandle,
	}); err != nil {
		return passkeyQueryFailed("writing a passkey challenge", err)
	}
	return nil
}

func (r PasskeyChallengeRepository) Consume(
	ctx context.Context, presented identity.Token, sessionID, accountID shared.ID, now time.Time,
) (identity.PasskeyChallenge, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.PasskeyChallenge{}, false, err
	}
	session, err := uuidOf(sessionID)
	if err != nil {
		return identity.PasskeyChallenge{}, false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return identity.PasskeyChallenge{}, false, err
	}
	row, err := queries.ConsumePasskeyChallenge(ctx, sqlc.ConsumePasskeyChallengeParams{
		Now: timestampOf(now), TokenHash: r.pendingHasher.Hash(presented.Secret()),
		SessionID: session, AccountID: account,
	})
	if err != nil {
		if IsNoRows(err) {
			return identity.PasskeyChallenge{}, false, nil
		}
		return identity.PasskeyChallenge{}, false, passkeyQueryFailed("spending a passkey challenge", err)
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return identity.PasskeyChallenge{}, false, err
	}
	return identity.PasskeyChallenge{
		ID: id, AccountID: accountID, SessionID: sessionID, UserHandle: row.UserHandle,
		CreatedAt: timeFrom(row.CreatedAt), ExpiresAt: timeFrom(row.ExpiresAt),
	}, true, nil
}
