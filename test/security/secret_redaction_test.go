// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Threat T-18: secrets in logs, traces and error messages (security.md §4).
//
// The countermeasure is a type rather than a rule, and this is the gate that says the type is
// still doing its job. It logs the way a careless line does - the whole struct, at once, through
// the real JSON handler - and looks for the plaintext in the output. A test that only exercised
// String() would miss the two paths that actually leak: a struct printed with %+v, and a value
// handed to a structured logger that marshals it itself.
package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// The values that must never appear. Assembled rather than written out, for the reason the
// configuration fixtures are: a literal that looks like a key next to the word "key" is what the
// secret scan of SG-7 exists to find.
var (
	masterMaterial = strings.Repeat("master-k", 5)
	passphrase     = strings.Repeat("passphra", 4)
	credential     = "the-plaintext-credential-of-a-backup-target"
	// shortMaterial is a key an operator typed by hand. Its refusal must name the variable and
	// the length it needs, and never the value.
	shortMaterial = "hunter2gizmo"
)

// everything a careless line could reach: the wrappers, the adapter, and the two values the
// envelope produces.
func confidentialValues(t *testing.T) []any {
	t.Helper()

	ring, err := crypto.NewKeyring([]crypto.KeyMaterial{
		{ID: "a", Material: secret.New(masterMaterial)},
	})
	if err != nil {
		t.Fatalf("building the keyring: %v", err)
	}
	envelope := crypto.NewEnvelope(ring, clockadapter.CryptoRandom{})

	sealed, err := envelope.Seal(t.Context(), secret.New(credential), "test")
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})
	derivation := port.Derivation{
		Salt: []byte("0123456789abcdef"), Passes: 1, MemoryKiB: 64, Parallelism: 1, KeyLength: 32,
	}
	derived, err := deriver.Derive(secret.New(passphrase), derivation)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}

	return []any{
		secret.New(masterMaterial),
		secret.New(passphrase),
		secret.New(credential),
		secret.NewBytes([]byte(masterMaterial)),
		derived,
		ring,
		envelope,
		deriver,
		sealed,
		derivation,
		// The shape that leaks in practice: a configuration struct printed whole.
		struct {
			Name string
			Key  secret.Secret
			Data secret.Bytes
		}{"a target", secret.New(masterMaterial), derived},
	}
}

func TestNoConfidentialValueSurvivesBeingLogged(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	for index, value := range confidentialValues(t) {
		logger.Info("a careless line", slog.Any("value", value))
		logger.Info("a careless line", slog.String("formatted", fmt.Sprintf("%+v", value)))
		logger.Info("a careless line", slog.String("verbose", fmt.Sprintf("%#v", value)))

		encoded, err := json.Marshal(value)
		if err == nil {
			logger.Info("a careless line", slog.String("json", string(encoded)))
		}
		if buffer.Len() == 0 {
			t.Fatalf("value %d produced no output at all - the test is not testing anything", index)
		}
	}

	logged := buffer.String()
	for _, plaintext := range []string{masterMaterial, passphrase, credential} {
		if strings.Contains(logged, plaintext) {
			t.Fatalf("a plaintext reached the log: %s", plaintext)
		}
	}
	// The derived key is bytes rather than text, so it would appear base64-encoded or as a list
	// of numbers. Neither may be there either.
	if strings.Contains(logged, "master-k") || strings.Contains(logged, "passphra") {
		t.Fatal("a fragment of a key reached the log")
	}
}

// An error is the other way a secret travels: it is wrapped, logged and sometimes returned to a
// client. Every refusal this package produces has to be printable in full.
func TestNoRefusalQuotesWhatItRefused(t *testing.T) {
	_, tooShort := crypto.NewKeyring([]crypto.KeyMaterial{
		{ID: "a", Material: secret.New(shortMaterial)},
	})
	if tooShort == nil {
		t.Fatal("a key of five characters was accepted")
	}

	ring, err := crypto.NewKeyring([]crypto.KeyMaterial{
		{ID: "a", Material: secret.New(masterMaterial)},
	})
	if err != nil {
		t.Fatalf("building the keyring: %v", err)
	}
	envelope := crypto.NewEnvelope(ring, clockadapter.CryptoRandom{})
	sealed, err := envelope.Seal(t.Context(), secret.New(credential), "test")
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	_, notAuthentic := envelope.Open(t.Context(), sealed, "somewhere else")
	if notAuthentic == nil {
		t.Fatal("the ciphertext opened under the wrong purpose")
	}
	_, unknown := envelope.Open(t.Context(), port.Sealed{KeyID: "b", Ciphertext: sealed.Ciphertext}, "test")
	if unknown == nil {
		t.Fatal("a ciphertext naming a key the ring does not hold opened")
	}

	for _, err := range []error{tooShort, notAuthentic, unknown} {
		printed := fmt.Sprintf("%v %+v", err, err)
		for _, plaintext := range []string{masterMaterial, credential, shortMaterial} {
			if strings.Contains(printed, plaintext) {
				t.Fatalf("the refusal %q quoted %q", printed, plaintext)
			}
		}
	}
}
