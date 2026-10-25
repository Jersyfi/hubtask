// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RFC 6749's form of the token endpoint (ADR-0093): what an Alexa skill's account linking sends,
// answered in the standard shape by the same use cases as the JSON form.

func formExchange(t *testing.T, registry *catalogue, form url.Values, basic bool) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/oauth/token",
		strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic {
		request.SetBasicAuth("01936f2a-7c1e-7000-8000-00000000d001", "hbt_ocs_secret")
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheStandardFormExchangesACode(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"access_token": "hbt_at", "refresh_token": "hbt_rt",
		"access_token_expires_at": time.Now().Add(15 * time.Minute),
	}}
	response := formExchange(t, registry, url.Values{
		"grant_type": {"authorization_code"}, "code": {"hbt_oac_x"}, "code_verifier": {"v"},
		"redirect_uri": {"https://layla.amazon.com/api/skill/link/M1"},
	}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.name != exchangeOauthCodeUseCase || registry.in["client_secret"] != "hbt_ocs_secret" ||
		registry.in["client_id"] != "01936f2a-7c1e-7000-8000-00000000d001" || registry.in["code_verifier"] != "v" {
		t.Errorf("asked %s with %+v", registry.name, registry.in)
	}
	var answer map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &answer)
	if answer["access_token"] != "hbt_at" || answer["token_type"] != "Bearer" || answer["refresh_token"] != "hbt_rt" {
		t.Errorf("answered %+v", answer)
	}
	if expires, _ := answer["expires_in"].(float64); expires < 800 || expires > 900 {
		t.Errorf("expires_in %v", answer["expires_in"])
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Error("a token answer may be cached")
	}
}

func TestTheStandardFormRefreshesAndRefusesInItsOwnWords(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"access_token": "a", "refresh_token": "r"}}
	response := formExchange(t, registry, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"hbt_rt"}}, false)
	if response.Code != http.StatusOK || registry.name != refreshSessionUseCase || registry.in["refresh_token"] != "hbt_rt" {
		t.Errorf("refresh answered %d via %s", response.Code, registry.name)
	}
	unknown := formExchange(t, &catalogue{}, url.Values{"grant_type": {"password"}}, false)
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "unsupported_grant_type") {
		t.Errorf("an unknown grant answered %d %s", unknown.Code, unknown.Body)
	}
	refused := formExchange(t, &catalogue{err: shared.ErrValidation}, url.Values{"grant_type": {"authorization_code"}}, true)
	if refused.Code != http.StatusBadRequest || !strings.Contains(refused.Body.String(), "invalid_grant") {
		t.Errorf("a bad code answered %d %s", refused.Code, refused.Body)
	}
}
