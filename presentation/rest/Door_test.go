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
		if door.IfMatch && door.IfMatchAs == "" && !fields["expected_version"] {
			t.Errorf("%s passes expected_version, which %s does not declare", operation, door.UseCase)
		}
		if door.IfMatchAs != "" && !fields[door.IfMatchAs] {
			t.Errorf("%s passes %s, which %s does not declare", operation, door.IfMatchAs, door.UseCase)
		}
		if door.StepUp && !fields["step_up_token"] {
			t.Errorf("%s passes step_up_token, which %s does not declare", operation, door.UseCase)
		}
	}
}

// A workflow's shape reaches the use case as the document it was sent as, and the answer passes
// through the contract's type: a member the contract does not declare is not written.
func TestAGeneratedAnswerHoldsToTheContractsType(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"key": "triage", "name": "Triage", "version": 1, "internal_note": "not in the contract",
	}}

	recorder := callDoor(t, registry, http.MethodPost, "/workflows",
		`{"key":"triage","name":"Triage","shape":{"statuses":[{"key":"open","name":"Open","category":"TODO"}],"initial":"open","steps":[]}}`,
		map[string]string{"Content-Type": "application/json"})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if shape, _ := registry.in["shape"].(map[string]any); shape["initial"] != "open" {
		t.Errorf("the shape arrived as %#v", registry.in["shape"])
	}
	if strings.Contains(recorder.Body.String(), "internal_note") {
		t.Errorf("an undeclared member was written: %s", recorder.Body)
	}
	if tag := recorder.Header().Get("ETag"); tag != `"1"` {
		t.Errorf("ETag %q", tag)
	}
}

// Publishing without a body sends no mapping, which the use case reads as none.
func TestAGeneratedActionTakesAnOptionalBody(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"key": "triage", "name": "Triage", "version": 2}}

	recorder := callDoor(t, registry, http.MethodPost, "/workflows/triage:publish", "", map[string]string{"If-Match": `"1"`})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "PublishWorkflow" || registry.in["key"] != "triage" || registry.in["expected_version"] != 1 {
		t.Errorf("%s %v", registry.name, registry.in)
	}
}

// The configuration document's verbs: a version named by number in the path, a hub by the query,
// and a document handed on as it was sent.
func TestTheConfigurationVerbsReachTheirUseCases(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"version": 7, "hash": "h", "plan_hash": "p", "actor_kind": "USER",
		"added": 0, "changed": 1, "removed": 0, "created_at": "2026-10-29T09:00:00Z"}}
	if recorder := callDoor(t, registry, http.MethodGet, "/config/versions/7", "", nil); recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ReadConfigVersion" || registry.in["version"] != 7 {
		t.Errorf("%s %v", registry.name, registry.in)
	}

	hub := "0192f000-0000-7000-8000-0000000000c1"
	registry.out = usecase.Output{"data": []usecase.Output{}}
	if recorder := callDoor(t, registry, http.MethodGet, "/config/versions?hub_id="+hub, "", nil); recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ListConfigVersions" || registry.in["hub_id"] != hub {
		t.Errorf("%s %v", registry.name, registry.in)
	}

	registry.out = usecase.Output{"base_hash": "b", "changes": []any{}, "refusals": []any{}, "operations": []any{},
		"counts": map[string]any{"added": 0, "changed": 0, "removed": 0}, "flags": map[string]any{}}
	recorder := callDoor(t, registry, http.MethodPost, "/config:plan", `{"document":{"modules":{"workflows":{"on":true}}}}`,
		map[string]string{"Content-Type": "application/json"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	document, _ := registry.in["document"].(map[string]any)
	if registry.name != "PlanConfig" || document["modules"] == nil {
		t.Errorf("%s %v", registry.name, registry.in)
	}
}

// Applying a plan: the hash it was planned on arrives from If-Match, the proof from its header,
// and the use case decides both.
func TestApplyingAPlanPassesTheBaseAndTheProof(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"state": "APPLIED"}}
	recorder := callDoor(t, registry, http.MethodPost, "/config:apply", `{"plan_hash":"p"}`, map[string]string{
		"Content-Type": "application/json", "If-Match": `W/"abc"`, "X-Hubtask-Step-Up": "proof",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.in["base_hash"] != "abc" || registry.in["step_up_token"] != "proof" || registry.in["plan_hash"] != "p" {
		t.Errorf("unexpected input: %v", registry.in)
	}
	if _, sent := registry.in["expected_version"]; sent {
		t.Errorf("If-Match also arrived as a version: %v", registry.in)
	}
}

func TestTheExportAnswersItsHashAsTheETag(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"hash": "h1", "document": map[string]any{}}}
	recorder := callDoor(t, registry, http.MethodGet, "/config", "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if tag := recorder.Header().Get("ETag"); tag != `"h1"` {
		t.Errorf("ETag %q", tag)
	}
}

// A module switched for a hub: the module from the path as its text, the switch and the hub from
// the body; an unlocked module's answer carries no lock.
func TestSwitchingAModuleReachesItsUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"modules": []usecase.Output{{"module": "WORKFLOWS", "on": true, "source": "HUB"}}}}
	hub := "0192f000-0000-7000-8000-0000000000c1"
	recorder := callDoor(t, registry, http.MethodPut, "/tenant/modules/WORKFLOWS", `{"switch":"ON","hub_id":"`+hub+`"}`,
		map[string]string{"Content-Type": "application/json"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "SetModule" || registry.in["module"] != "WORKFLOWS" || registry.in["switch"] != "ON" || registry.in["hub_id"] != hub {
		t.Errorf("%s %v", registry.name, registry.in)
	}
	if strings.Contains(recorder.Body.String(), "locked_by") {
		t.Errorf("an unlocked module answers a lock: %s", recorder.Body)
	}
}

// A type created with its behaviours: the members reach the use case by the contract's names, and
// a system type's missing name is answered as null, as before.
func TestCreatingAnEntryTypeReachesItsUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"key": "bug", "level": "TASK", "system": false, "name": nil,
		"icon": "bug", "color_token": "red", "allowed_children": []string{}, "position": 0, "version": 1}}
	recorder := callDoor(t, registry, http.MethodPost, "/entry-types",
		`{"key":"bug","level":"TASK","name":"Bug","icon":"bug","color_token":"red","record":false,"allowed_children":["sub"]}`,
		map[string]string{"Content-Type": "application/json"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "CreateEntryType" || registry.in["level"] != "TASK" || registry.in["record"] != false {
		t.Errorf("%s %v", registry.name, registry.in)
	}
	if children, _ := registry.in["allowed_children"].([]any); len(children) != 1 {
		t.Errorf("allowed_children arrived as %#v", registry.in["allowed_children"])
	}
	if !strings.Contains(recorder.Body.String(), `"name":null`) || !strings.Contains(recorder.Body.String(), `"allowed_children":[]`) {
		t.Errorf("the answer is %s", recorder.Body)
	}
}

// A type's form is the whole body, handed on as one document; evaluating it names the type by the
// path under the input's own name.
func TestAFormIsHandedOnWhole(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"key": "bug", "level": "TASK", "name": "Bug", "allowed_children": []string{}, "version": 2}}
	recorder := callDoor(t, registry, http.MethodPut, "/entry-types/bug/form", `{"sections":[{"key":"triage","name":"Triage"}]}`,
		map[string]string{"Content-Type": "application/json", "If-Match": `"1"`})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	form, _ := registry.in["form"].(map[string]any)
	if registry.in["key"] != "bug" || form["sections"] == nil || registry.in["expected_version"] != 1 {
		t.Errorf("unexpected input: %v", registry.in)
	}
	if _, spread := registry.in["sections"]; spread {
		t.Errorf("the form was spread into the input: %v", registry.in)
	}

	registry.out = usecase.Output{"entry_type": "bug", "channel": "API", "closed": false, "failed": []string{}, "fields": map[string]any{}}
	recorder = callDoor(t, registry, http.MethodPost, "/entry-types/bug/form:evaluate", `{"title":"x"}`,
		map[string]string{"Content-Type": "application/json"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "EvaluateEntryForm" || registry.in["entry_type"] != "bug" || registry.in["title"] != "x" {
		t.Errorf("%s %v", registry.name, registry.in)
	}
}
