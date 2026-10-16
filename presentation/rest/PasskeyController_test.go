// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	identity "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The passkey routes as a client reaches them (identity.md §23): what each sends the registry is
// exactly what its descriptor declares - an undeclared key is refused there - and what comes back
// reaches the body through the hand-written projection.

const aPasskeyID = "0192f000-0000-7000-8000-0000000000c1"

// declaredInputs is the descriptor's input, by name.
func declaredInputs(descriptor usecase.Descriptor) []string {
	names := make([]string, 0, len(descriptor.Input))
	for _, field := range descriptor.Input {
		names = append(names, field.Name)
	}
	sort.Strings(names)
	return names
}

func sentInputs(in usecase.Input) []string {
	names := make([]string, 0, len(in))
	for name := range in {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestRenamingAPasskeySendsWhatTheUseCaseDeclaresAndAnswersThePasskey(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"passkey": usecase.Output{
		"id": aPasskeyID, "name": "Laptop at work", "created_at": time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC),
		"rp_id": "acme.hubtask.example",
	}}}
	recorder := identityRequestWithBody(t, registry, http.MethodPatch, "/accounts/me/passkeys/"+aPasskeyID,
		`{"name":"Laptop at work"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != identity.RenameOwnPasskeyName || registry.in.String("passkey_id") != aPasskeyID ||
		registry.in.String("name") != "Laptop at work" {
		t.Errorf("invoked %s with %v", registry.name, registry.in)
	}
	want, got := declaredInputs(identity.RenameOwnPasskey{}.Descriptor()), sentInputs(registry.in)
	if len(want) != len(got) || want[0] != got[0] || want[1] != got[1] {
		t.Errorf("sent %v, the use case declares %v", got, want)
	}
	var body openapi.Passkey
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Laptop at work" || body.RpId == nil || *body.RpId != "acme.hubtask.example" {
		t.Errorf("answered %s", recorder.Body)
	}

	malformed := identityRequestWithBody(t, &catalogue{}, http.MethodPatch, "/accounts/me/passkeys/not-a-uuid",
		`{"name":"x"}`)
	if malformed.Code != http.StatusBadRequest {
		t.Errorf("a malformed id answered %d", malformed.Code)
	}
}

func TestRemovingAPasskeySendsTheProofAndAnswersNoContent(t *testing.T) {
	registry := &catalogue{out: usecase.Output{}}
	controller := NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(identityContext(t), http.MethodDelete,
		APIBasePath+"/accounts/me/passkeys/"+aPasskeyID, nil)
	request.Header.Set("X-Hubtask-Step-Up", "hbt_sup_proof")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.name != identity.RemoveOwnPasskeyName || registry.in.String("passkey_id") != aPasskeyID ||
		registry.in.String("step_up_token") != "hbt_sup_proof" {
		t.Errorf("invoked %s with %v", registry.name, registry.in)
	}
	want, got := declaredInputs(identity.RemoveOwnPasskey{}.Descriptor()), sentInputs(registry.in)
	if strings.Join(want, ",") != strings.Join(got, ",") {
		t.Errorf("sent %v, the use case declares %v", got, want)
	}
}
