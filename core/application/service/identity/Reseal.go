// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
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
	active := r.Encryptor.ActiveKeyID()
	rows, err := r.Enrollments.SealedNotUnder(ctx, active)
	if err != nil {
		return outcome, err
	}
	if err := r.move(ctx, rows, r.Enrollments.Rewrap, &outcome); err != nil {
		return outcome, err
	}
	// The replacements waiting to be confirmed under the same purpose: each becomes the
	// factor it is bound to, so it is sealed for that factor, and the census counts it.
	waiting, err := r.Enrollments.ReplacementsSealedNotUnder(ctx, active)
	if err != nil {
		return outcome, err
	}
	return outcome, r.move(ctx, waiting, r.Enrollments.RewrapReplacement, &outcome)
}

// move rewraps each row under its account's purpose and writes it back with the given guard.
func (r MfaResealer) move(
	ctx context.Context, rows []repository.MfaEnrollment,
	write func(context.Context, shared.ID, cryptoport.Sealed, string) (bool, error),
	outcome *sealing.Outcome,
) error {
	for _, row := range rows {
		moved, err := r.Encryptor.Rewrap(ctx, row.Secret, mfaSecretPurpose(row.AccountID))
		if err != nil {
			if sealing.Unopenable(err) {
				outcome.Skipped++
				continue
			}
			return err
		}
		rewrapped, err := write(ctx, row.AccountID, moved, row.Secret.KeyID)
		if err != nil {
			return err
		}
		if rewrapped {
			outcome.Rewrapped++
		}
	}
	return nil
}

// providerSealing is the slice of the identity provider store the resealer uses: the read of every
// wrapping at this level, and the one write that puts a moved one back.
type providerSealing interface {
	ListSealed(ctx context.Context) ([]repository.SealedProviderSecret, error)
	RewrapSecret(ctx context.Context, providerID shared.ID, sealed cryptoport.Sealed, expectedKeyID string) (bool, error)
}

// IdentityProviderResealer moves the client secrets of one level.
//
// Providers are plural, and the purpose is the level's rather than the row's, which is why
// Reseal takes the tenant: a ciphertext is bound to the workspace it was sealed for, so every row
// of that workspace opens under the same purpose.
//
// The installation's own rows are outside a per-tenant pass by construction - `ListSealed` answers
// the rows whose tenant matches the scope, and in a workspace's scope that is not NULL. They are
// re-sealed by a pass in the installation's own scope, which the rotation's driver does not run
// yet (ADR-0070's measured section says so).
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
	rows, err := r.Providers.ListSealed(ctx)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return outcome, nil
		}
		return outcome, err
	}

	active := r.Encryptor.ActiveKeyID()
	for _, row := range rows {
		if row.Sealed.KeyID == active {
			continue
		}
		moved, err := r.Encryptor.Rewrap(ctx, row.Sealed, ClientSecretPurpose(tenantID))
		if err != nil {
			if sealing.Unopenable(err) {
				// One row nobody can open does not stop the pass: the census is what says so, and
				// a rotation that gave up on the first of five would leave four behind.
				outcome.Skipped++
				continue
			}
			return outcome, err
		}
		rewrapped, err := r.Providers.RewrapSecret(ctx, row.ProviderID, moved, row.Sealed.KeyID)
		if err != nil {
			return outcome, err
		}
		if rewrapped {
			outcome.Rewrapped++
		}
	}
	return outcome, nil
}
