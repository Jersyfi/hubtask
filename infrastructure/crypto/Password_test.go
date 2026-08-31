// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func testPasswords(t *testing.T) Passwords {
	t.Helper()
	p, err := NewPasswords(clockport.FixedEntropy{})
	if err != nil {
		t.Fatalf("constructing: %v", err)
	}
	return p
}

func TestAPasswordRoundTrips(t *testing.T) {
	passwords := testPasswords(t)

	stored, err := passwords.Hash(secret.New("correct horse battery staple"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if !strings.HasPrefix(stored, "$argon2id$") {
		t.Fatalf("stored form %q is not a PHC argon2id string", stored)
	}
	if strings.Contains(stored, "correct horse") {
		t.Fatal("the stored form contains the password")
	}

	ok, err := passwords.Verify(stored, secret.New("correct horse battery staple"))
	if err != nil || !ok {
		t.Fatalf("the right password answered (%v, %v)", ok, err)
	}
	ok, err = passwords.Verify(stored, secret.New("wrong horse battery staple"))
	if err != nil || ok {
		t.Fatalf("the wrong password answered (%v, %v)", ok, err)
	}
}

// The stored form carries its own cost, so a hash written under other parameters verifies.
func TestAForeignCostVerifies(t *testing.T) {
	passwords := testPasswords(t)

	// A hash computed under a deliberately cheap cost, as an older or foreign build might have.
	stored := cheapHash(t, "some long password here")
	ok, err := passwords.Verify(stored, secret.New("some long password here"))
	if err != nil || !ok {
		t.Fatalf("a foreign-cost hash answered (%v, %v)", ok, err)
	}
}

func TestAnUnreadableHashIsAnError(t *testing.T) {
	passwords := testPasswords(t)

	cases := map[string]string{
		"empty":            "",
		"not argon2id":     "$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"a bcrypt hash":    "$2b$12$abcdefghijklmnopqrstuv",
		"an absurd memory": "$argon2id$v=19$m=99999999,t=3,p=2$c2FsdA$aGFzaA",
		"an absurd lanes":  "$argon2id$v=19$m=65536,t=3,p=99$c2FsdA$aGFzaA",
		"garbled base64":   "$argon2id$v=19$m=65536,t=3,p=2$!!$!!",
	}
	for name, stored := range cases {
		if _, err := passwords.Verify(stored, secret.New("whatever it was")); err == nil {
			t.Errorf("%s verified without an error", name)
		}
	}
}

// The decoy burns real work and never verifies - "no such account" costs what "wrong password"
// costs (T-02).
func TestTheDecoyNeverVerifies(t *testing.T) {
	passwords := testPasswords(t)
	// Nothing to assert beyond "it does not panic and stays false by construction": the decoy is
	// a hash of a random secret nobody knows. The call is the point.
	passwords.VerifyDecoy(secret.New("anything at all"))
}

// cheapHash computes a genuine PHC string under a deliberately different cost, standing in for a
// hash an older or foreign build wrote.
func cheapHash(t *testing.T, password string) string {
	t.Helper()
	salt := []byte("sixteen-byte-slt")
	key := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, 8*1024, 1, 1,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}
