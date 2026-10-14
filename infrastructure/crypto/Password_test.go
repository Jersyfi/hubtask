// SPDX-License-Identifier: Apache-2.0
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

// phcOf writes a PHC string with the given cost and key length. The key is zeros: these hashes are
// only judged for their cost, never verified.
func phcOf(memoryKiB, passes, lanes, keyBytes int) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, passes, lanes,
		base64.RawStdEncoding.EncodeToString([]byte("sixteen-byte-slt")),
		base64.RawStdEncoding.EncodeToString(make([]byte, keyBytes)))
}

// A hash this build wrote needs nothing; one written under a lower cost - any of memory, passes,
// lanes or key length - is to be recomputed after the next successful verification (T-02).
func TestAHashBelowTheBuildsCostNeedsARehash(t *testing.T) {
	passwords := testPasswords(t)

	current, err := passwords.Hash(secret.New("correct horse battery staple"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if passwords.NeedsRehash(current) {
		t.Error("a hash at the build's own cost asked for a rehash")
	}

	for name, stored := range map[string]string{
		"a cheap hash":  cheapHash(t, "some long password here"),
		"less memory":   phcOf(passwordMemoryKiB/2, passwordPasses, passwordParallelism, passwordKeyBytes),
		"fewer passes":  phcOf(passwordMemoryKiB, passwordPasses-1, passwordParallelism, passwordKeyBytes),
		"fewer lanes":   phcOf(passwordMemoryKiB, passwordPasses, passwordParallelism-1, passwordKeyBytes),
		"a shorter key": phcOf(passwordMemoryKiB, passwordPasses, passwordParallelism, passwordKeyBytes/2),
	} {
		if !passwords.NeedsRehash(stored) {
			t.Errorf("%s did not ask for a rehash", name)
		}
	}
}

// A cost above the build's is left alone: during a rolling update an older build serves beside a
// newer one, and lowering what the newer one raised would have the two rewrite each other's hashes
// on every sign-in (rule 12).
func TestAHashAboveTheBuildsCostIsLeftAlone(t *testing.T) {
	passwords := testPasswords(t)
	stored := phcOf(passwordMemoryKiB*2, passwordPasses+1, passwordParallelism+1, passwordKeyBytes+16)

	if passwords.NeedsRehash(stored) {
		t.Error("a hash above the build's cost asked to be lowered")
	}
}

// An unreadable hash is Verify's error to report; a rehash would paper over the defect.
func TestAnUnreadableHashAsksForNoRehash(t *testing.T) {
	passwords := testPasswords(t)
	for _, stored := range []string{
		"", "$2b$12$abcdefghijklmnopqrstuv", "$argon2id$v=19$m=99999999,t=3,p=2$c2FsdA$aGFzaA",
	} {
		if passwords.NeedsRehash(stored) {
			t.Errorf("%q asked for a rehash", stored)
		}
	}
}
