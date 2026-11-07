// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package doors proves what every door of the catalogue answers alike - REST, MCP and a rule's
// action - against the real adapters over one registry, without a database.
package doors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/planning"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

var (
	tenant = shared.MustParseID("018f0000-0000-7000-8000-000000000101")
	office = shared.MustParseID("018f0000-0000-7000-8000-000000000102")
	anna   = shared.MustParseID("018f0000-0000-7000-8000-0000000001a1")
)

type settings struct {
	rows map[settingrepo.Place]map[domain.Key]domain.Entry
}

func (s *settings) Above(context.Context, string) (domain.Layer, domain.Layer, error) {
	return domain.Layer{Level: domain.LevelInstance, Entries: map[domain.Key]domain.Entry{}},
		domain.Layer{Level: domain.LevelPlan, Entries: map[domain.Key]domain.Entry{}}, nil
}

func (s *settings) At(_ context.Context, places []settingrepo.Place, _ string) ([]settingrepo.Stored, error) {
	var out []settingrepo.Stored
	for _, place := range places {
		for key, entry := range s.rows[place] {
			out = append(out, settingrepo.Stored{Place: place, Key: key, Entry: entry})
		}
	}
	return out, nil
}

func (s *settings) Put(_ context.Context, _ shared.ID, place settingrepo.Place, key domain.Key, entry domain.Entry, _ time.Time) error {
	if s.rows[place] == nil {
		s.rows[place] = map[domain.Key]domain.Entry{}
	}
	s.rows[place][key] = entry
	return nil
}

func (s *settings) Remove(_ context.Context, place settingrepo.Place, key domain.Key) (bool, error) {
	_, held := s.rows[place][key]
	delete(s.rows[place], key)
	return held, nil
}

type containers struct{}

func (containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id == office {
		return work.Container{ID: office, TenantID: tenant, Type: work.ContainerHub}, nil
	}
	return work.Container{}, shared.ErrNotFound
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func person() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: anna, AccountName: "Anna Beispiel",
		Scopes: []string{"items:write"},
	}
}

// UC-INS-21 checks 3 and 5: with planning on for the workspace and off for the "Office" hub,
// planning a release of that hub is refused with `module.off` through REST, MCP and a rule's
// action alike, before the use case runs - the gate sits on the registry all three open onto.
func TestAPlanningOperationAHubHasOffIsRefusedThroughEveryDoor(t *testing.T) {
	ctx := context.Background()
	store := &settings{rows: map[settingrepo.Place]map[domain.Key]domain.Entry{}}
	workspace := settingrepo.Place{Level: domain.LevelWorkspace, ScopeID: tenant}
	hub := settingrepo.Place{Level: domain.LevelHub, ScopeID: office}
	store.rows[workspace] = map[domain.Key]domain.Entry{module.Planning.SettingKey(): {Value: true}}
	store.rows[hub] = map[domain.Key]domain.Entry{module.Planning.SettingKey(): {Value: false}}
	modules := settingservice.Modules{Settings: store, Containers: containers{}, UnitOfWork: unitOfWork{}}
	if on, err := modules.On(ctx, person(), ""); err != nil || !on.Has(module.Planning) {
		t.Fatalf("the workspace does not have planning on: %v, %v", on, err)
	}

	// The use case is never built: a refusal that reached it would fail on its missing parts.
	registry, err := usecase.NewRegistry(settingservice.ModuleGate{Modules: modules}.Wrap(nil),
		planning.CreateRelease{}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("REST", func(t *testing.T) {
		controller := rest.NewRestController()
		controller.UseCases = registry
		request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, person()), http.MethodPost,
			rest.APIBasePath+"/hubs/"+office.String()+"/releases",
			strings.NewReader(`{"name":"Autumn","release_on":"2026-11-30"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), module.OffCode) {
			t.Errorf("answered %d: %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("MCP", func(t *testing.T) {
		server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_release",` +
			`"arguments":{"hub_id":"` + office.String() + `","name":"Autumn","release_on":"2026-11-30"}}}`
		request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, person()),
			http.MethodPost, mcp.Path, strings.NewReader(body))
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if !strings.Contains(recorder.Body.String(), `"isError":true`) || !strings.Contains(recorder.Body.String(), module.OffCode) {
			t.Errorf("answered %d: %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("automation", func(t *testing.T) {
		rule := person()
		rule.Kind = appshared.ActorAutomation
		_, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
			Kind:   "CREATE_RELEASE",
			Params: map[string]any{"hub_id": office.String(), "name": "Autumn", "release_on": "2026-11-30"},
		}, nil)
		var refusal *shared.Error
		if !errors.As(err, &refusal) || refusal.DetailCode != module.OffCode {
			t.Errorf("the action answered %v, want %s", err, module.OffCode)
		}
	})
}
