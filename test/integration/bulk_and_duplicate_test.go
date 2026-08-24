// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strconv"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The acceptance runs C-11 names, against a real database and through the real catalogue: five
// hundred operations with one bad one, the same input all-or-nothing, and a copy of a subtree three
// levels deep. Each of them is a sentence in the task that would otherwise be a claim - and the
// atomic one in particular can only be proved where there is a transaction to roll back.

// deferredRegistry is the composition root's holder, in miniature: the bulk needs the catalogue
// that is built from its own descriptor, so it is handed this and this is filled straight after.
type deferredRegistry struct{ catalogue *usecase.Registry }

func (d *deferredRegistry) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return d.catalogue.Invoke(ctx, name, actor, in)
}

// bulkCatalogueFor wires the use cases C-11 needs to real repositories: the two it adds, and the
// creation the bulk performs.
func bulkCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()

	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork,
		Audit:       sink,
		Clock:       fixed,
	}
	catalogue := &deferredRegistry{}

	registry, err := usecase.NewRegistry(nil,
		work.CreateWorkItem{
			Items: itemRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Ownership: authorizer, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork,
			Clock: fixed, IDs: ids, HLC: hybrid,
		}.Descriptor(),
		work.DuplicateWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), ItemMembers: itemMemberRepo(),
			Labels: labelRepo(), Buckets: bucketRepo(), Fields: fieldRepo(),
			Containers: containerRepo(), Attachments: mediaRepo(), Media: mediaRepo(),
			Profiles: profiles, Authorizer: authorizer, Ownership: authorizer,
			Visibility: authorizer,
			Events:     outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}.Descriptor(),
		work.BulkUpdateWorkItems{
			Catalogue: catalogue, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	catalogue.catalogue = registry
	return registry
}

// creations builds one bulk of creations, with the operation at `invalidAt` missing its title -
// which is what makes it the one operation of the five hundred that cannot be applied.
func creations(collection shared.ID, count, invalidAt int) []any {
	operations := make([]any, 0, count)

	for index := range count {
		payload := map[string]any{
			"type":          "TASK",
			"collection_id": collection.String(),
			"title":         "Bulk entry " + strconv.Itoa(index),
		}
		if index == invalidAt {
			payload["title"] = "   "
		}
		operations = append(operations, map[string]any{"op": "CREATE_ITEM", "payload": payload})
	}
	return operations
}

func itemsIn(ctx context.Context, t *testing.T, tenant, collection shared.ID) int {
	t.Helper()
	return countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE tenant_id = $1 AND collection_id = $2`,
		tenant.String(), collection.String())
}

// The acceptance sentence: a bulk of 500 with one invalid operation applies 499 and reports one
// failure.
func TestABulkOfFiveHundredAppliesEveryOperationButTheInvalidOne(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantB, authorB)

	out, err := bulkCatalogueFor(ctx, t).Invoke(ctx, "BulkUpdateWorkItems", itemWriter(tenantB, authorB),
		usecase.Input{"operations": creations(collection, 500, 250)})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}

	if out.Int("applied") != 499 || out.Int("failed") != 1 {
		t.Fatalf("%d applied, %d failed", out.Int("applied"), out.Int("failed"))
	}
	// And in the database, which is the half a result cannot claim for itself.
	if rows := itemsIn(ctx, t, tenantB, collection); rows != 499 {
		t.Errorf("%d entries were written, want 499", rows)
	}

	results, _ := out["results"].([]usecase.Output)
	if len(results) != 500 {
		t.Fatalf("%d results, want one per operation", len(results))
	}
	failure, refused := results[250]["problem"].(usecase.Output)
	if !refused || results[250].Int("index") != 250 {
		t.Fatalf("the failed operation is reported as %v", results[250])
	}
	if failure["code"] != "validation_failed" {
		t.Errorf("the refusal is %v", failure)
	}
}

// And the same input with `atomic` applies none of it.
func TestTheSameBulkAtomicAppliesNoneOfIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantB, authorB)

	out, err := bulkCatalogueFor(ctx, t).Invoke(ctx, "BulkUpdateWorkItems", itemWriter(tenantB, authorB),
		usecase.Input{"atomic": true, "operations": creations(collection, 500, 250)})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}

	if out.Int("applied") != 0 {
		t.Fatalf("%d operations applied, want none", out.Int("applied"))
	}
	// The 250 entries written before the refusal were rolled back with it - which is the whole of
	// what `atomic` promises, and the half only a real transaction can show.
	if rows := itemsIn(ctx, t, tenantB, collection); rows != 0 {
		t.Errorf("%d entries survived the rollback", rows)
	}

	results, _ := out["results"].([]usecase.Output)
	first, _ := results[0]["problem"].(usecase.Output)
	if first["detail_code"] != "bulk.rolled_back" {
		t.Errorf("the first operation is reported as %v", results[0])
	}
	// The trail records the bulk even though nothing applied: the entry is written outside the
	// transaction that unwound.
	if entries := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'item.bulk_applied'`,
		tenantB.String()); entries == 0 {
		t.Error("a bulk that rolled everything back left no trace in the trail")
	}
}

// The acceptance sentence about the copy: a subtree of depth 3 produces a subtree of depth 3, with
// new identifiers and fresh ranks.
func TestACopyOfADepthThreeSubtreeProducesADepthThreeSubtree(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantB, authorB)
	registry := bulkCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": freshName(t),
	})
	if err != nil {
		t.Fatalf("the task: %v", err)
	}
	pack, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "WORK_PACKAGE", "parent_id": task.String("id"), "title": freshName(t),
	})
	if err != nil {
		t.Fatalf("the work package: %v", err)
	}
	if _, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "ACTIVITY", "parent_id": pack.String("id"), "title": freshName(t),
	}); err != nil {
		t.Fatalf("the activity: %v", err)
	}

	out, err := registry.Invoke(ctx, "DuplicateWorkItem", actor, usecase.Input{
		"item_id": task.String("id"), "include_subtree": true,
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	if out.Int("copied") != 3 {
		t.Fatalf("the copy is %d entries", out.Int("copied"))
	}
	copied, _ := out["item"].(usecase.Output)
	if copied.String("id") == task.String("id") {
		t.Fatal("the copy carries the original's identifier")
	}

	// Three rows under the copy's own path, at three depths: the shape survived.
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE tenant_id = $1 AND path LIKE $2 || '%'`,
		tenantB.String(), copied.String("path")); rows != 3 {
		t.Errorf("%d entries under the copy, want 3", rows)
	}
	if deepest := countIn(ctx, t,
		`SELECT coalesce(max(depth), 0) FROM work_item WHERE tenant_id = $1 AND path LIKE $2 || '%'`,
		tenantB.String(), copied.String("path")); deepest != 3 {
		t.Errorf("the copy reaches depth %d, want 3", deepest)
	}
	// Nothing of the original was reused: no identifier, and no rank shared with the entry it was
	// copied from - two entries with one rank would be two entries in one place.
	if shared := countIn(ctx, t,
		`SELECT count(*) FROM work_item copy JOIN work_item source
		    ON copy.order_key = source.order_key AND copy.parent_id IS NOT DISTINCT FROM source.parent_id
		 WHERE copy.tenant_id = $1 AND copy.path LIKE $2 || '%' AND source.path LIKE $3 || '%'`,
		tenantB.String(), copied.String("path"), task.String("path")); shared != 0 {
		t.Errorf("%d entries of the copy hold a rank of the original", shared)
	}
	// The conversation and the history of the original stayed where they were: the copy's history
	// is one step, its own.
	if steps := countIn(ctx, t,
		`SELECT count(*) FROM activity_entry WHERE tenant_id = $1 AND item_id = $2`,
		tenantB.String(), copied.String("id")); steps != 1 {
		t.Errorf("the copy's history is %d steps, want the one that says it was copied", steps)
	}
}

// I-W6 through a copy, against the database: a copy into another collection reports the label it
// could not resolve rather than dropping it in silence.
func TestACopyIntoAnotherCollectionReportsTheLabelItLeftBehind(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantB, authorB)
	registry := bulkCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	// A second collection under the same hub, so that the permission holds and only the vocabulary
	// differs.
	far := freshID(t)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		destination := containerIn(tenantB, authorB, far, freshName(t), "b0")
		destination.Type = domain.ContainerCollection
		destination.ParentID = hub
		return containerRepo().Insert(ctx, destination)
	}); err != nil {
		t.Fatalf("seeding the destination: %v", err)
	}

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": freshName(t),
	})
	if err != nil {
		t.Fatalf("the task: %v", err)
	}

	label := labelIn(tenantB, collection, freshID(t), freshName(t))
	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if err := labelRepo().Insert(ctx, label); err != nil {
			return err
		}
		return itemLabelRepo().Add(ctx, shared.MustParseID(task.String("id")), label.ID, added)
	}); err != nil {
		t.Fatalf("labelling the task: %v", err)
	}

	out, err := registry.Invoke(ctx, "DuplicateWorkItem", actor, usecase.Input{
		"item_id": task.String("id"), "target_collection_id": far.String(), "target_parent_id": nil,
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	copied, _ := out["item"].(usecase.Output)
	if copied.String("collection_id") != far.String() {
		t.Fatalf("the copy landed in %s", copied.String("collection_id"))
	}
	losses, _ := out["dropped_references"].([]usecase.Output)
	if len(losses) != 1 || losses[0].String("kind") != "LABEL" ||
		losses[0].String("id") != label.ID.String() ||
		losses[0].String("code") != "labels.not_in_collection" {
		t.Fatalf("the losses are %+v, want the label", losses)
	}
	// And the copy really does not carry it: reported is not the same as kept.
	if carried := itemLabels(ctx, t, tenantB, shared.MustParseID(copied.String("id"))); len(carried) != 0 {
		t.Errorf("the copy carries %v", carried)
	}
}
