// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// A sign-in begun on the invitation card carries the invitation (ADR-0078 §1, SC-32): the body's
// token reaches the use case under the name its descriptor declares, and a sign-in without one sends
// nothing - an empty token is not an invitation.
func TestTheInvitationTokenReachesTheStart(t *testing.T) {
	cases := []struct {
		name string
		body string
		want any
	}{
		{name: "carried", body: `{"invitation_token":"inv_the-token"}`, want: "inv_the-token"},
		{name: "absent", body: `{}`, want: nil},
		{name: "empty", body: `{"invitation_token":""}`, want: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			registry := &catalogue{out: usecase.Output{
				"authorization_url": "https://login.example.org/authorize", "state": "the-state",
			}}
			controller := NewRestController()
			controller.UseCases = registry
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				APIBasePath+"/auth/oidc:start", strings.NewReader(c.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			controller.Routes().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			if registry.name != startOidcSignInUseCase {
				t.Errorf("use case = %q", registry.name)
			}
			if got := registry.in["invitation_token"]; got != c.want {
				t.Errorf("the invitation reached the use case as %v, want %v", got, c.want)
			}
		})
	}
}

// A sign-in begun on the connect card carries the link a workspace without the password mailed
// (ADR-0078 §1, SC-33), under the name the descriptor declares; an empty token is no link.
func TestTheConnectTokenReachesTheStart(t *testing.T) {
	cases := []struct {
		name string
		body string
		want any
	}{
		{name: "carried", body: `{"connect_token":"hbt_mfa_the-link"}`, want: "hbt_mfa_the-link"},
		{name: "absent", body: `{}`, want: nil},
		{name: "empty", body: `{"connect_token":""}`, want: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			registry := &catalogue{out: usecase.Output{
				"authorization_url": "https://login.example.org/authorize", "state": "the-state",
			}}
			controller := NewRestController()
			controller.UseCases = registry
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				APIBasePath+"/auth/oidc:start", strings.NewReader(c.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			controller.Routes().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			if got := registry.in["connect_token"]; got != c.want {
				t.Errorf("the link reached the use case as %v, want %v", got, c.want)
			}
		})
	}
}
