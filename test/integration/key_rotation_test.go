// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// The rotation drill of ADR-0045, run against the real boundary rather than against a fake.
//
// A rotation nobody has run is a hypothesis - A-20's logic, applied to keys - and the part worth
// proving is not that AES works but that the *stored* shape survives one: the key identifier is a
// column beside the ciphertext, so a rotation is only real if a value written under the old key
// comes back through the repository and opens under a ring whose current key is a different one.
//
// It ends on the refusal, deliberately. Removing a key from the ring while a row still names it is
// what an operator would do next if nothing stopped them, and this is where the system says what
// happens: a refusal an operator can read, not a silent loss - and the reason the procedure in
// security.md §8.1 ends by counting rather than by waiting.

const (
	drillKeyOne = "rotation-drill-key-one-not-a-real-secret"
	drillKeyTwo = "rotation-drill-key-two-not-a-real-secret"
)

// ring builds a keyring the way the environment would, current first.
func ring(t *testing.T, entries ...crypto.KeyMaterial) crypto.Envelope {
	t.Helper()
	keyring, err := crypto.NewKeyring(entries)
	if err != nil {
		t.Fatalf("building the keyring: %v", err)
	}
	return crypto.NewEnvelope(keyring, clockadapter.CryptoRandom{})
}

func keyOne() crypto.KeyMaterial {
	return crypto.KeyMaterial{ID: "k1", Material: secret.New(drillKeyOne)}
}

func keyTwo() crypto.KeyMaterial {
	return crypto.KeyMaterial{ID: "k2", Material: secret.New(drillKeyTwo)}
}

func TestARotationRollsForwardAndLeavesTheOldValuesReadable(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	purpose := cryptoport.Purpose("account_mfa.secret:" + sessionAccountA.String())
	plaintext := "the-second-factor-of-an-account"

	// Before the rotation: one value sealed under the only key the installation has.
	before := ring(t, keyOne())
	sealed, err := before.Seal(ctx, secret.New(plaintext), purpose)
	if err != nil {
		t.Fatalf("sealing under k1: %v", err)
	}
	if sealed.KeyID != "k1" {
		t.Fatalf("sealed under %q, want k1", sealed.KeyID)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, sealed, now); err != nil {
			t.Fatalf("storing the enrolment: %v", err)
		}
		return nil
	})

	// The rotation itself: the new key goes in front, every predecessor stays in the ring.
	after := ring(t, keyTwo(), keyOne())
	if after.ActiveKeyID() != "k2" {
		t.Fatalf("the active key is %q, want k2", after.ActiveKeyID())
	}

	// What was written before the rotation still comes back out of the database, and still opens.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		stored, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading the enrolment back: %v", err)
		}
		if stored.Secret.KeyID != "k1" {
			t.Fatalf("the stored value names %q, want the key it was sealed under", stored.Secret.KeyID)
		}
		opened, err := after.Open(ctx, stored.Secret, purpose)
		if err != nil {
			t.Fatalf("opening a k1 value under the rotated ring: %v", err)
		}
		if opened.Reveal() != plaintext {
			t.Fatal("the value that came back is not the value that went in")
		}
		return nil
	})

	// And what is written from now on names the new key, without anything having been rewritten.
	next, err := after.Seal(ctx, secret.New(plaintext), purpose)
	if err != nil {
		t.Fatalf("sealing under the rotated ring: %v", err)
	}
	if next.KeyID != "k2" {
		t.Fatalf("a value sealed after the rotation names %q, want k2", next.KeyID)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, next, now); err != nil {
			t.Fatalf("storing the re-sealed enrolment: %v", err)
		}
		stored, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading it back: %v", err)
		}
		if stored.Secret.KeyID != "k2" {
			t.Fatalf("the stored value names %q after the rewrite, want k2", stored.Secret.KeyID)
		}
		return nil
	})
}

func TestRetiringAKeyARowStillNamesIsARefusalRatherThanALoss(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	purpose := cryptoport.Purpose("account_mfa.secret:" + sessionAccountA.String())

	sealed, err := ring(t, keyOne()).Seal(ctx, secret.New("still-under-the-old-key"), purpose)
	if err != nil {
		t.Fatalf("sealing under k1: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, sealed, now); err != nil {
			t.Fatalf("storing the enrolment: %v", err)
		}
		return nil
	})

	// The operator removes k1 from the ring while a row still names it - the step the procedure
	// puts a count in front of.
	retired := ring(t, keyTwo())

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		stored, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading the enrolment back: %v", err)
		}
		_, err = retired.Open(ctx, stored.Secret, purpose)
		if err == nil {
			t.Fatal("a value sealed under a retired key opened anyway")
		}
		if !errors.Is(err, shared.ErrUnavailable) {
			t.Fatalf("opening under a retired key answered %v, want an unavailability", err)
		}
		return nil
	})
}
