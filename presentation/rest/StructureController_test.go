// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

func listPresetsAnswer() usecase.Output {
	return usecase.Output{"data": []usecase.Output{{
		"key": "LIST",
		"settings": map[string]any{
			"levels":      []any{map[string]any{"type": "TASK", "name": nil}},
			"fields":      map[string]any{"TASK": map[string]any{"capabilities": []any{"COMPLETION"}}},
			"views":       map[string]any{"offered": []any{"LIST_COLLAPSED"}, "default": "LIST_COLLAPSED"},
			"counts":      map[string]any{"figure": "OPEN", "field_key": nil},
			"completion":  "MANUAL",
			"capture":     []any{"LINES", "AMOUNT"},
			"celebration": "SUBTLE",
			"automation":  []any{},
		},
	}}}
}

func TestThePresetsAreABoundedArrayInTheContractsWords(t *testing.T) {
	registry := &catalogue{out: listPresetsAnswer()}

	recorder := containerRequest(t, registry, http.MethodGet, "/presets", "", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ListPresets" {
		t.Errorf("use case = %q", registry.name)
	}
	var body []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an array: %v (%s)", err, recorder.Body)
	}
	if len(body) != 1 || body[0]["key"] != "LIST" {
		t.Fatalf("body = %v", body)
	}
	settings, _ := body[0]["settings"].(map[string]any)
	if len(settings) != 8 {
		t.Errorf("settings %v", settings)
	}
	// An empty set of behaviours is answered as an empty list, not left out: "nothing runs by
	// itself" is a value.
	if automation, held := settings["automation"].([]any); !held || len(automation) != 0 {
		t.Errorf("automation = %v", settings["automation"])
	}
}
