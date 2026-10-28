// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-07 check 2, UC-INS-19 check 2: a short name the product or the operator reserved is
// refused - exactly, as a plural, in any case - and every other is left to the label's grammar.
func TestReservedShortNames(t *testing.T) {
	operator := []string{"shop", "Intranet"}
	cases := []struct {
		name     string
		slug     string
		reserved bool
	}{
		{"the product's, exactly", "www", true},
		{"the product's, exactly, another", "media", true},
		{"the product's, as written in its list", "status", true},
		{"a plural with s", "apis", true},
		{"a plural with es", "statuses", true},
		{"the singular of a plural kept as a word", "docs", true},
		{"upper case", "ADMIN", true},
		{"mixed case and spaces", "  Auth ", true},
		{"the operator's, exactly", "shop", true},
		{"the operator's, plural", "shops", true},
		{"the operator's entry written in upper case", "intranet", true},
		{"a name that only starts like one", "apiary", false},
		{"a name that only ends like one", "my-www", false},
		{"an ordinary workspace", "familie-meier", false},
		{"a name nobody listed", "acme", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsReservedShortName(tc.slug, operator); got != tc.reserved {
				t.Errorf("IsReservedShortName(%q) = %v, want %v", tc.slug, got, tc.reserved)
			}
			err := RefuseReservedShortName(tc.slug, operator)
			if !tc.reserved {
				if err != nil {
					t.Errorf("refused %q: %v", tc.slug, err)
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || refusal.DetailCode != "admin.slug_reserved" ||
				len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/slug" ||
				refusal.Fields[0].Code != "admin.slug_reserved" {
				t.Errorf("refusal of %q = %v, want admin.slug_reserved at /slug", tc.slug, err)
			}
		})
	}
}

// The product's list is singular throughout - an entry written plural would leave its singular
// free - and reserves each of its own entries.
func TestTheProductsListIsWrittenSingular(t *testing.T) {
	endsInS := map[string]bool{"status": true, "news": true, "dns": true}
	for _, entry := range ProductReservedShortNames() {
		if !endsInS[entry] && entry[len(entry)-1] == 's' {
			t.Errorf("%q is a plural; write the singular", entry)
		}
		if !IsReservedShortName(entry, nil) {
			t.Errorf("%q is not reserved by its own list", entry)
		}
	}
}

// The operator's list is stored trimmed, lower case, each once and sorted; an entry no short name
// could be is refused.
func TestValidReservedShortNames(t *testing.T) {
	got, err := ValidReservedShortNames([]string{" Shop", "intranet", "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"intranet", "shop"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, err := ValidReservedShortNames(nil); err != nil || len(got) != 0 {
		t.Errorf("an empty list answered %v, %v", got, err)
	}
	for _, bad := range []string{"", "a_b", "-shop", "x", "shop.example"} {
		_, err := ValidReservedShortNames([]string{bad})
		var refusal *shared.Error
		if !errors.As(err, &refusal) || refusal.DetailCode != "admin.reserved_short_name_invalid" {
			t.Errorf("%q answered %v, want admin.reserved_short_name_invalid", bad, err)
		}
	}
}
