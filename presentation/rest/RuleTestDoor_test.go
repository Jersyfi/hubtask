// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// A dry run's inline rule reaches the use case as the document a create sends - the use case
// reads it through the create's own readers - and the answer is what the run found.
func TestADryRunHandsTheInlineRuleOn(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"matched":           true,
		"condition_results": []any{map[string]any{"index": 0, "matched": true}},
		"actions":           []any{map[string]any{"path": "actions[0]", "kind": "set_field", "would_run": true}},
	}}
	controller := NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/automation/rules:test",
		strings.NewReader(`{"rule":{"name":"Close done","scope":{"type":"TENANT"},`+
			`"run_as":"0192f000-0000-7000-8000-0000000000c1","trigger":{"kind":"event","event_type":"item.updated"},`+
			`"actions":[{"kind":"set_field","params":{"field":"status"}}],"throttle":{"max_runs_per_hour":5}},`+
			`"sample_event":{"type":"item.updated"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || registry.name != "TestRule" {
		t.Fatalf("status %d, asked %s: %s", response.Code, registry.name, response.Body)
	}
	rule, _ := registry.in["rule"].(map[string]any)
	trigger, _ := rule["trigger"].(map[string]any)
	if rule["name"] != "Close done" || trigger["event_type"] != "item.updated" || rule["throttle"] == nil {
		t.Errorf("rule %v", registry.in["rule"])
	}
	var answer map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &answer)
	if answer["matched"] != true || len(answer["actions"].([]any)) != 1 {
		t.Errorf("answered %v", answer)
	}
}
