// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package idempotency makes a repeated write harmless.
//
// The rule from api-guidelines.md §5: a POST carrying an Idempotency-Key is executed once, and
// every repeat of it gets the first answer back unchanged. That is what lets an automation
// platform or an agent retry a request whose answer it never saw - which, over a network, is
// every request that timed out.
package idempotency

import (
	"context"
	"crypto/subtle"

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Guard reserves keys and stores answers.
type Guard struct {
	Store      repository.Store
	UnitOfWork persistence.UnitOfWork
}

// Attempt is what the caller does next.
type Attempt struct {
	// Replay is set when a previous attempt already answered. The caller sends it as-is and does
	// not run the operation.
	Replay *repository.Record
}

// IsReplay reports whether the operation has already run.
func (a Attempt) IsReplay() bool { return a.Replay != nil }

// Begin claims the key for this request.
//
// It runs in a transaction of its own, which commits before the operation starts. That is the
// whole mechanism: two requests arriving together race for the same insert, one wins, and the
// loser sees a reserved row rather than a second execution.
func (g Guard) Begin(
	ctx context.Context,
	actor appshared.ActorContext,
	key repository.Key,
	requestHash []byte,
) (Attempt, error) {
	var attempt Attempt

	err := g.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		existing, reserved, err := g.Store.Reserve(ctx, key, requestHash)
		if err != nil {
			return err
		}
		if reserved {
			return nil
		}

		// Constant time, because the comparison is against a value the client supplies. The
		// difference it protects is small, but a hash comparison that leaks its prefix is the
		// kind of thing that is free to get right and expensive to explain later.
		if subtle.ConstantTimeCompare(existing.RequestHash, requestHash) != 1 {
			return shared.ErrConflict.WithDetail("idempotency.key_reused")
		}
		if existing.InProgress() {
			// The first attempt has not answered yet. Not an error the client caused, and not one
			// it should retry immediately - which is what 409 with this detail code says.
			return shared.ErrConflict.WithDetail("idempotency.in_progress")
		}

		replay := existing
		attempt.Replay = &replay
		return nil
	})
	if err != nil {
		return Attempt{}, err
	}
	return attempt, nil
}

// Complete stores the answer so the next repeat can be served from it.
//
// Only answers below 500 are stored. A 5xx is not a decision the server stands behind - it is one
// it may make differently next time - so a retry has to reach the operation rather than the
// stored failure (api-guidelines.md §5).
func (g Guard) Complete(
	ctx context.Context,
	actor appshared.ActorContext,
	key repository.Key,
	status int,
	body []byte,
) error {
	if status >= serverErrorFloor {
		return nil
	}
	return g.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return g.Store.Complete(ctx, key, status, body)
	})
}

const serverErrorFloor = 500
