// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

func (s *snapshotStore) Whiteboards(_ context.Context, after shared.ID, batch int) ([]whiteboard.Board, error) {
	return pageAfter(s.boards, func(b whiteboard.Board) string { return b.ID.String() }, after.String(), batch), nil
}

// A board is read offline before it was ever opened (UC-CNV-01 check 9): the walk carries the
// boards with their scenes, each filed under its hub, and a board in a hub the reader may not read
// is withheld.
func TestTheWalkCarriesTheBoardsOfTheHubsTheReaderReads(t *testing.T) {
	pull, f, store := walking(t)
	shown, hidden := idAt('9', 1), idAt('9', 2)
	note := whiteboard.Element{ID: "s1", Kind: whiteboard.KindSticky, Width: 160, Height: 160, Text: "Keep it"}
	store.boards = []whiteboard.Board{
		{ID: shown, TenantID: tenant, HubID: hub, Title: "Retro", Scene: whiteboard.Scene{Elements: []whiteboard.Element{note}}, Version: 3},
		{ID: hidden, TenantID: tenant, HubID: otherHub, Title: "Secret", Version: 1},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	var boards []shared.ID
	for _, record := range records {
		if record.Entity != "whiteboard" {
			continue
		}
		boards = append(boards, record.EntityID)
		elements, _ := record.Payload["elements"].([]map[string]any)
		if record.ContainerID != hub || record.Payload["hub_id"] != hub.String() || record.Payload["title"] != "Retro" ||
			record.Payload["element_count"] != 1 || (elements == nil && record.Payload["elements"] == nil) {
			t.Errorf("a board record %+v, want it filed under its hub with its scene", record)
		}
	}
	if !slices.Equal(boards, []shared.ID{shown}) {
		t.Errorf("boards delivered %v, want only %s - never %s, whose hub is out of reach", boards, shown, hidden)
	}
}
