// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package doors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

var depot = shared.MustParseID("018f0000-0000-7000-8000-000000000103")

type hubs struct{}

func (hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{ID: id, TenantID: tenant, Type: work.ContainerHub}, nil
}

type allowed struct{}

func (allowed) Authorize(context.Context, appshared.ActorContext, access.Request) error { return nil }

type silent struct{}

func (silent) Append(context.Context, audit.Entry) error { return nil }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC) }

type oneID struct{}

func (oneID) NewID() shared.ID { return shared.MustParseID("018f0000-0000-7000-8000-0000000001ff") }

// switching is a workspace with two hubs and the switch on its registry.
func switching(t *testing.T) (*usecase.Registry, *settings) {
	t.Helper()
	store := &settings{rows: map[settingrepo.Place]map[domain.Key]domain.Entry{}, hubs: []shared.ID{office, depot}}
	modules := settingservice.Modules{
		Settings: store, Containers: hubs{}, Authorizer: allowed{}, Audit: silent{},
		UnitOfWork: unitOfWork{}, Clock: fixedClock{}, IDs: oneID{},
	}
	registry, err := usecase.NewRegistry(nil, settingservice.SetModule{Modules: modules}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return registry, store
}

func throughREST(t *testing.T, registry *usecase.Registry, body string) (int, string) {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(context.Background(), person()), http.MethodPut,
		rest.APIBasePath+"/tenant/modules/PLANNING", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func throughMCP(t *testing.T, registry *usecase.Registry, arguments string) string {
	t.Helper()
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_module","arguments":` + arguments + `}}`
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(context.Background(), person()),
		http.MethodPost, mcp.Path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder.Body.String()
}

// planningIn is whether the hub stores planning on as its own value.
func planningIn(store *settings, hub shared.ID) any {
	return store.rows[settingrepo.Place{Level: domain.LevelHub, ScopeID: hub}][module.Planning.SettingKey()].Value
}

// UC-INS-21 check 13: the hubs chosen with a workspace's switch reach the use case through REST
// and through the registry's other door alike - the chosen hub on, the other left off, and the
// same refusal where the hubs come with anything but switching on.
func TestTheChosenHubsAnswerTheSameThroughEveryDoor(t *testing.T) {
	chosen := `["` + office.String() + `"]`

	restRegistry, restStore := switching(t)
	status, answer := throughREST(t, restRegistry, `{"switch":"ON","hub_ids":`+chosen+`}`)
	if status != http.StatusOK {
		t.Fatalf("REST answered %d: %s", status, answer)
	}
	var listed struct {
		Modules []struct {
			Module string `json:"module"`
			On     bool   `json:"on"`
		} `json:"modules"`
	}
	if err := json.Unmarshal([]byte(answer), &listed); err != nil {
		t.Fatal(err)
	}
	on := false
	for _, row := range listed.Modules {
		on = on || row.Module == string(module.Planning) && row.On
	}
	if !on {
		t.Errorf("REST does not answer planning on at the workspace: %s", answer)
	}

	mcpRegistry, mcpStore := switching(t)
	if answer := throughMCP(t, mcpRegistry, `{"module":"PLANNING","switch":"ON","hub_ids":`+chosen+`}`); strings.Contains(answer, `"isError":true`) {
		t.Fatalf("MCP answered %s", answer)
	}

	for door, store := range map[string]*settings{"REST": restStore, "MCP": mcpStore} {
		if planningIn(store, office) != true || planningIn(store, depot) != false {
			t.Errorf("%s: the chosen hub stores %v, the other %v", door, planningIn(store, office), planningIn(store, depot))
		}
	}
	if !reflect.DeepEqual(restStore.rows, mcpStore.rows) {
		t.Errorf("the doors stored differently:\nREST %v\nMCP  %v", restStore.rows, mcpStore.rows)
	}

	const refusal = "module.hubs_with_on_only"
	restRegistry, restStore = switching(t)
	if status, answer := throughREST(t, restRegistry, `{"switch":"OFF","hub_ids":`+chosen+`}`); status != http.StatusUnprocessableEntity || !strings.Contains(answer, refusal) {
		t.Errorf("REST answered %d: %s", status, answer)
	}
	mcpRegistry, mcpStore = switching(t)
	if answer := throughMCP(t, mcpRegistry, `{"module":"PLANNING","switch":"OFF","hub_ids":`+chosen+`}`); !strings.Contains(answer, `"isError":true`) || !strings.Contains(answer, refusal) {
		t.Errorf("MCP answered %s", answer)
	}
	if len(restStore.rows) != 0 || len(mcpStore.rows) != 0 {
		t.Errorf("a refused switch stored %v and %v", restStore.rows, mcpStore.rows)
	}
}
