// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"errors"
	"testing"
)

func TestParseIDAcceptsTheCanonicalForm(t *testing.T) {
	const raw = "01936f2a-7c1e-7000-8000-a1b2c3d4e5f6"

	id, err := ParseID(raw)

	if err != nil {
		t.Fatalf("a canonical UUID was rejected: %v", err)
	}
	if id.String() != raw {
		t.Errorf("String() = %q, want %q", id, raw)
	}
	if id.IsZero() {
		t.Error("a parsed identifier reports itself as absent")
	}
}

func TestParseIDRejectsWhatIsNotAnIdentifier(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"too short":        "01936f2a-7c1e-7000-8000-a1b2c3d4e5f",
		"too long":         "01936f2a-7c1e-7000-8000-a1b2c3d4e5f66",
		"upper case":       "01936F2A-7C1E-7000-8000-A1B2C3D4E5F6",
		"no hyphens":       "01936f2a7c1e70008000a1b2c3d4e5f6aaaa",
		"hyphen misplaced": "01936f2a7-c1e-7000-8000-a1b2c3d4e5f6",
		"not hex":          "01936g2a-7c1e-7000-8000-a1b2c3d4e5f6",
		"an SQL fragment":  "1' OR '1'='1                        ",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseID(raw)

			if err == nil {
				t.Fatalf("%q was accepted as an identifier", raw)
			}
			if !errors.Is(err, ErrValidation) {
				t.Errorf("category = %v, want a validation error", err)
			}
			var domainErr *Error
			if errors.As(err, &domainErr) && domainErr.DetailCode != "shared.id_malformed" {
				t.Errorf("detail code = %q", domainErr.DetailCode)
			}
		})
	}
}

// Upper case is rejected rather than folded: two spellings of one identifier become two cache
// keys and two index entries.
func TestUpperCaseIsNotSilentlyAccepted(t *testing.T) {
	lower, err := ParseID("01936f2a-7c1e-7000-8000-a1b2c3d4e5f6")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if _, err := ParseID("01936F2A-7C1E-7000-8000-A1B2C3D4E5F6"); err == nil {
		t.Error("the upper-case spelling was accepted, so one identifier now has two forms")
	}
	if lower.String() != "01936f2a-7c1e-7000-8000-a1b2c3d4e5f6" {
		t.Errorf("the accepted form was altered: %q", lower)
	}
}

func TestTheZeroIDIsAbsent(t *testing.T) {
	var id ID

	if !id.IsZero() {
		t.Error("the zero value does not report itself as absent")
	}
	if id.String() != "" {
		t.Errorf("String() = %q, want empty", id)
	}
}

func TestMustParseIDPanicsOnAMalformedLiteral(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParseID accepted a malformed literal")
		}
	}()

	MustParseID("not-an-id")
}

// The version is reported, not enforced: an import or an older installation may carry v4, and
// refusing those at the door would make a migration impossible.
func TestTheUUIDVersionIsReportedNotEnforced(t *testing.T) {
	v7 := MustParseID("01936f2a-7c1e-7000-8000-a1b2c3d4e5f6")
	v4 := MustParseID("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	if !v7.IsUUIDv7() {
		t.Error("a v7 identifier is not recognised as one")
	}
	if v4.IsUUIDv7() {
		t.Error("a v4 identifier passes as v7")
	}
	if _, err := ParseID(v4.String()); err != nil {
		t.Errorf("a v4 identifier was rejected: %v", err)
	}
}
