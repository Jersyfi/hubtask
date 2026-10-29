// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func callDoor(t *testing.T, registry UseCaseRegistry, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, reader)
	for name, value := range header {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// A generated mapping hands the path, the body's members and the version read to the use case by
// the contract's names, and answers with its output and its version as the ETag.
func TestAGeneratedOperationReachesItsUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"key": "reviewer", "name": "Reviewer", "permissions": []string{"READ"}, "version": 4}}

	recorder := callDoor(t, registry, http.MethodPatch, "/roles/reviewer", `{"name":"Reviewer","permissions":["READ"]}`,
		map[string]string{"Content-Type": "application/merge-patch+json", "If-Match": `"3"`})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ChangeRole" {
		t.Errorf("the catalogue was asked for %q", registry.name)
	}
	if registry.in["key"] != "reviewer" || registry.in["name"] != "Reviewer" || registry.in["expected_version"] != 3 {
		t.Errorf("unexpected input: %v", registry.in)
	}
	if permissions, _ := registry.in["permissions"].([]any); len(permissions) != 1 || permissions[0] != "READ" {
		t.Errorf("the permissions arrived as %#v", registry.in["permissions"])
	}
	if tag := recorder.Header().Get("ETag"); tag != `"4"` {
		t.Errorf("ETag %q", tag)
	}
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer["key"] != "reviewer" {
		t.Errorf("the answer is %s", recorder.Body)
	}
}

func TestAGeneratedOperationReadsItsQuery(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"roles": []any{}}}
	account := "0192f000-0000-7000-8000-00000000000e"

	recorder := callDoor(t, registry, http.MethodGet, "/access/explain?scope_type=WORKSPACE&account_id="+account, "", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.in["scope_type"] != "WORKSPACE" || registry.in["account_id"] != account {
		t.Errorf("unexpected input: %v", registry.in)
	}
	if _, sent := registry.in["permission"]; sent {
		t.Errorf("a parameter nobody sent arrived: %v", registry.in)
	}
}

func TestAGeneratedRemovalAnswers204(t *testing.T) {
	registry := &catalogue{out: usecase.Output{}}
	grant := "0192f000-0000-7000-8000-0000000000aa"

	recorder := callDoor(t, registry, http.MethodDelete, "/role-grants/"+grant, "", nil)

	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "RevokeRoleGrant" || registry.in["grant_id"] != grant {
		t.Errorf("%s %v", registry.name, registry.in)
	}
}

// A body the contract refuses is refused as a hand-written handler refuses it, before the use case.
func TestAGeneratedOperationRefusesAnUnknownMember(t *testing.T) {
	registry := &catalogue{}

	recorder := callDoor(t, registry, http.MethodPost, "/link-types", `{"key":"blocks","name":"Blocks","colour":"red"}`,
		map[string]string{"Content-Type": "application/json"})

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("the use case ran for a body the contract refuses")
	}
}

// Every generated mapping names a use case of the catalogue, and every query parameter and the
// version it passes is an input that use case declares (F-7: the doors come from the descriptors).
func TestEveryGeneratedMappingIsADescriptors(t *testing.T) {
	declared := map[string]map[string]bool{}
	for _, descriptor := range usecases.Descriptors() {
		fields := map[string]bool{}
		for _, field := range descriptor.Input {
			fields[field.Name] = true
		}
		declared[descriptor.Name] = fields
	}
	for operation, door := range restDoors {
		fields, ok := declared[door.UseCase]
		if !ok {
			t.Errorf("%s maps to %s, which the catalogue does not have", operation, door.UseCase)
			continue
		}
		for _, name := range door.Query {
			if !fields[name] {
				t.Errorf("%s passes %s, which %s does not declare", operation, name, door.UseCase)
			}
		}
		if door.IfMatch && !fields["expected_version"] {
			t.Errorf("%s passes expected_version, which %s does not declare", operation, door.UseCase)
		}
	}
}
