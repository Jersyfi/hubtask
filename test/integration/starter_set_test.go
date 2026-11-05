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
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
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

// templateTaker stands for the configuration's plan and apply: it records the order of the parts
// and refuses the hub part when told to.
type templateTaker struct {
	steps       []string
	hub         shared.ID
	refuseOnHub bool
}

func (r *templateTaker) TakeForWorkspace(context.Context, appshared.ActorContext, work.StarterTemplate) error {
	r.steps = append(r.steps, "workspace")
	return nil
}

func (r *templateTaker) TakeForHub(_ context.Context, _ appshared.ActorContext, _ work.StarterTemplate, hub shared.ID) error {
	r.steps = append(r.steps, "hub")
	r.hub = hub
	if r.refuseOnHub {
		return shared.ErrValidation.WithDetail("config.refused")
	}
	return nil
}

// A set built on a template takes its workspace part before the hub exists and its hub part into
// the hub just made; a refused part leaves nothing behind and the question open (UC-WRK-30 check 5).
func TestAStarterSetOnATemplateLandsWholeOrNotAtAll(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	cmd := teamSet("Queue")
	// No preset of its own: the hub starts from the workspace's default and the template shapes it.
	cmd.Hub.Preset = ""
	cmd.Template = work.StarterTemplate{Key: "itil_service_desk", Samples: true}

	refusing := &templateTaker{refuseOnHub: true}
	apply.Templates = refusing
	if _, _, err := apply.Execute(ctx, owner, cmd); err == nil {
		t.Fatal("a set whose template's hub part was refused landed")
	}
	if n := containersOf(ctx, t, tenant); n != 0 {
		t.Errorf("a refused template left %d containers behind", n)
	}
	if !offered(ctx, t, apply.Starter, owner) {
		t.Fatal("a refused template closed the first start")
	}

	taking := &templateTaker{}
	apply.Templates = taking
	hub, _, err := apply.Execute(ctx, owner, cmd)
	if err != nil {
		t.Fatalf("the set on a template answered %v", err)
	}
	if len(taking.steps) != 2 || taking.steps[0] != "workspace" || taking.steps[1] != "hub" || taking.hub != hub.ID {
		t.Errorf("the template was taken as %v into %s, want the workspace part, then the hub part into %s", taking.steps, taking.hub, hub.ID)
	}
	if offered(ctx, t, apply.Starter, owner) {
		t.Error("the first start is still offered after its set landed")
	}
}

// A set naming a template where none can be taken is refused before anything is written.
func TestAStarterSetNamingATemplateNeedsATaker(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	cmd := teamSet()
	cmd.Template = work.StarterTemplate{Key: "kanban_team"}
	if _, _, err := apply.Execute(ctx, owner, cmd); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a set naming a template without a taker answered %v, want a validation refusal", err)
	}
	if n := containersOf(ctx, t, tenant); n != 0 {
		t.Errorf("the refused set left %d containers", n)
	}
}

// Without a template the hub's preset is the set's own choice, and a set naming none is refused.
func TestAStarterSetWithoutTemplateNeedsItsPreset(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	_, owner := freshWorkspace(ctx, t, "OWNER")
	cmd := teamSet()
	cmd.Hub.Preset = ""
	if _, _, err := apply.Execute(ctx, owner, cmd); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a set without preset or template answered %v, want a validation refusal", err)
	}
}

func skipped(ctx context.Context, t *testing.T, starter work.Starter, actor appshared.ActorContext) bool {
	t.Helper()
	answer, err := starter.StarterSkipped(ctx, actor)
	if err != nil {
		t.Fatalf("asking whether the first start was skipped: %v", err)
	}
	return answer
}

// A skipped first start still takes its set while nothing was made (user-experience.md §3.2):
// Today offers it again, the set lands once, and afterwards it is neither offered nor skipped.
func TestASkippedFirstStartTakesItsSetOnce(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	if skipped(ctx, t, apply.Starter, owner) {
		t.Fatal("a first start nobody answered reads as skipped")
	}
	if err := (work.CloseStarter{Starter: apply.Starter}).Execute(ctx, owner); err != nil {
		t.Fatalf("skipping answered %v", err)
	}
	if offered(ctx, t, apply.Starter, owner) || !skipped(ctx, t, apply.Starter, owner) {
		t.Fatal("after a skip the first start is offered, or not read as skipped")
	}
	if _, _, err := apply.Execute(ctx, owner, teamSet("Projects")); err != nil {
		t.Fatalf("a set after a skip answered %v", err)
	}
	if n := containersOf(ctx, t, tenant); n != 2 {
		t.Errorf("the set after a skip made %d containers, want the hub and one collection", n)
	}
	if skipped(ctx, t, apply.Starter, owner) {
		t.Error("a workspace whose set landed still reads as skipped")
	}
	if _, _, err := apply.Execute(ctx, owner, teamSet("Projects")); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second set after a skip answered %v, want a conflict", err)
	}
}

// Something made after the skip ends it: the set is refused, as for a workspace in use.
func TestASkippedFirstStartEndsWithAHubSomebodyMade(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	if err := (work.CloseStarter{Starter: apply.Starter}).Execute(ctx, owner); err != nil {
		t.Fatalf("skipping answered %v", err)
	}
	if _, err := apply.Containers.Execute(ctx, owner, work.CreateContainerCommand{
		Type: domainwork.ContainerHub, Name: "Own", Preset: domainwork.PresetProject,
	}); err != nil {
		t.Fatalf("making a hub answered %v", err)
	}
	if skipped(ctx, t, apply.Starter, owner) {
		t.Error("a workspace with a hub somebody made reads as skipped")
	}
	if _, _, err := apply.Execute(ctx, owner, teamSet("Projects")); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a set after a skip and a hub answered %v, want a conflict", err)
	}
	if n := containersOf(ctx, t, tenant); n != 1 {
		t.Errorf("the refused set left %d containers, want the one hub", n)
	}
}

// Two sets at once after a skip make one: the tenant row's hold makes the second wait and find the
// first one's hub, although no close is left to take.
func TestTwoStarterSetsAtOnceAfterASkipMakeOne(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	if err := (work.CloseStarter{Starter: apply.Starter}).Execute(ctx, owner); err != nil {
		t.Fatalf("skipping answered %v", err)
	}
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
		t.Errorf("%d sets landed after a skip, want one", landed)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1 AND type = 'HUB'`, tenant.String()); n != 1 {
		t.Errorf("%d hubs, want one", n)
	}
}

// Another workspace's set is not this one's: a skip is read in the reader's own workspace only.
func TestASkipIsReadInItsOwnWorkspaceOnly(t *testing.T) {
	ctx := context.Background()
	apply := starterSetDesk(t)
	_, first := freshWorkspace(ctx, t, "OWNER")
	_, second := freshWorkspace(ctx, t, "OWNER")
	for _, owner := range []appshared.ActorContext{first, second} {
		if err := (work.CloseStarter{Starter: apply.Starter}).Execute(ctx, owner); err != nil {
			t.Fatalf("skipping answered %v", err)
		}
	}
	if _, _, err := apply.Execute(ctx, first, teamSet("Projects")); err != nil {
		t.Fatalf("the first workspace's set answered %v", err)
	}
	if !skipped(ctx, t, apply.Starter, second) {
		t.Error("another workspace's hub ended this workspace's skip")
	}
	if _, _, err := apply.Execute(ctx, second, teamSet("Projects")); err != nil {
		t.Errorf("the second workspace's set answered %v", err)
	}
}
