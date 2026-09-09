// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func storedWorkspace() usecase.Output {
	return usecase.Output{
		"id":                 "0192f000-0000-7000-8000-00000000000a",
		"slug":               "acme",
		"display_name":       "Acme",
		"status":             "ACTIVE",
		"default_locale":     "en",
		"default_time_zone":  "UTC",
		"require_admin_totp": false,
		"created_at":         "2026-01-02T03:04:05Z",
		"version":            4,
	}
}

func workspaceRequest(
	t *testing.T, registry UseCaseRegistry, method, body string, ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID(signedInAccount),
	})

	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+"/tenant", strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/merge-patch+json")
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// The read the administration area opens with, and the ETag a later write guards on.
func TestReadingTheWorkspaceAnswersItsConfigurationAndAnETag(t *testing.T) {
	registry := &catalogue{out: storedWorkspace()}

	recorder := workspaceRequest(t, registry, http.MethodGet, "", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != readWorkspaceUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if len(registry.in) != 0 {
		t.Errorf("input = %v, want none - the actor names the workspace", registry.in)
	}
	if got := recorder.Header().Get("ETag"); got != `"4"` {
		t.Errorf("ETag = %q, want the version", got)
	}

	var body struct {
		Slug             string `json:"slug"`
		DisplayName      string `json:"display_name"`
		RequireAdminTotp bool   `json:"require_admin_totp"`
		Version          int    `json:"version"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Slug != "acme" || body.DisplayName != "Acme" || body.Version != 4 {
		t.Errorf("the answer is %+v", body)
	}
	if body.RequireAdminTotp {
		t.Error("the enforcement switch came back on")
	}
}

// A merge-patch sends only what moves, and only what it sent reaches the catalogue: a key the
// client left out must not arrive as an empty string, or the workspace loses its locale on a
// rename.
func TestOnlyWhatThePatchSentReachesTheCatalogue(t *testing.T) {
	out := storedWorkspace()
	out["display_name"] = "Acme GmbH"
	out["version"] = 5
	registry := &catalogue{out: out}

	recorder := workspaceRequest(t, registry, http.MethodPatch,
		`{"display_name": "Acme GmbH"}`, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != updateWorkspaceUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if registry.in["display_name"] != "Acme GmbH" {
		t.Errorf("display_name reached the catalogue as %v", registry.in["display_name"])
	}
	for _, absent := range []string{"default_locale", "default_time_zone"} {
		if value := registry.in[absent]; value != nil {
			t.Errorf("%s reached the catalogue as %v, want absent", absent, value)
		}
	}
	if _, held := registry.in["require_admin_totp"]; held {
		t.Error("the enforcement switch reached the catalogue although the client sent none")
	}
	if _, held := registry.in["expected_version"]; held {
		t.Error("a version reached the catalogue without an If-Match")
	}
	if got := recorder.Header().Get("ETag"); got != `"5"` {
		t.Errorf("ETag = %q, want the new version", got)
	}
}

// False is a value this field holds, so a client switching enforcement off has to be told apart
// from one that did not mention it.
func TestSwitchingEnforcementOffIsNotTheSameAsNotMentioningIt(t *testing.T) {
	registry := &catalogue{out: storedWorkspace()}

	workspaceRequest(t, registry, http.MethodPatch, `{"require_admin_totp": false}`, "")

	value, held := registry.in["require_admin_totp"]
	if !held {
		t.Fatal("the switch did not reach the catalogue at all")
	}
	if value != false {
		t.Errorf("the switch reached the catalogue as %v", value)
	}
}

// An If-Match is the version the caller last read, and it is what the write guards on.
func TestAnIfMatchBecomesTheExpectedVersion(t *testing.T) {
	registry := &catalogue{out: storedWorkspace()}

	workspaceRequest(t, registry, http.MethodPatch, `{"display_name": "Acme GmbH"}`, `"4"`)

	if registry.in["expected_version"] != 4 {
		t.Errorf("expected_version = %v, want 4", registry.in["expected_version"])
	}
}

// The slug is not settable here, and a body naming it is refused rather than accepted and
// quietly ignored - which is what the contract promises.
func TestNamingTheSlugIsRefusedRatherThanIgnored(t *testing.T) {
	registry := &catalogue{out: storedWorkspace()}

	recorder := workspaceRequest(t, registry, http.MethodPatch, `{"slug": "acme-gmbh"}`, "")

	if recorder.Code != http.StatusUnprocessableEntity && recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want a refusal: %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("the use case ran for a body naming the slug")
	}
	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if len(problem.FieldErrors) != 1 || problem.FieldErrors[0].Path != "/slug" {
		t.Errorf("the refusal points at %v, want /slug", problem.FieldErrors)
	}
}
