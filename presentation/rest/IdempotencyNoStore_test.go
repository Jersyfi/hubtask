// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
)

// An answer marked as shown once is not kept for a replay: a start password, a token or any other
// credential shown once would otherwise sit in clear in the idempotency record. The key is still
// completed with the status, so a repeat runs nothing twice - it answers the status with no body.
// The mark never reaches the client.
func TestAnAnswerShownOnceKeepsNoBody(t *testing.T) {
	g := &guard{}
	secret := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set(ShownOnceHeader, "start_password")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"start_password":"shown-once"}`))
	}

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{"name":"a"}`, true), secret)

	if response.Code != http.StatusCreated || response.Body.String() != `{"start_password":"shown-once"}` {
		t.Fatalf("the client got %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get(ShownOnceHeader) != "" {
		t.Error("the internal mark reached the client")
	}
	if !g.completed || g.gotStatus != http.StatusCreated {
		t.Errorf("the key was completed %v with %d, want the status kept", g.completed, g.gotStatus)
	}
	if len(g.gotBody) != 0 {
		t.Errorf("the record keeps %q", g.gotBody)
	}
}

// Every JSON answer is `no-store` for caches; that alone keeps the replay's body, which is what makes
// a repeated write answer what the first one answered.
func TestAnOrdinaryNoStoreAnswerIsStillKept(t *testing.T) {
	g := &guard{}
	ordinary := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"018f0000-0000-7000-8000-000000000001"}`))
	}

	serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{}`, true), ordinary)

	if string(g.gotBody) != `{"id":"018f0000-0000-7000-8000-000000000001"}` {
		t.Errorf("the record keeps %q, want the answer", g.gotBody)
	}
}

// Every answer of a route that always carries a credential shown once keeps no body, without the
// handler marking it: one route of each class, at the template the generated router registers
// (the contract test holds every entry of ShownOnceRoutes to those templates).
func TestAShownOnceRouteKeepsNoBody(t *testing.T) {
	cases := []struct {
		class, template, path, answer string
	}{
		{"a session's tokens", "/auth/sessions:refresh", "/auth/sessions:refresh",
			`{"access_token":"hbt_at_x","refresh_token":"hbt_rt_x"}`},
		{"a personal access token", "/auth/tokens", "/auth/tokens", `{"id":"a","token":"hbt_pat_x"}`},
		{"a rule's inbound token", "/automation/rules/{ruleId}:rotate-inbound-token",
			"/automation/rules/018f0000-0000-7000-8000-000000000001:rotate-inbound-token", `{"token":"x"}`},
		{"a webhook secret", "/integrations/webhooks/{webhookId}:rotate-secret",
			"/integrations/webhooks/018f0000-0000-7000-8000-000000000001:rotate-secret", `{"secret":"whsec_x"}`},
		{"an OAuth client secret", "/oauth/clients", "/oauth/clients", `{"client_secret":"hbt_ocs_x"}`},
		{"recovery codes", "/auth/mfa/recovery:regenerate", "/auth/mfa/recovery:regenerate",
			`{"recovery_codes":["a","b"]}`},
		{"a start password", "/accounts/{accountId}:issue-start-password",
			"/accounts/018f0000-0000-7000-8000-000000000001:issue-start-password", `{"start_password":"x"}`},
	}
	for _, c := range cases {
		t.Run(c.class, func(t *testing.T) {
			template := http.MethodPost + " " + APIBasePath + c.template
			if !ShownOnceRoutes[template] {
				t.Fatalf("%s is not in ShownOnceRoutes", template)
			}
			routes := NewMux()
			routes.HandleFunc(template, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(c.answer))
			})
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+c.path, strings.NewReader(`{}`))
			r.Header.Set(IdempotencyKeyHeader, idempotencyKey)
			r = r.WithContext(appshared.ContextWithActor(r.Context(),
				appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantID}))
			g := &guard{}
			response := httptest.NewRecorder()
			Idempotent{Guard: g, Routes: routes, Next: routes}.ServeHTTP(response, r)

			if response.Code != http.StatusCreated || response.Body.String() != c.answer {
				t.Fatalf("the client got %d %q", response.Code, response.Body.String())
			}
			if g.beganWith.Endpoint != template {
				t.Errorf("the key was reserved for %q", g.beganWith.Endpoint)
			}
			if !g.completed || g.gotStatus != http.StatusCreated || len(g.gotBody) != 0 {
				t.Errorf("the record keeps %v %d %q, want the status alone", g.completed, g.gotStatus, g.gotBody)
			}
		})
	}
}
