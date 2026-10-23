// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// Milestones against PostgreSQL as the application role (UC-WRK-29 check 4): a completion that
// leaves nothing open records the collection's milestone and, when the whole hub is finished, the
// hub's. Tenant B, whose installation profile no other suite narrows. The catalogue carries the
// kudos doors too (kudos_test.go): both celebrate a completion.

func celebrationCatalogue(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(figureMoment)
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
	milestones := postgres.NewMilestoneRepository()
	completion := work.CompletionWriter{
		Items: itemRepo(), Containers: containers, Profiles: postgres.NewCapabilityProfileRepository(),
		Authorizer: authoriser, Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(),
		Audit: sink, Activity: work.ActivityJournal{Entries: historyRepo(), IDs: ids},
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Milestones: milestones,
	}
	desk := work.KudosDesk{
		Kudos: postgres.NewKudosRepository(), Items: itemRepo(), Containers: containers,
		Authorizer: authoriser, UnitOfWork: unitOfWork, Clock: fixed,
	}
	registry, err := usecase.NewRegistry(nil,
		work.CompleteWorkItem{Completion: completion}.Descriptor(),
		work.ReopenWorkItem{Completion: completion}.Descriptor(),
		work.GetContainer{
			Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork, Milestones: milestones,
		}.Descriptor(),
		work.ListContainers{
			Containers: containers, Authorizer: authoriser, Reader: authoriser, UnitOfWork: unitOfWork,
			Milestones: milestones,
		}.Descriptor(),
		work.ReadKudos{Desk: desk}.Descriptor(),
		work.GiveKudos{Desk: desk}.Descriptor(),
		work.TakeBackKudos{Desk: desk}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// celebrant is a person in tenant B with what a browser session holds for these doors.
func celebrant(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantB, AccountID: account, AccountName: "Anna Beispiel",
		Scopes: []string{"items:read", "items:write", "containers:read"},
	}
}

// seedSecondCollection puts another collection under the hub.
func seedSecondCollection(ctx context.Context, t *testing.T, hub shared.ID) shared.ID {
	t.Helper()
	collection := containerIn(tenantB, authorB, freshID(t), freshName(t), "a1")
	collection.Type, collection.ParentID = domain.ContainerCollection, hub
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, collection)
	}); err != nil {
		t.Fatalf("writing the collection: %v", err)
	}
	return collection.ID
}

func milestonesOf(ctx context.Context, t *testing.T, tenant shared.ID, ids ...shared.ID) map[shared.ID]time.Time {
	t.Helper()
	var found map[shared.ID]time.Time
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = postgres.NewMilestoneRepository().Of(ctx, ids)
		return err
	}); err != nil {
		t.Fatalf("reading milestones: %v", err)
	}
	return found
}

func invokeAs(
	ctx context.Context, t *testing.T, registry *usecase.Registry, name string, actor appshared.ActorContext, id shared.ID,
) (usecase.Output, error) {
	t.Helper()
	return registry.Invoke(ctx, name, actor, usecase.Input{"item_id": id.String()})
}

func restAs(
	ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext, method, path string,
) (int, map[string]any) {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), method, rest.APIBasePath+path, nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func TestAFinishedCollectionAndHubKeepTheirMilestone(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := celebrationCatalogue(ctx, t)
	owner := celebrant(authorB)

	hub, kitchen := seedHubWithCollection(ctx, t, tenantB, authorB)
	electrics := seedSecondCollection(ctx, t, hub.ID)
	first := seedEntry(ctx, t, tenantB, kitchen.ID, entryRow{})
	last := seedEntry(ctx, t, tenantB, kitchen.ID, entryRow{})
	// Neither an archived nor a trashed open entry keeps a collection from being finished.
	seedEntry(ctx, t, tenantB, kitchen.ID, entryRow{trashed: true})
	cables := seedEntry(ctx, t, tenantB, electrics, entryRow{})
	seedEntry(ctx, t, tenantB, electrics, entryRow{archived: true})

	complete := func(name string, entry *seededEntry) {
		t.Helper()
		if _, err := invokeAs(ctx, t, registry, name, owner, entry.id); err != nil {
			t.Fatalf("%s %s: %v", name, entry.id, err)
		}
	}

	complete("CompleteWorkItem", first)
	if found := milestonesOf(ctx, t, tenantB, kitchen.ID, hub.ID); len(found) != 0 {
		t.Fatalf("a milestone while an entry is open: %v", found)
	}

	complete("CompleteWorkItem", last)
	found := milestonesOf(ctx, t, tenantB, kitchen.ID, hub.ID, electrics)
	if !found[kitchen.ID].Equal(figureMoment) {
		t.Fatalf("the kitchen's milestone is %v, want %v", found[kitchen.ID], figureMoment)
	}
	if _, held := found[hub.ID]; held {
		t.Fatal("the hub has a milestone while the electrics are open")
	}

	// Reopening takes nothing away: the milestone says when everything was finished.
	complete("ReopenWorkItem", last)
	if _, held := milestonesOf(ctx, t, tenantB, kitchen.ID)[kitchen.ID]; !held {
		t.Fatal("reopening an entry took the milestone away")
	}
	complete("CompleteWorkItem", last)

	complete("CompleteWorkItem", cables)
	found = milestonesOf(ctx, t, tenantB, electrics, hub.ID)
	if !found[electrics].Equal(figureMoment) || !found[hub.ID].Equal(figureMoment) {
		t.Fatalf("after the last entry of the hub: %v", found)
	}

	// Through the REST door, on the hub's read and on its level.
	status, body := restAs(ctx, t, registry, owner, http.MethodGet, "/containers/"+hub.ID.String())
	milestone, _ := body["milestone"].(map[string]any)
	if status != http.StatusOK || milestone["reached_at"] != figureMoment.Format(time.RFC3339) {
		t.Fatalf("GET the hub: %d %v", status, body)
	}
	status, body = restAs(ctx, t, registry, owner, http.MethodGet, "/containers?parent_id="+hub.ID.String())
	data, _ := body["data"].([]any)
	if status != http.StatusOK || len(data) != 2 {
		t.Fatalf("GET the hub's level: %d %v", status, body)
	}
	for _, row := range data {
		container, _ := row.(map[string]any)
		if _, held := container["milestone"].(map[string]any); !held {
			t.Errorf("collection %v answered no milestone", container["id"])
		}
	}
}

func TestAnotherWorkspaceReadsAndRecordsNoMilestone(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := celebrationCatalogue(ctx, t)
	hub, collection := seedHubWithCollection(ctx, t, tenantB, authorB)
	entry := seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	if _, err := invokeAs(ctx, t, registry, "CompleteWorkItem", celebrant(authorB), entry.id); err != nil {
		t.Fatalf("completing: %v", err)
	}

	if found := milestonesOf(ctx, t, tenantA, collection.ID, hub.ID); len(found) != 0 {
		t.Errorf("tenant A reads tenant B's milestones: %v", found)
	}
	milestones := postgres.NewMilestoneRepository()
	_ = write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := milestones.LockHub(ctx, hub.ID); err != nil {
			return err
		}
		recorded, _ := milestones.RecordCollection(ctx, collection.ID, figureMoment)
		if recorded {
			t.Error("tenant A recorded a milestone for tenant B's collection")
		}
		return nil
	})
	_ = write(ctx, t, tenantA, func(ctx context.Context) error {
		if recorded, _ := milestones.RecordHub(ctx, hub.ID, figureMoment); recorded {
			t.Error("tenant A recorded a milestone for tenant B's hub")
		}
		return nil
	})
	if found := milestonesOf(ctx, t, tenantB, collection.ID); len(found) != 1 {
		t.Errorf("tenant B's milestone moved: %v", found)
	}
}
