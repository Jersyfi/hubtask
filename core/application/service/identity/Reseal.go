// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
)

// MfaResealer moves the second factors of a workspace under the current master key (ADR-0045).
// It lives here because the purpose a secret is bound to - mfaSecretPurpose - is this package's,
// and a re-seal that had to be told the purpose would be a re-seal that could be told the wrong
// one.
type MfaResealer struct {
	Enrollments repository.MfaSealings
	Encryptor   cryptoport.Encryptor
}

var _ sealing.Resealer = MfaResealer{}

func (MfaResealer) Store() string { return "account_mfa" }

func (r MfaResealer) Reseal(ctx context.Context, _ shared.ID) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	rows, err := r.Enrollments.SealedNotUnder(ctx, r.Encryptor.ActiveKeyID())
	if err != nil {
		return outcome, err
	}
	for _, row := range rows {
		moved, err := r.Encryptor.Rewrap(ctx, row.Secret, mfaSecretPurpose(row.AccountID))
		if err != nil {
			if sealing.Unopenable(err) {
				outcome.Skipped++
				continue
			}
			return outcome, err
		}
		rewrapped, err := r.Enrollments.Rewrap(ctx, row.AccountID, moved, row.Secret.KeyID)
		if err != nil {
			return outcome, err
		}
		if rewrapped {
			outcome.Rewrapped++
		}
	}
	return outcome, nil
}

// providerSealing is the slice of the identity provider store the resealer uses: the deliberate
// read of the sealed secret, and the one write that puts a moved wrapping back.
type providerSealing interface {
	FindWithSecret(ctx context.Context) (identity.IdentityProvider, cryptoport.Sealed, error)
	RewrapSecret(ctx context.Context, sealed cryptoport.Sealed, expectedKeyID string) (bool, error)
}

// IdentityProviderResealer moves a workspace's client secret. The one store whose purpose is
// bound to the workspace rather than to a row, which is why Reseal takes the tenant.
type IdentityProviderResealer struct {
	Providers providerSealing
	Encryptor cryptoport.Encryptor
}

var _ sealing.Resealer = IdentityProviderResealer{}

func (IdentityProviderResealer) Store() string { return "identity_provider" }

func (r IdentityProviderResealer) Reseal(
	ctx context.Context, tenantID shared.ID,
) (sealing.Outcome, error) {
	var outcome sealing.Outcome
	_, sealed, err := r.Providers.FindWithSecret(ctx)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return outcome, nil
		}
		return outcome, err
	}
	if sealed.KeyID == r.Encryptor.ActiveKeyID() {
		return outcome, nil
	}
	moved, err := r.Encryptor.Rewrap(ctx, sealed, clientSecretPurpose(tenantID))
	if err != nil {
		if sealing.Unopenable(err) {
			outcome.Skipped++
			return outcome, nil
		}
		return outcome, err
	}
	rewrapped, err := r.Providers.RewrapSecret(ctx, moved, sealed.KeyID)
	if err != nil {
		return outcome, err
	}
	if rewrapped {
		outcome.Rewrapped++
	}
	return outcome, nil
}
