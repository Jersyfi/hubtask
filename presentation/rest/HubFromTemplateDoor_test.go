// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func postHubFromTemplate(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/containers:from-template",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// UC-ID-16 check 9: the door hands `private` to the use case as it arrived, an explicit null as
// nothing chosen - the catalogue reads it as absent - and still refuses a member nobody declared.
func TestTheHubFromATemplateDoorCarriesPrivate(t *testing.T) {
	t.Run("private", func(t *testing.T) {
		registry := &catalogue{out: createdContainer()}
		recorder := postHubFromTemplate(t, registry, `{"name":"Diary","template":"kanban_team","private":true}`)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
		}
		if registry.name != "CreateHubFromTemplate" || registry.in["private"] != true {
			t.Errorf("%s was asked with %v", registry.name, registry.in)
		}
	})
	t.Run("null", func(t *testing.T) {
		registry := &catalogue{out: createdContainer()}
		recorder := postHubFromTemplate(t, registry, `{"name":"Diary","template":"kanban_team","private":null}`)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
		}
		if value, sent := registry.in["private"]; sent && value != nil {
			t.Errorf("an explicit null arrived as %v", value)
		}
	})
	t.Run("an unknown member", func(t *testing.T) {
		registry := &catalogue{out: createdContainer()}
		recorder := postHubFromTemplate(t, registry, `{"name":"Diary","template":"kanban_team","privat":true}`)
		if recorder.Code != http.StatusUnprocessableEntity || registry.invoked {
			t.Errorf("status %d, invoked %v: %s", recorder.Code, registry.invoked, recorder.Body)
		}
	})
}
