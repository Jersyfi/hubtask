// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// UC-FLW-01 check 9 against PostgreSQL: a parent type's allowed children hold for a create and a
// move alike - a work package of a type the parent leaves out is refused by name both ways, and
// one of an allowed type moves in.
func TestAParentTypesAllowedChildrenHoldForACreateAndAMove(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO entry_type (id, tenant_id, key, level, name, icon, color_token, allowed_children, created_at, updated_at)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000fd01', '` + tenantB.String() + `', 'pl_change', 'TASK', 'Change', 'change', 'blue', '{pl_risk}', now(), now())
		 ON CONFLICT (tenant_id, key) DO NOTHING`,
		`INSERT INTO entry_type (id, tenant_id, key, level, name, icon, color_token, created_at, updated_at)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000fd02', '` + tenantB.String() + `', 'pl_risk', 'WORK_PACKAGE', 'Risk', 'risk', 'red', now(), now())
		 ON CONFLICT (tenant_id, key) DO NOTHING`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the types: %v", err)
		}
	}

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatal(err)
	}
	sink := postgres.NewAuditSink(ids)
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	containers := containerRepo()
	profiles := postgres.NewCapabilityProfileRepository()
	types := postgres.NewEntryTypeRepository()
	resolver := work.StructureResolver{
		Containers: containers, Structures: postgres.NewStructureRepository(), Profiles: profiles,
	}
	gate := work.EntryGate{Structure: resolver, Types: itemRepo(), EntryTypes: types}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	registry, err := usecase.NewRegistry(nil,
		work.CreateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containers, Profiles: profiles, Gate: gate,
			EntryTypes: types, Authorizer: authoriser, Ownership: authoriser, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Structure: resolver, Text: textadapter.Forms{},
		}.Descriptor(),
		work.MoveWorkItem{Placement: work.PlacementWriter{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containers, Profiles: profiles, Gate: gate,
			Authorizer: authoriser, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}}.Descriptor(),
	)
	if err != nil {
		t.Fatal(err)
	}
	actor := hiddenActor(tenantB, authorB)
	collection := collectionFor(ctx, t, tenantB, authorB).String()
	create := func(input usecase.Input) (string, error) {
		input["collection_id"] = collection
		out, err := registry.Invoke(ctx, "CreateWorkItem", actor, input)
		if err != nil {
			return "", err
		}
		return out.String("id"), nil
	}
	refused := func(err error) bool {
		refusal := shared.AsError(err)
		return refusal != nil && refusal.DetailCode == "entry_types.not_allowed_here"
	}

	change, err := create(usecase.Input{"type": "TASK", "title": "Replace the switch", "entry_type": "pl_change"})
	if err != nil {
		t.Fatalf("the change: %v", err)
	}
	other, err := create(usecase.Input{"type": "TASK", "title": "Order cables"})
	if err != nil {
		t.Fatalf("a plain task: %v", err)
	}
	if _, err := create(usecase.Input{"type": "WORK_PACKAGE", "title": "Label the ports", "parent_id": change}); !refused(err) {
		t.Errorf("a work package was created under a change allowing only risks: %v", err)
	}
	pack, err := create(usecase.Input{"type": "WORK_PACKAGE", "title": "Label the ports", "parent_id": other})
	if err != nil {
		t.Fatalf("the work package: %v", err)
	}
	if _, err := registry.Invoke(ctx, "MoveWorkItem", actor, usecase.Input{"item_id": pack, "target_parent_id": change}); !refused(err) {
		t.Errorf("a work package moved under a change allowing only risks: %v", err)
	}
	var parent string
	if err := admin.QueryRow(ctx, `SELECT parent_id::text FROM work_item WHERE id = $1`, pack).Scan(&parent); err != nil || parent != other {
		t.Errorf("the refused move left the package under %s, %v", parent, err)
	}

	risk, err := create(usecase.Input{"type": "WORK_PACKAGE", "title": "Rollback window", "parent_id": other, "entry_type": "pl_risk"})
	if err != nil {
		t.Fatalf("the risk: %v", err)
	}
	if _, err := registry.Invoke(ctx, "MoveWorkItem", actor, usecase.Input{"item_id": risk, "target_parent_id": change}); err != nil {
		t.Errorf("an allowed child could not move in: %v", err)
	}
}
