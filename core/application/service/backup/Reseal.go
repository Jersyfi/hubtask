// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// TargetResealer moves a workspace's backup target credentials under the current master key
// (ADR-0045). The purpose is credentialPurpose's, bound to the target's row.
type TargetResealer struct {
	Targets   repository.CredentialSealings
	Encryptor crypto.Encryptor
}

var _ sealing.Resealer = TargetResealer{}

func (TargetResealer) Store() string { return "backup_target" }

func (r TargetResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	credentials, err := r.Targets.SealedNotUnder(ctx, r.Encryptor.ActiveKeyID())
	if err != nil {
		return outcome, err
	}
	for _, credential := range credentials {
		moved, err := r.Encryptor.Rewrap(
			ctx, credential.Credential, credentialPurpose(credential.TargetID))
		if err != nil {
			if sealing.Unopenable(err) {
				outcome.Skipped++
				continue
			}
			return outcome, err
		}
		rewrapped, err := r.Targets.Rewrap(
			ctx, credential.TargetID, moved, credential.Credential.KeyID)
		if err != nil {
			return outcome, err
		}
		if rewrapped {
			outcome.Rewrapped++
		}
	}
	return outcome, nil
}
