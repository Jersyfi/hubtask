// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// WebhookResealer moves a workspace's signing secrets under the current master key (ADR-0045) -
// the current secret and, while a rotation grace is running, the previous one too, because the
// verifier has to open both.
type WebhookResealer struct {
	Subscriptions repository.WebhookSealings
	Encryptor     crypto.Encryptor
}

var _ sealing.Resealer = WebhookResealer{}

func (WebhookResealer) Store() string { return "webhook_subscription" }

func (r WebhookResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	active := r.Encryptor.ActiveKeyID()
	stored, err := r.Subscriptions.SealedNotUnder(ctx, active)
	if err != nil {
		return outcome, err
	}
	for _, row := range stored {
		purpose := SecretPurpose(row.Subscription.ID)
		secret, moved, err := r.move(ctx, row.Secret, purpose, active)
		if err != nil {
			return outcome, err
		}
		previous, movedPrevious, err := r.move(ctx, row.Previous, purpose, active)
		if err != nil {
			return outcome, err
		}
		if moved+movedPrevious == 0 {
			// Both name a key the ring no longer holds: nothing to write.
			outcome.Skipped += r.stuck(row.Secret, active) + r.stuck(row.Previous, active)
			continue
		}
		rewrapped, err := r.Subscriptions.Rewrap(
			ctx, row.Subscription.ID, secret, previous, row.Subscription.Version)
		if err != nil {
			return outcome, err
		}
		if rewrapped {
			outcome.Rewrapped += moved + movedPrevious
		}
		outcome.Skipped += r.stuck(secret, active) + r.stuck(previous, active)
	}
	return outcome, nil
}

// move rewraps one sealed secret when it names another key the ring holds, and hands back what
// should be written: the moved value, or the value as it was.
func (r WebhookResealer) move(
	ctx context.Context, sealed repository.SealedSecret, purpose crypto.Purpose, active string,
) (repository.SealedSecret, int64, error) {
	if sealed.IsZero() || sealed.KeyID == active {
		return sealed, 0, nil
	}
	moved, err := r.Encryptor.Rewrap(
		ctx, crypto.Sealed{KeyID: sealed.KeyID, Ciphertext: sealed.Ciphertext}, purpose)
	if err != nil {
		if sealing.Unopenable(err) {
			return sealed, 0, nil
		}
		return sealed, 0, err
	}
	return repository.SealedSecret{KeyID: moved.KeyID, Ciphertext: moved.Ciphertext}, 1, nil
}

func (WebhookResealer) stuck(sealed repository.SealedSecret, active string) int64 {
	if sealed.IsZero() || sealed.KeyID == active {
		return 0
	}
	return 1
}
