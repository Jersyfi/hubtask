// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// AiProviderResealer moves a workspace's API key under the current master key (ADR-0045).
//
// It arrives with the store rather than after it, which is the lesson of the re-sealing task: a
// store that seals and cannot be re-sealed is invisible until a rotation stalls on a key nobody
// can account for, and by then the census is the only thing that knows.
type AiProviderResealer struct {
	Providers repository.AiProviders
	Encryptor crypto.Encryptor
}

var _ sealing.Resealer = AiProviderResealer{}

func (AiProviderResealer) Store() string { return "ai_provider" }

func (r AiProviderResealer) Reseal(
	ctx context.Context, tenantID shared.ID,
) (sealing.Outcome, error) {
	var outcome sealing.Outcome

	_, sealed, err := r.Providers.FindWithKey(ctx)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return outcome, nil
		}
		return outcome, err
	}
	// A provider that needs no key has nothing to move, which is the ordinary state of a local
	// model rather than a gap in the rotation.
	if sealed == nil || sealed.KeyID == r.Encryptor.ActiveKeyID() {
		return outcome, nil
	}

	moved, err := r.Encryptor.Rewrap(ctx, *sealed, AiKeyPurpose(tenantID))
	if err != nil {
		if sealing.Unopenable(err) {
			outcome.Skipped++
			return outcome, nil
		}
		return outcome, err
	}
	rewrapped, err := r.Providers.RewrapKey(ctx, moved, sealed.KeyID)
	if err != nil {
		return outcome, err
	}
	if rewrapped {
		outcome.Rewrapped++
	}
	return outcome, nil
}

// OfferedAiProviderResealer moves the keys of the models the installation offers under the current
// master key (UC-AI-06 check 1, UC-INS-16 check 2). It runs in the installation's own round, under
// the scope with no tenant - the only one whose writes the installation_write policy admits - and
// binds each key to its own row, the purpose it was sealed under.
type OfferedAiProviderResealer struct {
	Providers repository.OfferedAiSealings
	Encryptor crypto.Encryptor
}

var _ sealing.Resealer = OfferedAiProviderResealer{}

func (OfferedAiProviderResealer) Store() string { return "ai_provider" }

func (r OfferedAiProviderResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	rows, err := r.Providers.ListSealed(ctx)
	if err != nil {
		return outcome, err
	}
	active := r.Encryptor.ActiveKeyID()
	for _, row := range rows {
		if row.Sealed.KeyID == active {
			continue
		}
		moved, err := r.Encryptor.Rewrap(ctx, row.Sealed, OfferedAiKeyPurpose(row.ProviderID))
		if err != nil {
			if sealing.Unopenable(err) {
				outcome.Skipped++
				continue
			}
			return outcome, err
		}
		rewrapped, err := r.Providers.RewrapKey(ctx, row.ProviderID, moved, row.Sealed.KeyID)
		if err != nil {
			return outcome, err
		}
		if rewrapped {
			outcome.Rewrapped++
		}
	}
	return outcome, nil
}

// AiKeyPurpose binds the sealed key to the workspace it belongs to, so a ciphertext lifted into
// another workspace's row does not open (the identity provider's client secret).
func AiKeyPurpose(tenantID shared.ID) crypto.Purpose {
	return crypto.Purpose("ai_provider.api_key:" + tenantID.String())
}

// OfferedAiKeyPurpose binds an offered model's sealed key to that model, so a ciphertext lifted
// into another row - a workspace's own, or another offered one - does not open.
func OfferedAiKeyPurpose(providerID shared.ID) crypto.Purpose {
	return crypto.Purpose("ai_provider.api_key:offered:" + providerID.String())
}
