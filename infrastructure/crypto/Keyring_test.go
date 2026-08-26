// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// The ring is configuration read at startup, so every refusal here is a process that does not
// start - which is the right outcome for all of them: a key nobody can name, a key named twice,
// and a key short enough to have been typed from memory.

func TestTheFirstKeyIsTheCurrentOne(t *testing.T) {
	built, err := crypto.NewKeyring([]crypto.KeyMaterial{
		key("b", materialB), key("a", materialA),
	})
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	if built.ActiveKeyID() != "b" {
		t.Fatalf("the current key is %q, want the first configured", built.ActiveKeyID())
	}
	if !slices.Equal(built.KeyIDs(), []string{"b", "a"}) {
		t.Fatalf("the ring holds %v", built.KeyIDs())
	}
	if built.IsEmpty() {
		t.Error("a ring with two keys says it is empty")
	}
}

// An installation that encrypts nothing needs no key, so an empty ring is a valid configuration
// rather than a startup failure. What it must not do is pretend to have one.
func TestAnEmptyRingIsValidAndSaysSo(t *testing.T) {
	built, err := crypto.NewKeyring(nil)
	if err != nil {
		t.Fatalf("an installation with no key would not start: %v", err)
	}
	if !built.IsEmpty() || built.ActiveKeyID() != "" || len(built.KeyIDs()) != 0 {
		t.Fatalf("the empty ring reports %v / %q", built.KeyIDs(), built.ActiveKeyID())
	}
}

func TestTheRingRefusesWhatItCannotStandBehind(t *testing.T) {
	cases := map[string]struct {
		entries []crypto.KeyMaterial
		code    string
	}{
		"an identifier with a capital": {
			[]crypto.KeyMaterial{key("A", materialA)}, "crypto.key_id_invalid",
		},
		"an identifier with a hyphen": {
			[]crypto.KeyMaterial{key("key-1", materialA)}, "crypto.key_id_invalid",
		},
		"no identifier at all": {
			[]crypto.KeyMaterial{key("", materialA)}, "crypto.key_id_invalid",
		},
		"an identifier longer than a column wants": {
			[]crypto.KeyMaterial{key(strings.Repeat("k", 33), materialA)}, "crypto.key_id_invalid",
		},
		"the same identifier twice": {
			[]crypto.KeyMaterial{key("a", materialA), key("a", materialB)},
			"crypto.key_id_duplicate",
		},
		"material somebody could have typed from memory": {
			[]crypto.KeyMaterial{key("a", "too short")}, "crypto.key_too_short",
		},
		"no material at all": {
			[]crypto.KeyMaterial{{ID: "a", Material: secret.Secret{}}}, "crypto.key_too_short",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := crypto.NewKeyring(c.entries)
			if err == nil {
				t.Fatal("the ring was built anyway")
			}
			if !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("refused as %v", err)
			}
			if code := shared.AsError(err).DetailCode; code != c.code {
				t.Fatalf("detail code %q, want %s", code, c.code)
			}
			// The refusal may name the key and its required length. It may never name the
			// material, not even a prefix of it (rule 10, T-18).
			if strings.Contains(err.Error(), "too short") && strings.Contains(err.Error(), materialA) {
				t.Fatal("the refusal quoted the material")
			}
		})
	}
}

// Two keys configured with the same material are still two different keys: the identifier is part
// of the label the material is stretched through, so a value sealed under one does not open under
// the other by accident.
func TestTheIdentifierIsPartOfTheKey(t *testing.T) {
	under := envelope(t, key("a", materialA))
	sealed, err := under.Seal(t.Context(), secret.New("the value"), purpose)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	// The same material, a different name - and the ciphertext still names `a`, so this is the
	// wrong-key path rather than the unknown-key one.
	renamed := envelope(t, key("a", materialB), key("b", materialA))
	if _, err := renamed.Open(t.Context(), sealed, purpose); err == nil {
		t.Fatal("the same material under a different identifier opened the value")
	}
}
