// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) Pages(_ context.Context, after shared.ID, batch int) ([]knowledge.Page, error) {
	return pageAfter(s.pages, func(p knowledge.Page) string { return p.ID.String() }, after.String(), batch), nil
}

// A page is read offline (M3): the walk carries the living pages with their bodies, each filed
// under its hub, and a page in a hub the reader may not read is withheld.
func TestTheWalkCarriesThePagesOfTheHubsTheReaderReads(t *testing.T) {
	pull, f, store := walking(t)
	shown, hidden := idAt('8', 1), idAt('8', 2)
	store.pages = []knowledge.Page{
		{ID: shown, TenantID: tenant, HubID: hub, Title: "Handbook", Body: "Travel rules", Version: 1},
		{ID: hidden, TenantID: tenant, HubID: otherHub, Title: "Secret", Body: "Hidden words", Version: 1},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	var pages []shared.ID
	for _, record := range records {
		if record.Entity != "page" {
			continue
		}
		pages = append(pages, record.EntityID)
		if record.ContainerID != hub || record.Payload["body"] != "Travel rules" || record.Payload["hub_id"] != hub.String() {
			t.Errorf("a page record %+v, want it filed under its hub with its body", record)
		}
	}
	if !slices.Equal(pages, []shared.ID{shown}) {
		t.Errorf("pages delivered %v, want only %s - never %s, whose hub is out of reach", pages, shown, hidden)
	}
}
