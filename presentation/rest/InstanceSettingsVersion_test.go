// SPDX-License-Identifier: Apache-2.0
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

func callSettings(t *testing.T, registry UseCaseRegistry, method, body, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+"/admin/settings", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// The level answers its version as an ETag and in the body, on a read and after a save.
func TestTheInstanceLevelAnswersItsVersion(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		registry := &catalogue{out: usecase.Output{"version": 7, "source": "database"}}
		body := ""
		if method == http.MethodPut {
			body = `{}`
		}
		recorder := callSettings(t, registry, method, body, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", method, recorder.Code, recorder.Body)
		}
		if tag := recorder.Header().Get("ETag"); tag != `"7"` {
			t.Errorf("%s: ETag = %q", method, tag)
		}
		var answer map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer["version"] != float64(7) {
			t.Errorf("%s: the body's version is %v (%v)", method, answer["version"], err)
		}
	}
}

// The version a save was made on reaches the use case from If-Match; the body's own is ignored.
func TestASaveCarriesItsIfMatchToTheUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"version": 5}}
	recorder := callSettings(t, registry, http.MethodPut, `{"version": 99}`, `"4"`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if got := registry.in["expected_version"]; got != 4 {
		t.Errorf("the use case received expected_version %v, want 4", got)
	}

	registry = &catalogue{out: usecase.Output{"version": 5}}
	callSettings(t, registry, http.MethodPut, `{}`, "")
	if _, sent := registry.in["expected_version"]; sent {
		t.Errorf("a save without If-Match named a version: %v", registry.in["expected_version"])
	}
}

// A stale save answers 409 with the code a client recovers by: re-read and reapply.
func TestAStaleSaveAnswersVersionConflict(t *testing.T) {
	registry := &catalogue{err: shared.ErrVersionConflict.WithDetail("admin.instance_settings_version_conflict")}
	recorder := callSettings(t, registry, http.MethodPut, `{}`, `"3"`)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("not a problem document: %v", err)
	}
	if problem.Code != "version_conflict" || problem.DetailCode != "admin.instance_settings_version_conflict" {
		t.Errorf("the problem is %+v", problem)
	}
}

// A saved level reaches the use case as the areas it read: each setting's value and lock, its
// description members ignored by the use case, and `module` accepted and dropped as before the
// door was generated - the use case declares no input of that name.
func TestASaveHandsTheAreasOnAndDropsTheModules(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"version": 2}}
	recorder := callSettings(t, registry, http.MethodPut,
		`{"sign_in":{"methods":{"set":true,"value":["password"],"locked":true,"kind":"CHOICES"}},`+
			`"module":{"ai":{"set":true,"value":true,"locked":false}},"metering":true}`, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	signIn, _ := registry.in["sign_in"].(map[string]any)
	methods, _ := signIn["methods"].(map[string]any)
	if methods["locked"] != true || methods["value"] == nil {
		t.Errorf("sign_in = %v", registry.in["sign_in"])
	}
	if _, passed := registry.in["module"]; passed || registry.in["metering"] != true {
		t.Errorf("in = %v", registry.in)
	}
}
