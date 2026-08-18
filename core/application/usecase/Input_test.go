// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package usecase

import "testing"

// The three accessors the identity use cases needed, and the distinction they exist for.
func TestOptionalStringSeparatesAbsentFromEmpty(t *testing.T) {
	in := Input{"locale": "de", "time_zone": "", "week_start": nil}

	if value := in.OptionalString("locale"); value == nil || *value != "de" {
		t.Errorf("a present value read as %v", value)
	}
	if value := in.OptionalString("time_zone"); value == nil || *value != "" {
		t.Errorf("an empty value read as %v, want a pointer to the empty string - that is 'clear it'", value)
	}
	if value := in.OptionalString("missing"); value != nil {
		t.Errorf("an absent field read as %v, want nil - that is 'leave it'", value)
	}
	if value := in.OptionalString("week_start"); value != nil {
		t.Errorf("an explicit null read as %v, want nil", value)
	}
}

func TestPresentReportsWhetherTheCallerSaidAnything(t *testing.T) {
	in := Input{"members": []any{}, "name": nil}

	if !in.Present("members") {
		t.Error("an empty list read as absent - emptying a group is an instruction")
	}
	if in.Present("name") {
		t.Error("an explicit null read as present")
	}
	if in.Present("nothing") {
		t.Error("a field nobody sent read as present")
	}
}

func TestIDListReadsASetAndRefusesHalfOfOne(t *testing.T) {
	first := "01936f2a-7c1e-7000-8000-0000000000a1"
	second := "01936f2a-7c1e-7000-8000-0000000000a2"

	ids, err := (Input{"members": []any{first, second}}).IDList("members")
	if err != nil {
		t.Fatalf("reading a list: %v", err)
	}
	if len(ids) != 2 || ids[0].String() != first {
		t.Errorf("read %v", ids)
	}

	// A single identifier where a list belongs is the mistake every channel's caller writes once.
	single, err := (Input{"members": first}).IDList("members")
	if err != nil || len(single) != 1 {
		t.Errorf("a bare identifier read as %v (%v)", single, err)
	}

	absent, err := (Input{}).IDList("members")
	if err != nil || absent != nil {
		t.Errorf("an absent list read as %v (%v)", absent, err)
	}

	// Half a set of members is not a smaller request, it is a different one.
	if _, err := (Input{"members": []any{first, "not-an-id"}}).IDList("members"); err == nil {
		t.Error("a list with a malformed member was accepted")
	}
	if _, err := (Input{"members": []any{first, 42}}).IDList("members"); err == nil {
		t.Error("a list with a number in it was accepted")
	}
}
