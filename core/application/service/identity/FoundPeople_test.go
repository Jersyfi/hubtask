// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A member finds people by their names; somebody who is not a member - a requester of the
// portal - finds nobody (UC-WRK-18 check 9, UC-SVC-05).
func TestAMemberFindsPeopleAndARequesterNobody(t *testing.T) {
	rows := []searchrepo.Row{{ID: shared.ID("0193a0c0-0000-7000-8000-0000000000f1"), Title: "Ada Lovelace", Tile: "USER", State: "ACTIVE"}}
	for _, c := range []struct {
		name   string
		member bool
		want   int
	}{{"a member", true, 1}, {"a requester", false, 0}} {
		t.Run(c.name, func(t *testing.T) {
			finder := PeopleFinder{Members: st.HubReader{Member: c.member}, Names: st.Names{Rows: rows}, UnitOfWork: st.UnitOfWork{}}
			found, err := finder.Find(t.Context(), st.Actor(accountsRead), search.Question{Terms: []string{"ada"}, Size: 5})
			if err != nil || found.Count != c.want {
				t.Fatalf("found %+v, %v; want %d", found, err, c.want)
			}
		})
	}
}
