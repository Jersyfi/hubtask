// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	usecase "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogueusecase "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type manifestUnitOfWork struct{}

func (manifestUnitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (manifestUnitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type manifestProfiles []work.CapabilityProfile

func (p manifestProfiles) List(context.Context) ([]work.CapabilityProfile, error) { return p, nil }
func (p manifestProfiles) ListSystem(context.Context) ([]work.CapabilityProfile, error) {
	return p, nil
}

type manifestLanguages []string

func (l manifestLanguages) List(context.Context) ([]string, error) { return l, nil }

type manifestHeld []service.Permission

func (h manifestHeld) HeldOnWorkspace(context.Context, appshared.ActorContext) ([]service.Permission, error) {
	return h, nil
}

type manifestLeftOut entitlement.LeftOut

func (l manifestLeftOut) LeftOut(context.Context, appshared.ActorContext) (entitlement.LeftOut, error) {
	return entitlement.LeftOut(l), nil
}

type manifestModules module.Set

func (m manifestModules) On(context.Context, appshared.ActorContext, shared.ID) (module.Set, error) {
	return module.Set(m), nil
}

// manifestReader is the real use case, its maps filled in the order given: the declarations the
// composition root hands in from the catalogue are maps, and a map's order is chance.
func manifestReader(kinds []string) usecase.GetCapabilities {
	fields := map[string][]catalogueusecase.Field{}
	summaries := map[string]string{}
	entitlements := map[string]entitlement.Feature{}
	for _, kind := range kinds {
		fields[kind] = []catalogueusecase.Field{
			{Name: "title", Kind: catalogueusecase.KindString, Required: true},
			{Name: "note", Kind: catalogueusecase.KindString, Description: "the " + kind},
		}
		summaries[kind] = "does " + kind
		entitlements[kind] = entitlement.Webhooks
	}
	return usecase.GetCapabilities{
		Profiles: manifestProfiles{
			{Type: work.ItemTask, Capabilities: []work.Capability{work.CapabilityCompletion}, MaxDepth: 3},
			{Type: work.ItemActivity, Capabilities: []work.Capability{work.CapabilityCompletion}, MaxDepth: 1},
		},
		Languages:          manifestLanguages{"de", "en"},
		UnitOfWork:         manifestUnitOfWork{},
		Workspace:          manifestHeld{service.PermissionRead, service.PermissionStructure},
		Entitlements:       manifestLeftOut{entitlement.AutomationRules},
		Modules:            manifestModules{module.Workflows, module.Planning},
		Actions:            []string{"item.create", "item.update", "webhook.send"},
		ActionFields:       fields,
		ActionSummaries:    summaries,
		ActionEntitlements: entitlements,
		Scopes:             []string{"items:read", "items:write"},
		Config: env.Config{
			Version: "1.2.3",
			Tenancy: env.TenancySingle,
			Request: env.RequestConfig{MaxBodyBytes: 1 << 20, MaxUploadBytes: 1 << 26},
		},
	}
}

// Nothing in the manifest's answer is ordered by chance: the real use case, read again and again
// for one signed-in caller with its declarations' maps filled in another order each time, answers
// the same bytes and therefore the same tag - which is what lets a client's copy revalidate
// (UC-SYN-01/8).
func TestTheRealManifestAnswersTheSameBytesWhateverOrderItsMapsWereFilledIn(t *testing.T) {
	actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: shared.ID("018f2a1b-0000-7000-8000-0000000000ab")}
	read := func(reader CapabilityReader) *httptest.ResponseRecorder {
		controller := NewRestController()
		controller.Meta = &MetaDoor{Capabilities: reader}
		r := httptest.NewRequestWithContext(appshared.ContextWithActor(t.Context(), actor),
			http.MethodGet, APIBasePath+"/meta/capabilities", nil)
		response := httptest.NewRecorder()
		controller.Routes().ServeHTTP(response, r)
		if response.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", response.Code, response.Body.String())
		}
		return response
	}

	orders := [][]string{
		{"item.create", "item.update", "webhook.send"},
		{"webhook.send", "item.update", "item.create"},
		{"item.update", "webhook.send", "item.create"},
	}
	first := read(manifestReader(orders[0]))
	if first.Body.Len() < 1000 || !containsAll(first.Body.String(), `"does webhook.send"`, `"WORKFLOWS"`, `"AUTOMATION_RULES"`) {
		t.Fatalf("the manifest did not carry the signed-in caller's declarations: %s", first.Body.String())
	}
	for round := range 20 {
		again := read(manifestReader(orders[round%len(orders)]))
		if again.Body.String() != first.Body.String() {
			t.Fatalf("read %d answered other bytes than the first", round+2)
		}
		if got, want := again.Header().Get("ETag"), first.Header().Get("ETag"); got != want {
			t.Fatalf("read %d tagged %s, the first %s", round+2, got, want)
		}
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
