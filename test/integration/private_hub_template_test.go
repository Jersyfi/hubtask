// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// A private hub from a template against PostgreSQL (UC-ID-16 check 9): private from its first row,
// its creator its owner, and the template's hub part taken into it in the same transaction - which
// passes only because the owner's membership, written a moment before, is read there.

// switchingTemplates takes a template's parts as far as this test needs the configuration: the
// workspace part switches Workflows on, the hub part finds the hub, asks what applying a hub's
// document asks (STRUCTURE along the workspace and the hub) and switches Workflows on in the hub -
// each through the real use case, inside the hub's transaction.
type switchingTemplates struct {
	catalogue  *usecase.Registry
	authoriser access.Service
	refuseHub  bool
}

func (s switchingTemplates) TakeWorkspacePartOnce(ctx context.Context, actor appshared.ActorContext, _ work.StarterTemplate) error {
	_, err := s.catalogue.Invoke(ctx, settingservice.SetModuleName, actor, usecase.Input{"module": "WORKFLOWS", "switch": "ON"})
	return err
}

func (s switchingTemplates) TakeForHub(ctx context.Context, actor appshared.ActorContext, _ work.StarterTemplate, hub shared.ID) error {
	if _, err := s.catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{"container_id": hub.String()}); err != nil {
		return err
	}
	if err := s.authoriser.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope(), identity.HubScope(hub)},
		Action: "config.applied", TokenScope: "workspace:manage", TargetType: "config", TargetID: actor.TenantID,
	}); err != nil {
		return err
	}
	if _, err := s.catalogue.Invoke(ctx, settingservice.SetModuleName, actor,
		usecase.Input{"module": "WORKFLOWS", "switch": "ON", "hub_id": hub.String()}); err != nil {
		return err
	}
	if s.refuseHub {
		return shared.ErrValidation.WithDetail("config.refused")
	}
	return nil
}

// privateTemplateDesk is the operation on its registry, beside the reads the test asks with.
func privateTemplateDesk(t *testing.T, refuseHub bool) *usecase.Registry {
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
	containers := starterSetDesk(t).Containers
	modules := settingservice.Modules{
		Settings: postgres.SettingRepository{}, Containers: containerRepo(), Authorizer: authoriser,
		Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids,
		Epochs: postgres.NewEpochRepository(), Hold: postgres.NewConfigRepository(),
		Changes: postgres.NewChangeLog(), HLC: hybrid,
	}
	parts, err := usecase.NewRegistry(nil,
		work.GetContainer{
			Containers: containerRepo(), Authorizer: authoriser, UnitOfWork: unitOfWork, Structure: containers.Structure,
		}.Descriptor(),
		settingservice.SetModule{Modules: modules}.Descriptor(),
		settingservice.ListModules{Modules: modules}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the parts' catalogue: %v", err)
	}
	templates := switchingTemplates{catalogue: parts, authoriser: authoriser, refuseHub: refuseHub}
	registry, err := usecase.NewRegistry(nil,
		work.CreateHubFromTemplate{Containers: containers, Templates: templates}.Descriptor(),
		work.GetContainer{
			Containers: containerRepo(), Authorizer: authoriser, UnitOfWork: unitOfWork, Structure: containers.Structure,
		}.Descriptor(),
		settingservice.ListModules{Modules: modules}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func withScopes(actor appshared.ActorContext) appshared.ActorContext {
	actor.Scopes = []string{"containers:read", "containers:write", "workspace:read", "workspace:manage"}
	return actor
}

func postHubFromTemplate(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext, body string) (int, map[string]any) {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodPost,
		rest.APIBasePath+"/containers:from-template", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	var answer map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	return recorder.Code, answer
}

func TestAPrivateHubFromATemplateIsPrivateFromItsFirstRowInPostgres(t *testing.T) {
	ctx := context.Background()
	registry := privateTemplateDesk(t, false)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	owner = withScopes(owner)
	other := withScopes(personIn(ctx, t, tenant, "ADMIN"))

	code, answer := postHubFromTemplate(ctx, t, registry, owner,
		`{"name":"Diary","template":"kanban_team","private":true}`)
	if code != http.StatusCreated {
		t.Fatalf("POST /containers:from-template answered %d %v", code, answer)
	}
	hub, _ := answer["id"].(string)
	if answer["private"] != true || hub == "" {
		t.Fatalf("the answer is not a private hub: %v", answer)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1 AND id = $2 AND private`, tenant.String(), hub); n != 1 {
		t.Errorf("the stored hub is not private (%d rows)", n)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1 AND parent_id = $2 AND name = 'Diary'`, tenant.String(), hub); n != 1 {
		t.Errorf("%d first collections, want one named as the hub", n)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM membership WHERE tenant_id = $1 AND account_id = $2
		AND scope_type = 'HUB' AND scope_id = $3 AND role = 'OWNER'`, tenant.String(), owner.AccountID.String(), hub); n != 1 {
		t.Errorf("%d owner memberships on the hub, want its creator's", n)
	}
	// The container.created event of the hub, the one record that leaves the transaction for the
	// workspace's subscribers, says private from the start.
	if n := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1 AND subject = $2
		AND event_type LIKE '%container.created%' AND (payload ->> 'private')::boolean`, tenant.String(), "container/"+hub); n != 1 {
		t.Errorf("%d private container.created events for the hub, want one", n)
	}

	// The hub part's product is on in the hub, read by its owner.
	modules, err := registry.Invoke(ctx, settingservice.ListModulesName, owner, usecase.Input{"hub_id": hub})
	if err != nil {
		t.Fatalf("the owner reading the hub's modules: %v", err)
	}
	if !moduleOn(modules, "WORKFLOWS") {
		t.Errorf("Workflows is not on in the private hub: %v", modules)
	}

	// Another administrator of the workspace reads not found.
	if _, err := registry.Invoke(ctx, "GetContainer", other, usecase.Input{"container_id": hub}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another administrator reading the private hub answered %v, want not found", err)
	}
}

func moduleOn(answer usecase.Output, key string) bool {
	rows, _ := answer["modules"].([]usecase.Output)
	for _, row := range rows {
		if row["module"] == key {
			on, _ := row["on"].(bool)
			return on
		}
	}
	return false
}

// A refused hub part, private hubs switched off, and a member who may create only private hubs
// each leave no hub, no membership and no module switched in a hub behind them.
func TestAPrivateHubFromATemplateRefusedLeavesNothingInPostgres(t *testing.T) {
	ctx := context.Background()
	body := `{"name":"Diary","template":"kanban_team","private":true}`
	refusing, taking := privateTemplateDesk(t, true), privateTemplateDesk(t, false)
	nothingLeft := func(t *testing.T, tenant shared.ID) {
		t.Helper()
		if n := containersOf(ctx, t, tenant); n != 0 {
			t.Errorf("%d containers left behind", n)
		}
		if n := countIn(ctx, t, `SELECT count(*) FROM membership WHERE tenant_id = $1 AND scope_type = 'HUB'`, tenant.String()); n != 0 {
			t.Errorf("%d hub memberships left behind", n)
		}
	}

	t.Run("a refused hub part", func(t *testing.T) {
		tenant, owner := freshWorkspace(ctx, t, "OWNER")
		code, answer := postHubFromTemplate(ctx, t, refusing, withScopes(owner), body)
		if code != http.StatusUnprocessableEntity {
			t.Errorf("answered %d %v, want the part's refusal", code, answer)
		}
		nothingLeft(t, tenant)
	})
	t.Run("private hubs switched off", func(t *testing.T) {
		tenant, owner := freshWorkspace(ctx, t, "OWNER")
		switchPrivateHubs(ctx, t, tenant, false)
		code, answer := postHubFromTemplate(ctx, t, taking, withScopes(owner), body)
		if code != http.StatusForbidden || answer["detail_code"] != "containers.private_hubs_off" {
			t.Errorf("answered %d %v, want containers.private_hubs_off", code, answer)
		}
		nothingLeft(t, tenant)
	})
	t.Run("a member who may create only private hubs", func(t *testing.T) {
		tenant, _ := freshWorkspace(ctx, t, "OWNER")
		member := withScopes(personIn(ctx, t, tenant, "MEMBER"))
		code, answer := postHubFromTemplate(ctx, t, taking, member, body)
		if code != http.StatusForbidden || answer["detail_code"] != "access.not_permitted" {
			t.Errorf("answered %d %v, want access.not_permitted", code, answer)
		}
		nothingLeft(t, tenant)
		if n := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND actor_id = $2
			AND action = 'container.created' AND outcome = 'DENIED'`, tenant.String(), member.AccountID.String()); n != 1 {
			t.Errorf("%d refusals of the member in the trail, want one", n)
		}
	})
}
