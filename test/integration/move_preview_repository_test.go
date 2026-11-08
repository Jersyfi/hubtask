// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-WRK-39 check 3: what a collection's move leaves pointing back at the hub it leaves - its
// entries in a release of that hub and its running or paused clocks measured by a policy of that
// hub - counted from real releases and clocks; another hub's, another collection's and a met clock
// are not, and the workspace next door counts nothing (a cross-tenant negative for both methods).
func TestAMovesLeftoversAreTheReleaseEntriesAndRunningClocksOfTheHubItLeaves(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	from, to, collection, other := freshID(t), freshID(t), freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var none shared.ID
		for _, each := range []struct{ id, parent shared.ID }{{from, none}, {to, none}, {collection, from}, {other, from}} {
			container := containerIn(tenantA, authorA, each.id, freshName(t), "a0")
			if each.parent != none {
				container = collectionIn(tenantA, authorA, each.id, each.parent, freshName(t), "a0")
			}
			if _, err := containerRepo().Insert(ctx, container); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the hubs and collections: %v", err)
	}
	held, paused, elsewhere := seedTask(ctx, t, tenantA, authorA, collection), seedTask(ctx, t, tenantA, authorA, collection), seedTask(ctx, t, tenantA, authorA, collection)
	neighbour := seedTask(ctx, t, tenantA, authorA, other)

	admin := adminPool(ctx, t)
	exec := func(what, sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding %s: %v", what, err)
		}
	}
	release, otherRelease := freshID(t), freshID(t)
	for _, each := range []struct{ id, hub shared.ID }{{release, from}, {otherRelease, to}} {
		exec("a release", `INSERT INTO release (id, tenant_id, hub_id, name, release_on, state, created_at, updated_at)
			VALUES ($1, $2, $3, $4, '2026-12-01', 'PLANNED', $5, $5)`, each.id.String(), tenantA.String(), each.hub.String(), freshName(t), created)
	}
	for _, each := range []struct{ release, entry shared.ID }{{release, held}, {release, paused}, {release, neighbour}, {otherRelease, elsewhere}} {
		exec("a release entry", `INSERT INTO release_entry (tenant_id, release_id, entry_id, added_at) VALUES ($1, $2, $3, $4)`,
			tenantA.String(), each.release.String(), each.entry.String(), created)
	}
	policy := fmt.Sprintf("kept_%d", fixtureCounter.Add(1))
	otherPolicy := fmt.Sprintf("kept_%d", fixtureCounter.Add(1))
	for _, each := range []struct {
		key string
		hub shared.ID
	}{{policy, from}, {otherPolicy, to}} {
		exec("a policy", `INSERT INTO sla_policy (id, tenant_id, key, name, entry_types, goals, hub_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, '{TASK}', '[{"kind":"RESOLUTION","minutes":240}]', $5, $6, $6)`,
			freshID(t).String(), tenantA.String(), each.key, freshName(t), each.hub.String(), created)
	}
	for _, each := range []struct {
		entry          shared.ID
		goal, standing string
		key            string
	}{
		{held, "RESOLUTION", "RUNNING", policy},
		{paused, "FIRST_RESPONSE", "PAUSED", policy},
		{held, "FIRST_RESPONSE", "MET", policy},
		{elsewhere, "RESOLUTION", "RUNNING", otherPolicy},
		{neighbour, "RESOLUTION", "RUNNING", policy},
	} {
		var met any
		if each.standing == "MET" {
			met = created
		}
		exec("a clock", `INSERT INTO entry_sla (id, tenant_id, item_id, goal, policy_key, target_seconds, standing, armed_at, met_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 14400, $6, $7, $8, $7)`,
			freshID(t).String(), tenantA.String(), each.entry.String(), each.goal, each.key, each.standing, created, met)
	}

	leftovers := postgres.MovePreviewRepository{}
	count := func(tenant shared.ID) (releases, clocks int) {
		t.Helper()
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			if releases, err = leftovers.KeptReleaseEntries(ctx, collection, from); err != nil {
				return err
			}
			clocks, err = leftovers.KeptSLAClocks(ctx, collection, from)
			return err
		}); err != nil {
			t.Fatalf("counting in %s: %v", tenant, err)
		}
		return releases, clocks
	}
	if releases, clocks := count(tenantA); releases != 2 || clocks != 2 {
		t.Errorf("the move leaves %d release entries and %d clocks; want 2 and 2", releases, clocks)
	}
	if releases, clocks := count(tenantB); releases != 0 || clocks != 0 {
		t.Errorf("the workspace next door counted %d release entries and %d clocks", releases, clocks)
	}
}
