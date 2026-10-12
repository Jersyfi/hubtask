// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// The AI area of the installation's level crosses this adapter in both directions, like the
// other four: sent as `{value, locked}`, answered whole.
func TestTheAiAreaTravelsThroughTheSettingsRoute(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"ai": usecase.Output{
			"sources":          usecase.Output{"set": true, "value": "OFFERED", "locked": true},
			"min_jurisdiction": usecase.Output{"set": false, "locked": false},
		},
	}}

	recorder := callBackup(t, registry, http.MethodPut, "/admin/settings",
		`{"ai": {"sources": {"set": true, "value": "OFFERED", "locked": true}}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	sent, _ := registry.in["ai"].(map[string]any)
	sources, _ := sent["sources"].(map[string]any)
	if sources["value"] != "OFFERED" || sources["locked"] != true {
		t.Errorf("the use case received %v", registry.in["ai"])
	}

	var body map[string]map[string]map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not the settings document: %v", err)
	}
	if body["ai"]["sources"]["value"] != "OFFERED" || body["ai"]["min_jurisdiction"]["set"] != false {
		t.Errorf("the answer is %v", body["ai"])
	}
}
