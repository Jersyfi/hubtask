// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestASetupCodeReadsBackHoweverItIsTyped(t *testing.T) {
	code, err := NewSetupCode([]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0})
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	display := code.Display()
	if len(display) != 14 || strings.Count(display, "-") != 2 {
		t.Fatalf("display = %q, want three groups of four", display)
	}

	for _, typed := range []string{
		display,
		code.Canonical(),
		strings.ToLower(display),
		" " + strings.ReplaceAll(display, "-", " ") + " ",
	} {
		parsed, err := ParseSetupCode(typed)
		if err != nil {
			t.Errorf("%q: %v", typed, err)
			continue
		}
		if parsed.Canonical() != code.Canonical() {
			t.Errorf("%q read as %q, want %q", typed, parsed.Canonical(), code.Canonical())
		}
	}
}

func TestLettersThatLookLikeDigitsAreReadAsDigits(t *testing.T) {
	parsed, err := ParseSetupCode("oIl0-1111-2222")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if parsed.Canonical() != "011011112222" {
		t.Errorf("canonical = %q", parsed.Canonical())
	}
}

func TestWhatIsNotACodeIsTheWrongCodesRefusal(t *testing.T) {
	for _, typed := range []string{"", "ABCD-EFGH", "ABCD-EFGH-JKMN-P", "ABCD-EFGH-JKM!", "ÄBCD-EFGH-JKMN"} {
		_, err := ParseSetupCode(typed)
		var refusal *shared.Error
		if !errors.As(err, &refusal) || refusal.DetailCode != "setup.code_wrong" {
			t.Errorf("%q: %v, want setup.code_wrong", typed, err)
		}
	}
}

func TestDifferentBytesMakeDifferentCodes(t *testing.T) {
	a, _ := NewSetupCode([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	b, _ := NewSetupCode([]byte{1, 2, 3, 4, 5, 6, 7, 9})
	if a.Canonical() == b.Canonical() {
		t.Error("two draws made one code")
	}
	if _, err := NewSetupCode([]byte{1, 2}); err == nil {
		t.Error("too few bytes made a code")
	}
}

func TestASlugIsDerivedFromTheName(t *testing.T) {
	cases := map[string]string{
		"Family Winkel":         "family-winkel",
		"  Acme & Co.  ":        "acme-co",
		"Küche":                 "k-che",
		"日本":                    "workspace",
		"ab":                    "workspace",
		"Our 2nd   workspace":   "our-2nd-workspace",
		strings.Repeat("a", 60): strings.Repeat("a", MaxTenantSlugLength),
	}
	for name, want := range cases {
		if got := DeriveTenantSlug(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
		if got := DeriveTenantSlug(name); !validTenantSlug(got) {
			t.Errorf("%q: %q is not a valid short name", name, got)
		}
	}
}
