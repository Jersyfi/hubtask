// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
)

func TestTheStoredEntriesAreOneLayerPerPlaceInThePlacesOrder(t *testing.T) {
	workspace := Place{Level: domain.LevelWorkspace, ScopeID: "01TENANT"}
	hub := Place{Level: domain.LevelHub, ScopeID: "01HUB"}
	other := Place{Level: domain.LevelHub, ScopeID: "01OTHER"}
	stored := []Stored{
		{Place: hub, Key: "views", Entry: domain.Entry{Value: "KANBAN"}},
		{Place: workspace, Key: "views", Entry: domain.Entry{Value: "LIST"}},
		{Place: other, Key: "views", Entry: domain.Entry{Value: "TIMELINE"}},
	}
	layers := LayersOf([]Place{workspace, hub}, stored)
	if len(layers) != 2 || layers[0].Level != domain.LevelWorkspace || layers[1].Level != domain.LevelHub {
		t.Fatalf("layers %+v", layers)
	}
	if layers[0].Entries["views"].Value != "LIST" || layers[1].Entries["views"].Value != "KANBAN" || len(layers[1].Entries) != 1 {
		t.Errorf("entries %+v", layers)
	}
	if empty := LayersOf([]Place{workspace}, nil); len(empty) != 1 || empty[0].Entries == nil || len(empty[0].Entries) != 0 {
		t.Errorf("a place without entries: %+v", empty)
	}
}
