// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PublicFormChallengeRepository is the record of a public form's spent challenges (migration
// 0326), shared by every server of the installation.
type PublicFormChallengeRepository struct{}

// Take spends a challenge in the caller's transaction after forgetting the workspace's expired
// ones; false when it was spent already.
func (PublicFormChallengeRepository) Take(ctx context.Context, nonce string, expires, now time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	if err := queries.PruneSpentChallenges(ctx, timestampOf(now)); err != nil {
		return false, requesterFailed("forgetting expired challenges", err)
	}
	taken, err := queries.TakeChallenge(ctx, sqlc.TakeChallengeParams{Nonce: nonce, ExpiresAt: timestampOf(expires)})
	if err != nil {
		return false, requesterFailed("spending a challenge", err)
	}
	return taken == 1, nil
}
