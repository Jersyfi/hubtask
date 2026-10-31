// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	homeHub  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a7")
	otherHub = shared.MustParseID("0192f000-0000-7000-8000-0000000000a8")
)

// UC-FLW-01 check 11: an entry inside a hub may carry the hub's own type; an entry in another hub
// cannot choose it - it is unknown there - and an entry carrying it does not leave the hub.
func TestAHubsOwnTypeStaysInsideItsHub(t *testing.T) {
	ctx := context.Background()
	types := workspaceTypes()
	types.rows = append(types.rows, domain.EntryType{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e9"), TenantID: tenantID,
		Key: "incident", Level: domain.ItemTask, Name: "Incident", Icon: "bug", ColorToken: "red",
		Behaviours: domain.EntryBehaviours{Completable: true}, Version: 1, HubID: homeHub,
	})
	profile := systemProfiles()[0]

	if key, _, err := chooseEntryType(ctx, types, domain.ItemTask, "incident", nil, profile, nil, homeHub); err != nil || key != "incident" {
		t.Fatalf("inside its hub: %q, %v", key, err)
	}
	if _, _, err := chooseEntryType(ctx, types, domain.ItemTask, "incident", nil, profile, nil, otherHub); detail(err) != "entry_types.unknown" {
		t.Fatalf("in another hub: %v, want entry_types.unknown", err)
	}
	if key, _, err := chooseEntryType(ctx, types, domain.ItemTask, "bug", nil, profile, nil, otherHub); err != nil || key != "bug" {
		t.Fatalf("the workspace's type in another hub: %q, %v", key, err)
	}

	item := domain.WorkItem{Path: "/a/"}
	types.bound = true
	if err := keepsHubTypes(ctx, types, item, homeHub, otherHub); detail(err) != "entry_types.hub_bound" {
		t.Errorf("leaving the hub with its type: %v, want entry_types.hub_bound", err)
	}
	if err := keepsHubTypes(ctx, types, item, homeHub, homeHub); err != nil {
		t.Errorf("a move inside the hub: %v", err)
	}
	types.bound = false
	if err := keepsHubTypes(ctx, types, item, homeHub, otherHub); err != nil {
		t.Errorf("leaving the hub without its types: %v", err)
	}
}
