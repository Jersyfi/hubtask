// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// ConnectorResealer moves the connectors' sealed credentials under the active key, for a key
// rotation to finish (security.md §8.1). The purpose is bound to each connector, which is why
// this package and no other re-seals them.
type ConnectorResealer struct {
	Connectors repository.Connectors
	Encryptor  crypto.Encryptor
}

var _ sealing.Resealer = ConnectorResealer{}

// Store is the census label.
func (ConnectorResealer) Store() string { return sealingrepo.StoreCmdbConnector }

// Reseal rewraps every credential of the transaction's workspace under another key.
func (r ConnectorResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	active := r.Encryptor.ActiveKeyID()
	stored, err := r.Connectors.SealedNotUnder(ctx, active)
	if err != nil {
		return outcome, err
	}
	for _, c := range stored {
		moved, err := r.Encryptor.Rewrap(ctx, crypto.Sealed{KeyID: c.KeyID, Ciphertext: c.Ciphertext}, CredentialPurpose(c.ConnectorID))
		if err != nil {
			if sealing.Unopenable(err) {
				outcome.Skipped++
				continue
			}
			return outcome, err
		}
		if err := r.Connectors.Reseal(ctx, repository.Credential{ConnectorID: c.ConnectorID, Ciphertext: moved.Ciphertext, KeyID: moved.KeyID}); err != nil {
			return outcome, err
		}
		outcome.Rewrapped++
	}
	return outcome, nil
}
