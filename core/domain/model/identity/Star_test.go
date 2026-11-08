// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A person's stars are written whole: bounded, of the known kinds, each place once (UC-WRK-37/2).
func TestCheckStars(t *testing.T) {
	page := shared.ID("0190a5f0-0000-7000-8000-000000000001")
	hub := shared.ID("0190a5f0-0000-7000-8000-000000000002")
	many := make([]Star, MaxStars+1)
	for i := range many {
		many[i] = Star{Kind: StarPage, ID: page}
	}
	cases := []struct {
		name  string
		given []Star
		want  int
		code  string
	}{
		{"none", nil, 0, ""},
		{"every kind", []Star{{StarHub, hub}, {StarCollection, hub}, {StarView, page}, {StarPage, page}, {StarWhiteboard, page}, {StarDashboard, page}, {StarPortfolio, page}}, 7, ""},
		{"the same place twice is kept once", []Star{{StarPage, page}, {StarHub, hub}, {StarPage, page}}, 2, ""},
		{"exactly the bound", many[:MaxStars], 1, ""},
		{"more than the bound", many, 0, "stars.too_many"},
		{"an unknown kind", []Star{{"FOLDER", page}}, 0, "stars.kind_invalid"},
		{"no identifier", []Star{{StarPage, ""}}, 0, "stars.id_required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CheckStars(c.given)
			if c.code != "" {
				if err == nil || shared.AsError(err).DetailCode != c.code {
					t.Fatalf("error %v, want %s", err, c.code)
				}
				return
			}
			if err != nil || len(got) != c.want {
				t.Fatalf("got %v, %v; want %d stars", got, err, c.want)
			}
		})
	}
	got, _ := CheckStars([]Star{{StarPage, page}, {StarHub, hub}, {StarPage, page}})
	if got[0].Kind != StarPage || got[1].Kind != StarHub {
		t.Errorf("order %v, want the first place of each kept", got)
	}
}
