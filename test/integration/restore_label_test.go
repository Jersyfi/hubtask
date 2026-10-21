// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// A deleted label comes back with every entry that carried it (UC-WRK-28 check 3, UC-WRK-09 checks
// 2, 6 and 7), through REST, MCP and an automation action, against the database: the deletion
// rewrote no entry, so the restoration rewrites none either. Tenant B and collections of the
// test's own.

func vocabularyCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	writer := work.LabelWriter{
		Labels: labelRepo(), Containers: containerRepo(), Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, UnitOfWork: unitOfWork,
		Clock: fixed, IDs: ids, HLC: hybrid, Text: text.Composing{},
	}

	registry, err := usecase.NewRegistry(nil,
		work.CreateLabel{
			Labels: labelRepo(), Containers: containerRepo(), Authorizer: authorizer,
			Events: outbox, Changes: changes, Audit: sink, UnitOfWork: unitOfWork,
			Clock: fixed, IDs: ids, HLC: hybrid, Text: text.Composing{},
		}.Descriptor(),
		work.ListLabels{
			Labels: labelRepo(), Containers: containerRepo(), Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.DeleteLabel{Writer: writer}.Descriptor(),
		work.RestoreLabel{Writer: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// shaper may change a collection's shape and read it, as a workspace owner signed in may.
func shaper(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, AccountName: "Anna Beispiel",
		Scopes: []string{"containers:read", "containers:write", "items:read", "items:write"},
	}
}

func asREST(
	ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext,
	method, path string,
) *httptest.ResponseRecorder {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor),
		method, rest.APIBasePath+path, strings.NewReader(""))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", freshID(t).String())
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// deletedLabelOn seeds a label on a fresh task in a fresh collection of tenant B, and deletes it
// through the catalogue.
func deletedLabelOn(ctx context.Context, t *testing.T, registry *usecase.Registry) (collection, task shared.ID, label string) {
	t.Helper()
	collection = collectionFor(ctx, t, tenantB, authorB)
	task = seedTask(ctx, t, tenantB, authorB, collection)

	actor := shaper(tenantB, authorB)
	out, err := registry.Invoke(ctx, work.CreateLabelName, actor, usecase.Input{
		"collection_id": collection.String(), "name": freshName(t), "color_token": "accent.red",
	})
	if err != nil {
		t.Fatalf("creating the label: %v", err)
	}
	label = out.String("id")
	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, shared.MustParseID(label), added)
	}); err != nil {
		t.Fatalf("putting the label on the task: %v", err)
	}
	if _, err := registry.Invoke(ctx, work.DeleteLabelName, actor, usecase.Input{"label_id": label}); err != nil {
		t.Fatalf("deleting the label: %v", err)
	}
	if carried := itemLabels(ctx, t, tenantB, task); len(carried) != 0 {
		t.Fatalf("the deleted label is still shown on the task: %v", carried)
	}
	return collection, task, label
}

func TestARestoredLabelIsBackOnEveryEntryThatCarriedIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := vocabularyCatalogueFor(ctx, t)
	collection, task, label := deletedLabelOn(ctx, t, registry)
	actor := shaper(tenantB, authorB)

	listed := asREST(ctx, t, registry, actor, http.MethodGet,
		"/containers/"+collection.String()+"/labels?include_deleted=true")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"deleted_at":"`) {
		t.Fatalf("the deleted label is not listed with its stamp: %d %s", listed.Code, listed.Body)
	}
	plain := asREST(ctx, t, registry, actor, http.MethodGet, "/containers/"+collection.String()+"/labels")
	if strings.Contains(plain.Body.String(), label) {
		t.Errorf("a plain read lists the deleted label: %s", plain.Body)
	}

	restored := asREST(ctx, t, registry, actor, http.MethodPost,
		"/containers/"+collection.String()+"/labels/"+label+":restore")
	if restored.Code != http.StatusOK {
		t.Fatalf("the restoration answered %d: %s", restored.Code, restored.Body)
	}
	var answered struct {
		ID        string  `json:"id"`
		DeletedAt *string `json:"deleted_at"`
	}
	if err := json.Unmarshal(restored.Body.Bytes(), &answered); err != nil || answered.ID != label || answered.DeletedAt != nil {
		t.Errorf("the answer is not the live label: %v %s", err, restored.Body)
	}
	if carried := itemLabels(ctx, t, tenantB, task); !slices.Contains(carried, shared.MustParseID(label)) {
		t.Errorf("the task does not carry the restored label: %v", carried)
	}

	// What it owes: the event, the audit entry, and one change log entry with the whole label.
	if events := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE event_type = $1 AND subject = $2`,
		string(event.LabelRestored), "label/"+label); events != 1 {
		t.Errorf("%d label.restored events, want 1", events)
	}
	if entries := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE action = $1 AND target_id = $2`,
		string(work.LabelRestoredAction), label); entries != 1 {
		t.Errorf("%d label.restored audit entries, want 1", entries)
	}
	var op string
	var payload map[string]any
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT op, payload FROM change_log WHERE entity_id = $1 ORDER BY seq DESC LIMIT 1`, label,
	).Scan(&op, &payload); err != nil {
		t.Fatalf("reading the change log: %v", err)
	}
	if op != "UPSERT" || payload["name"] == nil || payload["color_token"] == nil ||
		payload["collection_id"] != collection.String() || payload["deleted_at"] != nil {
		t.Errorf("the last change is not the whole live label: %s %v", op, payload)
	}

	// Twice is harmless: nothing more is announced.
	again := asREST(ctx, t, registry, actor, http.MethodPost,
		"/containers/"+collection.String()+"/labels/"+label+":restore")
	if again.Code != http.StatusOK {
		t.Fatalf("a second restoration answered %d: %s", again.Code, again.Body)
	}
	if events := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE event_type = $1 AND subject = $2`,
		string(event.LabelRestored), "label/"+label); events != 1 {
		t.Errorf("a second restoration announced again: %d events", events)
	}
}

func TestARestorationIsRefusedWithItsStatus(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := vocabularyCatalogueFor(ctx, t)

	t.Run("another label took the name", func(t *testing.T) {
		collection, _, label := deletedLabelOn(ctx, t, registry)
		var name string
		if err := adminPool(ctx, t).QueryRow(ctx, `SELECT name FROM label WHERE id = $1`, label).Scan(&name); err != nil {
			t.Fatalf("reading the name: %v", err)
		}
		if _, err := registry.Invoke(ctx, work.CreateLabelName, shaper(tenantB, authorB), usecase.Input{
			"collection_id": collection.String(), "name": name, "color_token": "accent.amber",
		}); err != nil {
			t.Fatalf("taking the name: %v", err)
		}

		recorder := asREST(ctx, t, registry, shaper(tenantB, authorB), http.MethodPost,
			"/containers/"+collection.String()+"/labels/"+label+":restore")
		if recorder.Code != http.StatusConflict || problemOf(t, recorder).DetailCode != "labels.name_taken" {
			t.Errorf("answered %d %s, want 409 labels.name_taken", recorder.Code, recorder.Body)
		}
	})

	t.Run("a member without the right to shape the collection", func(t *testing.T) {
		collection, _, label := deletedLabelOn(ctx, t, registry)
		member := seedAccount(ctx, t, tenantB)
		if _, err := adminPool(ctx, t).Exec(ctx,
			`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
			 VALUES ($1, $2, $3, 'COLLECTION', $4, 'MEMBER')`,
			freshID(t).String(), tenantB.String(), member.String(), collection.String()); err != nil {
			t.Fatalf("seeding the membership: %v", err)
		}

		recorder := asREST(ctx, t, registry, shaper(tenantB, member), http.MethodPost,
			"/containers/"+collection.String()+"/labels/"+label+":restore")
		if recorder.Code != http.StatusForbidden {
			t.Errorf("answered %d %s, want 403", recorder.Code, recorder.Body)
		}
	})

	t.Run("another workspace's label", func(t *testing.T) {
		collection, _, label := deletedLabelOn(ctx, t, registry)
		recorder := asREST(ctx, t, registry, shaper(tenantA, authorA), http.MethodPost,
			"/containers/"+collection.String()+"/labels/"+label+":restore")
		if recorder.Code != http.StatusNotFound {
			t.Errorf("answered %d %s, want 404", recorder.Code, recorder.Body)
		}
		var deleted bool
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT deleted_at IS NOT NULL FROM label WHERE id = $1`, label).Scan(&deleted); err != nil || !deleted {
			t.Errorf("another workspace brought the label back: %v", err)
		}
	})
}

// MCP and an automation action reach the same restoration.
func TestALabelIsRestoredThroughMCPAndAnAction(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := vocabularyCatalogueFor(ctx, t)

	_, task, viaTool := deletedLabelOn(ctx, t, registry)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "restore_label", "arguments": map[string]any{"label_id": viaTool}},
	})
	if err != nil {
		t.Fatalf("encoding the call: %v", err)
	}
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, shaper(tenantB, authorB)),
		http.MethodPost, mcp.Path, strings.NewReader(string(body)))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if strings.Contains(recorder.Body.String(), `"isError":true`) || recorder.Code != http.StatusOK {
		t.Fatalf("the tool call failed: %d %s", recorder.Code, recorder.Body)
	}
	if carried := itemLabels(ctx, t, tenantB, task); !slices.Contains(carried, shared.MustParseID(viaTool)) {
		t.Errorf("through MCP the task does not carry the label again: %v", carried)
	}

	_, other, viaRule := deletedLabelOn(ctx, t, registry)
	rule := shaper(tenantB, authorB)
	rule.Kind = appshared.ActorAutomation
	if _, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind: "RESTORE_LABEL", Params: map[string]any{"label_id": viaRule},
	}, nil); err != nil {
		t.Fatalf("the automation action failed: %v", err)
	}
	if carried := itemLabels(ctx, t, tenantB, other); !slices.Contains(carried, shared.MustParseID(viaRule)) {
		t.Errorf("through the action the task does not carry the label again: %v", carried)
	}

	var found domain.Label
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		found, err = labelRepo().Find(ctx, shared.MustParseID(viaRule))
		return err
	}); err != nil || found.IsDeleted() {
		t.Errorf("the label is still deleted: %v %+v", err, found)
	}
}
