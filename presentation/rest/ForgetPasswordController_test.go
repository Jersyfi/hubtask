// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-ID-04 check 1: the route answers byte for byte the same whatever the use case found behind the
// address - an account, none, a disabled one, a provider-only one. The use case answers each of them
// with nothing (its own tests); what this holds is that the adapter adds nothing either: not a body,
// not a header, not a status drawn from what the use case returned.
func TestTheForgottenPasswordRouteAnswersTheSameWhateverTheUseCaseDid(t *testing.T) {
	forgot := func(address string, out usecase.Output) (*httptest.ResponseRecorder, *catalogue) {
		t.Helper()
		registry := &catalogue{out: out}
		controller := NewRestController()
		controller.UseCases = registry
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			APIBasePath+"/auth/password:forgot", strings.NewReader(`{"email":"`+address+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(TenantHeader, "01936f2a-7c1e-7000-8000-0000000000a0")
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, request)
		return recorder, registry
	}

	held, heldCall := forgot("bert@example.org", usecase.Output{})
	unheld, _ := forgot("nobody@example.org", nil)
	// An output the use case never gives, to show the adapter would not pass it on if it did.
	chatty, _ := forgot("provider-only@example.org", usecase.Output{"queued": true})

	if heldCall.name != forgetPasswordUseCase || heldCall.in["email"] != "bert@example.org" {
		t.Fatalf("the route asked %q with %v", heldCall.name, heldCall.in)
	}
	for name, answer := range map[string]*httptest.ResponseRecorder{"unknown": unheld, "provider-only": chatty} {
		if answer.Code != held.Code || answer.Body.String() != held.Body.String() ||
			!maps.EqualFunc(answer.Header(), held.Header(), func(a, b []string) bool {
				return strings.Join(a, "\n") == strings.Join(b, "\n")
			}) {
			t.Errorf("the %s address answered %d %v %q, a held one %d %v %q", name,
				answer.Code, answer.Header(), answer.Body, held.Code, held.Header(), held.Body)
		}
	}
	if held.Code != http.StatusAccepted || held.Body.Len() != 0 {
		t.Errorf("the route answered %d with %q, want 202 and no body", held.Code, held.Body)
	}
}
