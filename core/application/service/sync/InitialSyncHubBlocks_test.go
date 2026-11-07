// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A hub's own type and workflow are filed under their hub (UC-FLW-01 check 10, UC-FLW-02
// check 9): a reader who may not read the hub never receives their words, the workspace's own
// reach everyone.
func TestTheWalkWithholdsTheBuildingBlocksOfAHubOutOfReach(t *testing.T) {
	pull, f, store := walking(t)
	shared1, own, hidden := idAt('9', 1), idAt('9', 2), idAt('9', 3)
	store.entryTypes = []work.EntryType{
		{ID: shared1, TenantID: tenant, Key: "bug", Level: work.ItemTask},
		{ID: own, TenantID: tenant, Key: "incident", Level: work.ItemTask, HubID: hub},
		{ID: hidden, TenantID: tenant, Key: "secret", Level: work.ItemTask, HubID: otherHub},
	}
	store.workflows = []work.Workflow{
		{ID: idAt('9', 4), TenantID: tenant, Key: "service"},
		{ID: idAt('9', 5), TenantID: tenant, Key: "support", HubID: hub},
		{ID: idAt('9', 6), TenantID: tenant, Key: "hidden", HubID: otherHub},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	delivered := map[string][]string{}
	for _, record := range records {
		if record.Entity != "entry_type" && record.Entity != "workflow" {
			continue
		}
		key, _ := record.Payload["key"].(string)
		delivered[record.Entity] = append(delivered[record.Entity], key)
		hubID, _ := record.Payload["hub_id"].(string)
		if want := shared.ID(hubID); record.ContainerID != want {
			t.Errorf("%s %s is filed under %q, want %q", record.Entity, key, record.ContainerID, want)
		}
	}
	if got := delivered["entry_type"]; !slices.Equal(got, []string{"bug", "incident"}) {
		t.Errorf("types delivered %v, want the workspace's and the readable hub's", got)
	}
	if got := delivered["workflow"]; !slices.Equal(got, []string{"service", "support"}) {
		t.Errorf("workflows delivered %v, want the workspace's and the readable hub's", got)
	}
}
