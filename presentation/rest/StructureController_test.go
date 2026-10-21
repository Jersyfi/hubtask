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

func structureAnswer() map[string]any {
	return map[string]any{
		"preset":   "LIST",
		"settings": map[string]any{"completion": "MANUAL", "celebration": "SUBTLE"},
		"sources":  map[string]any{"completion": "HUB", "celebration": "DEFAULT"},
		"locks": map[string]any{
			"views": map[string]any{"origin": "WORKSPACE", "locked_by": "0192f000-0000-7000-8000-00000000000d"},
		},
	}
}

// Every container read answers its structure in force, in the contract's words.
func TestAContainerReadCarriesItsStructure(t *testing.T) {
	answer := renamedContainer()
	answer["structure"] = structureAnswer()
	registry := &catalogue{out: answer}

	recorder := containerRequest(t, registry, http.MethodGet, "/containers/"+containerID, "", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	structure, _ := body["structure"].(map[string]any)
	sources, _ := structure["sources"].(map[string]any)
	locks, _ := structure["locks"].(map[string]any)
	views, _ := locks["views"].(map[string]any)
	if structure["preset"] != "LIST" || sources["completion"] != "HUB" || views["origin"] != "WORKSPACE" {
		t.Errorf("structure = %v", body["structure"])
	}
}

// The preset asked for at creation reaches the catalogue as sent, and nothing when none was.
func TestACreateNamesItsPreset(t *testing.T) {
	registry := &catalogue{out: createdContainer()}

	containerRequest(t, registry, http.MethodPost, "/containers", `{"type":"HUB","name":"Home","preset":"LIST"}`, "")
	if registry.in["preset"] != "LIST" {
		t.Errorf("preset = %v", registry.in["preset"])
	}

	containerRequest(t, registry, http.MethodPost, "/containers", `{"type":"HUB","name":"Home"}`, "")
	if registry.in["preset"] != nil {
		t.Errorf("an absent preset reached the catalogue as %v", registry.in["preset"])
	}
}
