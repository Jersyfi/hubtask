// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// switchRecords answers a record for the workspace and for each hub asked, and remembers the asks.
type switchRecords struct{ asked [][]shared.ID }

func (s *switchRecords) Records(_ context.Context, actor appshared.ActorContext, hubs []shared.ID) (map[shared.ID]map[string]any, error) {
	s.asked = append(s.asked, hubs)
	records := map[shared.ID]map[string]any{actor.TenantID: {"hub_id": nil, "modules": []any{}}}
	for _, id := range hubs {
		records[id] = map[string]any{"hub_id": id.String(), "modules": []any{}}
	}
	return records, nil
}

// settingsWalk is the walking fixture with a second hub in the store and the switches wired.
func settingsWalk(t *testing.T) (PullChanges, fixture, *switchRecords) {
	t.Helper()
	pull, f, store := walking(t)
	store.containers = append(store.containers, work.Container{ID: otherHub, TenantID: tenant, Type: work.ContainerHub, Name: "Other"})
	f.containers.parents[collectionC] = otherHub
	switches := &switchRecords{}
	pull.Switches = switches
	return pull, f, switches
}

func settingsOf(records []Record) (places, containers []shared.ID) {
	for _, record := range records {
		if record.Entity == entitySetting {
			places = append(places, record.EntityID)
			containers = append(containers, record.ContainerID)
		}
	}
	return places, containers
}

// UC-INS-21 check 12, a device starting from nothing: the walk hands a member the workspace's
// switches under no container and each hub's under the hub; a hub the reader may not read keeps
// its record.
func TestTheWalkCarriesTheSwitchesOfTheWorkspaceAndTheHubsTheReaderReads(t *testing.T) {
	pull, f, _ := settingsWalk(t)
	f.auth.allowed[otherHub] = false

	// A page of two, so the kind is resumed after the workspace's record and after a hub.
	records, _ := wholeWalk(t, pull, 2)

	places, containers := settingsOf(records)
	if !slices.Equal(places, []shared.ID{tenant, hub}) || !slices.Equal(containers, []shared.ID{"", hub}) {
		t.Errorf("switch records for %v under %v, want the workspace's under none and the readable hub's under it", places, containers)
	}
	for _, record := range records {
		if record.Entity == entitySetting && record.Payload["modules"] == nil {
			t.Errorf("a switch record without its modules: %+v", record)
		}
	}
}

// Both hubs' records reach whoever reads both, each once, whatever the page size.
func TestTheWalkCarriesEveryHubsSwitchesOnce(t *testing.T) {
	for _, limit := range []int{1, 2, 3, 100} {
		pull, _, _ := settingsWalk(t)
		records, _ := wholeWalk(t, pull, limit)
		places, _ := settingsOf(records)
		if !slices.Equal(places, []shared.ID{tenant, hub, otherHub}) {
			t.Errorf("limit %d: switch records for %v, want the workspace and both hubs once", limit, places)
		}
	}
}

// A requester is no member: neither the workspace's switches nor a hub's they may not read.
func TestTheWalkCarriesNoSwitchesToARequester(t *testing.T) {
	pull, f, _ := settingsWalk(t)
	f.auth.requester = true
	f.auth.allowed[hub], f.auth.allowed[otherHub] = false, false

	records, _ := wholeWalk(t, pull, 100)

	if places, _ := settingsOf(records); len(places) != 0 {
		t.Errorf("a requester's device was handed the switches of %v", places)
	}
}

// UC-INS-21 check 12, a signed-in device: the workspace's record in the log reaches every member
// - one whose roles are all on hubs too - and no requester; a hub's reaches whoever reads the hub.
func TestASwitchRecordReachesThePeopleItConcerns(t *testing.T) {
	steps := []struct {
		name      string
		requester bool
		want      []shared.ID
	}{
		{"a member reading hub A alone", false, []shared.ID{"", hub}},
		{"a requester", true, nil},
	}
	for _, step := range steps {
		workspaceWide, inA, inB := entry(1, ""), entry(2, hub), entry(3, otherHub)
		workspaceWide.Entity, inA.Entity, inB.Entity = entitySetting, entitySetting, entitySetting
		pull, f := pulling(t, workspaceWide, inA, inB)
		f.auth.workspace = false
		f.auth.allowed[otherHub] = false
		if step.requester {
			f.auth.requester = true
			f.auth.allowed[hub] = false
		}

		batch, err := pull.Stream.Next(t.Context(), actor(), Position{IssuedAt: now})
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		var got []shared.ID
		for _, sent := range batch.Records {
			got = append(got, sent.ContainerID)
		}
		if !slices.Equal(got, step.want) {
			t.Errorf("%s was sent the switches under %v, want %v", step.name, got, step.want)
		}
	}
}
