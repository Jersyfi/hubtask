// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const key = "an-installation-secret-long-enough-to-pass"

func TestTheHashIsDeterministic(t *testing.T) {
	hasher := NewTokenHasher(secret.New(key))
	token := "hbt_pat_0193" + strings.Repeat("a", 28) + "_" + strings.Repeat("b", 43)

	first, second := hasher.Hash(token), hasher.Hash(token)
	if !bytes.Equal(first, second) {
		t.Fatal("the same token hashed to two different values - no lookup could ever match")
	}
	if len(first) != 32 {
		t.Errorf("hash length %d, want 32 (SHA-256)", len(first))
	}
}

func TestDifferentTokensHashDifferently(t *testing.T) {
	hasher := NewTokenHasher(secret.New(key))

	if bytes.Equal(hasher.Hash("hbt_pat_a"), hasher.Hash("hbt_pat_b")) {
		t.Error("two tokens share a hash")
	}
}

// The pepper is what makes a stolen database dump useless: the same token under a different
// installation secret has to produce a different hash, or the dump could be attacked offline
// (security.md §8).
func TestThePepperSeparatesInstallations(t *testing.T) {
	token := "hbt_pat_0193" + strings.Repeat("a", 28) + "_" + strings.Repeat("b", 43)

	here := NewTokenHasher(secret.New(key)).Hash(token)
	elsewhere := NewTokenHasher(secret.New("a-different-installation-secret-entirely")).Hash(token)

	if bytes.Equal(here, elsewhere) {
		t.Error("the hash does not depend on the installation secret")
	}
}

// The raw configuration value must not be the key of every construction in the system: the
// derivation is what keeps one purpose's output from being replayable as another's.
func TestTheKeyIsDerivedRatherThanUsedDirectly(t *testing.T) {
	hasher := NewTokenHasher(secret.New(key))

	if bytes.Contains(hasher.pepper, []byte(key)) {
		t.Error("the installation secret appears verbatim in the derived key")
	}
	if len(hasher.pepper) != 32 {
		t.Errorf("derived key length %d, want 32", len(hasher.pepper))
	}
}
