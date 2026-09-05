// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The one assertion worth making about a stand-in authenticator: what it produces is what the
// verifier accepts. Against the real verifier, not against a second copy of the arithmetic.
func TestTheCodeItPrintsIsOneTheServerVerifies(t *testing.T) {
	secret := []byte("0123456789abcdefghij") // 20 bytes, RFC 4226 §4's requirement
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)

	code, err := run(identity.TotpSecretBase32(secret), now)
	if err != nil {
		t.Fatalf("computing the code: %v", err)
	}
	if _, accepted := identity.VerifyTotp(secret, code, now, 0); !accepted {
		t.Errorf("the verifier refused %q", code)
	}
}

// A secret pasted off a screen carries spacing, padding and whatever case somebody typed. Failing
// on a newline would make the helper the thing a session debugs rather than the thing it uses.
func TestASecretIsReadHoweverItWasPasted(t *testing.T) {
	secret := []byte("0123456789abcdefghij")
	canonical := identity.TotpSecretBase32(secret)
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)

	want, err := run(canonical, now)
	if err != nil {
		t.Fatalf("computing the code: %v", err)
	}
	for _, pasted := range []string{
		strings.ToLower(canonical),
		canonical + "\n",
		canonical[:8] + " " + canonical[8:],
	} {
		got, err := run(pasted, now)
		if err != nil {
			t.Errorf("%q was refused: %v", pasted, err)
			continue
		}
		if got != want {
			t.Errorf("%q produced %s, want %s", pasted, got, want)
		}
	}
}

func TestASecretThatIsNotOneIsRefusedRatherThanGuessedAt(t *testing.T) {
	for _, secret := range []string{"", "not base32 at all!"} {
		if _, err := run(secret, time.Now()); err == nil {
			t.Errorf("%q was accepted", secret)
		}
	}
}
