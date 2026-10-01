// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// SC-17: the authenticator's replacement waits beside the armed factor as a second, unconfirmed
// enrolment, and becomes the armed one in a single statement - only for the session that began it,
// only inside its window, and only if it is still the secret the confirmation verified. Gate SG-3:
// none of it reaches another workspace. And the key rotation's census and re-seal see it.

var replacingAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000ca")

func TestAReplacementWaitsBesideTheFactorAndSwapsOnce(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'replacing@example.org', 'Replacing', 'ACTIVE') ON CONFLICT (id) DO NOTHING`,
		replacingAccount.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM account_mfa WHERE account_id = $1`, replacingAccount.String())
	})
	now := time.Now().UTC()
	session := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000cb")
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000cc")
	old := crypto.Sealed{KeyID: "k1", Ciphertext: []byte("the-old-secret")}
	replacement := crypto.Sealed{KeyID: "k1", Ciphertext: []byte("the-new-secret")}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// Nothing armed is nothing to replace.
		if started, err := mfa.StartReplacement(ctx, replacingAccount, replacement, session, now.Add(time.Minute), now); err != nil || started {
			t.Fatalf("a replacement began without an armed factor: (%v, %v)", started, err)
		}
		if fresh, err := mfa.Upsert(ctx, replacingAccount, old, now); err != nil || !fresh {
			t.Fatalf("enrolling: (%v, %v)", fresh, err)
		}
		if armed, err := mfa.Confirm(ctx, replacingAccount, 100, now); err != nil || !armed {
			t.Fatalf("arming: (%v, %v)", armed, err)
		}
		if started, err := mfa.StartReplacement(ctx, replacingAccount, replacement, session, now.Add(10*time.Minute), now); err != nil || !started {
			t.Fatalf("beginning the replacement: (%v, %v)", started, err)
		}
		// Beside the armed one, which is untouched.
		found, err := mfa.Find(ctx, replacingAccount)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if string(found.Secret.Ciphertext) != "the-old-secret" || found.ConfirmedAt.IsZero() {
			t.Errorf("the armed factor changed when the replacement began: %+v", found)
		}
		if found.Replacement == nil || string(found.Replacement.Ciphertext) != "the-new-secret" ||
			found.ReplacementSession != session || found.ReplacementExpiresAt.IsZero() {
			t.Errorf("the replacement reads back as %+v", found)
		}
		return nil
	})

	// Gate SG-3: StartReplacement and SwapReplacement reach nothing next door.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if started, err := mfa.StartReplacement(ctx, replacingAccount, replacement, session, now.Add(time.Minute), now); err != nil || started {
			t.Errorf("another tenant began a replacement here: (%v, %v)", started, err)
		}
		if swapped, err := mfa.SwapReplacement(ctx, replacingAccount, session, replacement, 200, now); err != nil || swapped {
			t.Errorf("another tenant swapped the factor here: (%v, %v)", swapped, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// Another session does not swap it; nor does a secret other than the one verified; nor a
		// moment after the window.
		if swapped, _ := mfa.SwapReplacement(ctx, replacingAccount, other, replacement, 200, now); swapped {
			t.Error("another session swapped the factor")
		}
		if swapped, _ := mfa.SwapReplacement(ctx, replacingAccount, session, old, 200, now); swapped {
			t.Error("a secret other than the replacement was armed")
		}
		if swapped, _ := mfa.SwapReplacement(ctx, replacingAccount, session, replacement, 200, now.Add(11*time.Minute)); swapped {
			t.Error("a lapsed replacement was armed")
		}

		// The session that began it, inside the window: one statement, once.
		swapped, err := mfa.SwapReplacement(ctx, replacingAccount, session, replacement, 200, now)
		if err != nil || !swapped {
			t.Fatalf("swapping: (%v, %v)", swapped, err)
		}
		if again, _ := mfa.SwapReplacement(ctx, replacingAccount, session, replacement, 201, now); again {
			t.Error("the replacement was armed twice")
		}
		found, err := mfa.Find(ctx, replacingAccount)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if string(found.Secret.Ciphertext) != "the-new-secret" || found.Replacement != nil || found.LastStep != 200 {
			t.Errorf("after the swap the enrolment reads %+v", found)
		}
		return nil
	})
}

// The census a key rotation ends on counts a replacement's wrapping, and the re-seal moves it: a
// rotation that ignored it would report done while a sealed value still named the leaving key.
func TestTheRotationSeesAReplacementsWrapping(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'replacing@example.org', 'Replacing', 'ACTIVE') ON CONFLICT (id) DO NOTHING`,
		replacingAccount.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM account_mfa WHERE account_id = $1`, replacingAccount.String())
	})
	now := time.Now().UTC()
	session := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000cb")

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, replacingAccount, crypto.Sealed{KeyID: "kr-armed", Ciphertext: []byte("a")}, now); err != nil {
			return err
		}
		if _, err := mfa.Confirm(ctx, replacingAccount, 1, now); err != nil {
			return err
		}
		_, err := mfa.StartReplacement(ctx, replacingAccount,
			crypto.Sealed{KeyID: "kr-leaving", Ciphertext: []byte("b")}, session, now.Add(10*time.Minute), now)
		return err
	})

	// The census a rotation ends on counts it.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		counts, err := postgres.NewSealingRepository().CountByKey(ctx)
		if err != nil {
			return err
		}
		if counts["kr-leaving"] != 1 {
			t.Errorf("the census counts %d values under the replacement's key, want one", counts["kr-leaving"])
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		pending, err := mfa.ReplacementsSealedNotUnder(ctx, "kr-current")
		if err != nil {
			return err
		}
		var mine int
		for _, row := range pending {
			if row.AccountID == replacingAccount && row.Secret.KeyID == "kr-leaving" {
				mine++
			}
		}
		if mine != 1 {
			t.Errorf("the re-seal sees %d of this replacement, want one", mine)
		}
		moved, err := mfa.RewrapReplacement(ctx, replacingAccount,
			crypto.Sealed{KeyID: "kr-current", Ciphertext: []byte("b2")}, "kr-leaving")
		if err != nil || !moved {
			t.Errorf("rewrapping the replacement: (%v, %v)", moved, err)
		}
		// Guarded by the key it was read under: a second, stale rewrap moves nothing.
		if stale, _ := mfa.RewrapReplacement(ctx, replacingAccount,
			crypto.Sealed{KeyID: "kr-current", Ciphertext: []byte("b3")}, "kr-leaving"); stale {
			t.Error("a stale rewrap overwrote the replacement")
		}
		return nil
	})
}
