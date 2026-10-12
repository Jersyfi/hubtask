// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestTheRestrictedSet(t *testing.T) {
	const kim, former, other shared.ID = "01936f2a-7c1e-7000-8000-000000000a01",
		"01936f2a-7c1e-7000-8000-000000000a02", "01936f2a-7c1e-7000-8000-000000000a03"
	set := NewRestricted([]RestrictedPerson{
		{AccountID: kim, DisplayName: "Kim Keeper", Email: " Kim@Example.org "},
		{AccountID: former}, // anonymised, kept by a hold: no name, no address
		{AccountID: ""},
	})
	for _, testCase := range []struct {
		name string
		got  bool
		want bool
	}{
		{"a restricted account", set.Has(kim), true},
		{"an anonymised account a hold keeps", set.Has(former), true},
		{"somebody else", set.Has(other), false},
		{"the zero account", set.Has(""), false},
		{"their address, in another case", set.HasAddress("kim@example.ORG"), true},
		{"an empty address", set.HasAddress(" "), false},
		{"another address", set.HasAddress("lee@example.org"), false},
		{"the zero set", Restricted{}.Has(kim), false},
		{"the zero set is empty", Restricted{}.Empty(), true},
		{"a set with people is not", set.Empty(), false},
	} {
		if testCase.got != testCase.want {
			t.Errorf("%s: %v, want %v", testCase.name, testCase.got, testCase.want)
		}
	}
}
