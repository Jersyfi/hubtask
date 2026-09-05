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

const signedInAccount = "0192f000-0000-7000-8000-0000000000e1"

func ownAccount() usecase.Output {
	return usecase.Output{
		"id":           signedInAccount,
		"kind":         "USER",
		"display_name": "Anna Beispiel",
		"status":       "ACTIVE",
		"email":        "anna@example.org",
		"locale":       "de",
		"time_zone":    "Europe/Berlin",
		"week_start":   "MONDAY",
	}
}

func identityRequest(
	t *testing.T, registry UseCaseRegistry, method, path string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID(signedInAccount),
	})

	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(""))
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// The read a client makes before it can render anything: who am I, and in which language, time
// zone and week shape do you speak to me.
func TestReadingTheOwnAccountAnswersTheActorsPreferences(t *testing.T) {
	registry := &catalogue{out: ownAccount()}

	recorder := identityRequest(t, registry, http.MethodGet, "/accounts/me")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != getOwnAccountUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	// No input at all: the actor is the identifier, and a field for it would be a field a caller
	// could get wrong.
	if len(registry.in) != 0 {
		t.Errorf("input = %v, want none", registry.in)
	}

	var body struct {
		ID        string  `json:"id"`
		Locale    *string `json:"locale"`
		TimeZone  *string `json:"time_zone"`
		WeekStart *string `json:"week_start"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.ID != signedInAccount {
		t.Errorf("id = %q, want %q", body.ID, signedInAccount)
	}
	if body.Locale == nil || *body.Locale != "de" {
		t.Errorf("locale = %v, want de", body.Locale)
	}
	if body.TimeZone == nil || *body.TimeZone != "Europe/Berlin" {
		t.Errorf("time_zone = %v, want Europe/Berlin", body.TimeZone)
	}
	if body.WeekStart == nil || *body.WeekStart != "MONDAY" {
		t.Errorf("week_start = %v, want MONDAY", body.WeekStart)
	}
}

// Absent rather than empty, which is the distinction the whole preference chain rests on: a client
// tells "inherits the workspace default" from "set to nothing", and only one of those is a thing a
// person chose.
func TestAnUnsetPreferenceIsAbsentRatherThanEmpty(t *testing.T) {
	out := ownAccount()
	delete(out, "locale")
	delete(out, "time_zone")
	delete(out, "week_start")

	recorder := identityRequest(t, &catalogue{out: out}, http.MethodGet, "/accounts/me")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	for _, field := range []string{`"locale"`, `"time_zone"`, `"week_start"`} {
		if strings.Contains(recorder.Body.String(), field) {
			t.Errorf("%s is present in %s, want it absent", field, recorder.Body)
		}
	}
}

// `me` is a reserved segment and not a possible identifier - identifiers are UUIDs - so the two
// routes cannot collide. Asserted rather than assumed, because the day somebody widens the
// AccountId schema to a plain string is the day this stops being true silently.
func TestMeIsNotReadAsAnAccountIdentifier(t *testing.T) {
	registry := &catalogue{out: ownAccount()}

	identityRequest(t, registry, http.MethodGet, "/accounts/me")

	if registry.name == updateAccountPreferencesUseCase {
		t.Fatal("/accounts/me was routed to the preferences path")
	}
	if _, taken := registry.in["account_id"]; taken {
		t.Error("`me` was passed on as an account identifier")
	}
}

// The read that turns an identifier into a name, and the one thing worth asserting at this layer:
// what the route answers is `AccountSummary` and not `Account`. The catalogue here deliberately
// hands back the *wide* projection - which is what a mapping built by clearing fields would happily
// pass on - so the test proves the narrow mapping rather than a narrow fixture.
func TestReadingAnotherAccountAnswersANameAndNotAnEmail(t *testing.T) {
	registry := &catalogue{out: ownAccount()}
	other := "0192f000-0000-7000-8000-0000000009aa"

	recorder := identityRequest(t, registry, http.MethodGet, "/accounts/"+other)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != getAccountUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if registry.in.String("account_id") != other {
		t.Errorf("account_id = %q, want %q", registry.in.String("account_id"), other)
	}

	for _, field := range []string{`"email"`, `"locale"`, `"time_zone"`, `"week_start"`} {
		if strings.Contains(recorder.Body.String(), field) {
			t.Errorf("%s reached another member in %s - data-protection.md §9 puts the visibility at minimal",
				field, recorder.Body)
		}
	}
	for _, field := range []string{`"id"`, `"kind"`, `"display_name"`, `"status"`} {
		if !strings.Contains(recorder.Body.String(), field) {
			t.Errorf("%s is missing from %s", field, recorder.Body)
		}
	}
}
