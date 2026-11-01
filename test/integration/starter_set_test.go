// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainwork "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// The first start's set against PostgreSQL (UC-WRK-30 check 5): the hub, its collections and the
// close land together or not at all, and a second attempt never makes a second set.

func starterSetDesk(t *testing.T) work.ApplyStarter {
	t.Helper()
	ctx := context.Background()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	containers := containerRepo()
	resolver := work.StructureResolver{
		Containers: containers, Structures: postgres.NewStructureRepository(),
		Profiles: postgres.NewCapabilityProfileRepository(), Presets: postgres.NewPresetRepository(),
		Installation: postgres.NewInstallationPresetRepository(),
	}
	return work.ApplyStarter{
		Starter: starterDesk(t),
		Containers: work.CreateContainer{
			Containers: containers, Authorizer: authoriser, OwnHubs: authoriser,
			Grants:      postgres.NewMembershipGrantRepository(pageCursors()),
			PrivateHubs: privateHubsRule(unitOfWork), Events: postgres.NewOutbox(jobQueue(t)),
			Changes: postgres.NewChangeLog(), Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids,
			HLC: hybrid, Text: textadapter.Forms{}, Structure: resolver,
			Buckets: postgres.NewBucketRepository(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Renderer: catalogueRenderer(t),
		},
	}
}

func teamSet(collections ...string) work.ApplyStarterCommand {
	cmd := work.ApplyStarterCommand{Hub: work.StarterPlace{Name: "Team", Preset: domainwork.PresetProject}}
	for _, name := range collections {
		cmd.Collections = append(cmd.Collections, work.StarterPlace{Name: name})
	}
	return cmd
}

func containersOf(ctx context.Context, t *testing.T, tenant shared.ID) int {
	t.Helper()
	return countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1`, tenant.String())
}

// A set whose second collection fails leaves nothing behind and the question open; the retry
// makes exactly one set and closes it.
func TestAStarterSetCutShortLeavesNothingAndARetryMakesOne(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")

	// Two collections of one name: the second is refused, after the hub and the first were written.
	if _, _, err := apply.Execute(ctx, owner, teamSet("Projects", "Projects")); err == nil {
		t.Fatal("a set whose second collection cannot be made was applied")
	}
	if n := containersOf(ctx, t, tenant); n != 0 {
		t.Errorf("a set cut short left %d containers behind", n)
	}
	if !offered(ctx, t, apply.Starter, owner) {
		t.Fatal("a set cut short closed the first start")
	}

	hub, _, err := apply.Execute(ctx, owner, teamSet("Projects", "Requests", "Meetings"))
	if err != nil {
		t.Fatalf("the retry answered %v", err)
	}
	if hub.Type != domainwork.ContainerHub || hub.Name != "Team" {
		t.Errorf("the retry answered %+v, want the hub", hub)
	}
	if n := containersOf(ctx, t, tenant); n != 4 {
		t.Errorf("the retry made %d containers, want the hub and three collections", n)
	}
	if offered(ctx, t, apply.Starter, owner) {
		t.Error("the first start is still offered after its set landed")
	}

	// Another attempt - another key, another tab - is refused and makes nothing.
	if _, _, err := apply.Execute(ctx, owner, teamSet("Projects")); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second set answered %v, want a conflict", err)
	}
	if n := containersOf(ctx, t, tenant); n != 4 {
		t.Errorf("a second attempt changed the workspace to %d containers", n)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'workspace.starter_closed'`,
		tenant.String()); rows != 1 {
		t.Errorf("%d audit entries for the first start, want one", rows)
	}
}

// Two sets sent at once - two tabs - make one: the second waits for the first and finds it closed.
func TestTwoStarterSetsAtOnceMakeOne(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, errs[i] = apply.Execute(ctx, owner, teamSet("Projects"))
		}()
	}
	wg.Wait()
	landed := 0
	for _, err := range errs {
		switch {
		case err == nil:
			landed++
		case !errors.Is(err, shared.ErrConflict):
			t.Errorf("a set sent at once answered %v", err)
		}
	}
	if landed != 1 {
		t.Errorf("%d sets landed, want one", landed)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1 AND type = 'HUB'`, tenant.String()); n != 1 {
		t.Errorf("%d hubs, want one", n)
	}
}

// Only whoever may shape the workspace applies a set.
func TestAMemberMayNotApplyAStarterSet(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, _ := freshWorkspace(ctx, t, "OWNER")
	member := personIn(ctx, t, tenant, "MEMBER")
	if _, _, err := apply.Execute(ctx, member, teamSet()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member applying a set answered %v, want forbidden", err)
	}
}
