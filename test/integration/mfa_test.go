// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The second factor of H-02, against the real boundary. Gate SG-3: one negative per port method.

func mfaStores(ctx context.Context, t *testing.T) (postgres.MfaRepository, persistence.UnitOfWork) {
	t.Helper()
	installation := secret.New(installationSecret)
	return postgres.NewMfaRepository(
		security.NewPendingTokenHasher(installation),
		security.NewRecoveryCodeHasher(installation),
	), postgres.NewUnitOfWork(appPool(ctx, t))
}

func sealedSecret() crypto.Sealed {
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte("sealed-not-a-secret")}
}

// Gate SG-3: Upsert, Find, Confirm, RecordStep, Disable.
func TestAnEnrollmentIsInvisibleAndImmovableFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	// Enrolled and armed in its own tenant.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		fresh, err := mfa.Upsert(ctx, sessionAccountA, sealedSecret(), now)
		if err != nil || !fresh {
			t.Fatalf("enrolling (%v, %v)", fresh, err)
		}
		armed, err := mfa.Confirm(ctx, sessionAccountA, 100, now)
		if err != nil || !armed {
			t.Fatalf("arming (%v, %v)", armed, err)
		}
		return nil
	})

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		// Invisible: the read finds nothing.
		if _, err := mfa.Find(ctx, sessionAccountA); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's enrolment answered %v, want not found", err)
		}
		// Immovable: none of the writes reach it.
		if fresh, err := mfa.Upsert(ctx, sessionAccountA, sealedSecret(), now); err == nil && fresh {
			t.Error("another tenant's enrolment was replaced from here")
		}
		if armed, err := mfa.Confirm(ctx, sessionAccountA, 101, now); err != nil || armed {
			t.Errorf("another tenant's enrolment was armed from here (%v, %v)", armed, err)
		}
		if advanced, err := mfa.RecordStep(ctx, sessionAccountA, 999, now); err != nil || advanced {
			t.Errorf("another tenant's replay floor moved from here (%v, %v)", advanced, err)
		}
		if removed, err := mfa.Disable(ctx, sessionAccountA); err != nil || removed {
			t.Errorf("another tenant's enrolment was disabled from here (%v, %v)", removed, err)
		}
		return nil
	})

	// The replay floor is atomic in its own tenant: the same step never records twice.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		advanced, err := mfa.RecordStep(ctx, sessionAccountA, 101, now)
		if err != nil || !advanced {
			t.Fatalf("advancing (%v, %v)", advanced, err)
		}
		advanced, err = mfa.RecordStep(ctx, sessionAccountA, 101, now)
		if err != nil || advanced {
			t.Fatalf("the same step recorded twice (%v, %v)", advanced, err)
		}
		return nil
	})
}

// Gate SG-3: Replace, Burn, Remaining.
func TestRecoveryCodesBurnOnceAndOnlyInTheirTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	codes := []string{"AAAA-BBBB-CCCC-DDDD", "EEEE-FFFF-GGGG-HHHH"}
	ids := []shared.ID{
		shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f5"),
		shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f6"),
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		return mfa.Replace(ctx, sessionAccountA, ids, codes, now)
	})

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		burned, err := mfa.Burn(ctx, sessionAccountA, codes[0], now)
		if err != nil || burned {
			t.Errorf("another tenant's code burned from here (%v, %v)", burned, err)
		}
		left, err := mfa.Remaining(ctx, sessionAccountA)
		if err != nil || left != 0 {
			t.Errorf("another tenant's codes counted from here (%d, %v)", left, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// Mangled reading, same code: normalisation happens before the hash.
		burned, err := mfa.Burn(ctx, sessionAccountA, " aaaa bbbb cccc dddd ", now)
		if err != nil || !burned {
			t.Fatalf("burning (%v, %v)", burned, err)
		}
		burned, err = mfa.Burn(ctx, sessionAccountA, codes[0], now)
		if err != nil || burned {
			t.Fatalf("a code burned twice (%v, %v)", burned, err)
		}
		left, err := mfa.Remaining(ctx, sessionAccountA)
		if err != nil || left != 1 {
			t.Errorf("remaining %d, want one", left)
		}
		return nil
	})
}

// Gate SG-3: Insert, FindByToken, Consume - and the rewritten-tenant probe.
func TestAPendingCredentialIsBoundToItsTenantAndDiesOnce(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	presented, err := identity.NewPendingToken(tenantA, sessionSecretOf(0xD1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	credential := identity.PendingCredential{
		ID:        shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f7"),
		TenantID:  tenantA,
		AccountID: sessionAccountA,
		Purpose:   identity.PendingTotp,
		UserAgent: "hubctl/1.0",
		IPClass:   "203.0.113.0/24",
		CreatedAt: now,
		ExpiresAt: now.Add(identity.PendingLifetime),
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		return mfa.Insert(ctx, credential, presented)
	})

	// The same secret rewritten to name tenant B matches nothing at all.
	rewritten, err := identity.NewPendingToken(tenantB, sessionSecretOf(0xD1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, err := mfa.FindByToken(ctx, rewritten); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a rewritten pending token answered %v, want not found", err)
		}
		if consumed, err := mfa.Consume(ctx, credential.ID, now); err != nil || consumed {
			t.Errorf("another tenant's credential consumed from here (%v, %v)", consumed, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		lookup, err := mfa.FindByToken(ctx, presented)
		if err != nil {
			t.Fatalf("own credential not found: %v", err)
		}
		if lookup.Credential.Purpose != identity.PendingTotp || lookup.Account.ID != sessionAccountA {
			t.Errorf("lookup = %+v", lookup)
		}
		if lookup.Credential.UserAgent != "hubctl/1.0" || lookup.Credential.IPClass != "203.0.113.0/24" {
			t.Errorf("the client hint did not travel: %+v", lookup.Credential)
		}
		consumed, err := mfa.Consume(ctx, credential.ID, now)
		if err != nil || !consumed {
			t.Fatalf("consuming (%v, %v)", consumed, err)
		}
		consumed, err = mfa.Consume(ctx, credential.ID, now)
		if err != nil || consumed {
			t.Fatalf("a credential consumed twice (%v, %v)", consumed, err)
		}
		return nil
	})
}

// Gate SG-3: RequireAdminTotp reads the running transaction's tenant and no other.
func TestTheEnforcementSwitchIsPerTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		UPDATE tenant SET settings = jsonb_set(settings, '{require_admin_totp}', 'true')
		WHERE id = $1`, tenantA.String()); err != nil {
		t.Fatalf("setting the switch: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `
			UPDATE tenant SET settings = settings - 'require_admin_totp' WHERE id = $1`,
			tenantA.String())
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		required, err := mfa.RequireAdminTotp(ctx)
		if err != nil || !required {
			t.Errorf("the switch answered (%v, %v) in its own tenant", required, err)
		}
		return nil
	})
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		required, err := mfa.RequireAdminTotp(ctx)
		if err != nil || required {
			t.Errorf("another tenant's switch answered (%v, %v) here", required, err)
		}
		return nil
	})
}

// Gate SG-3: PasswordHashOf.
func TestTheStoredHashIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		hash, err := signIn.PasswordHashOf(ctx, sessionAccountA)
		if err != nil || hash.IsEmpty() {
			t.Fatalf("own hash (%v, empty=%v)", err, hash.IsEmpty())
		}
		if _, err := signIn.PasswordHashOf(ctx, sessionAccountB); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's hash answered %v, want not found", err)
		}
		return nil
	})
}
