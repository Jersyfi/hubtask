// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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

// A structure change reaches the catalogue as the contract wrote it: the preset, the settings as a
// document, the resets by name, and the version from If-Match. The answer carries the new ETag and
// the locks that kept a value.
func TestAStructureChangeReachesTheCatalogueAsSent(t *testing.T) {
	answer := renamedContainer()
	answer["structure"] = structureAnswer()
	answer["kept_by_lock"] = []any{"celebration"}
	registry := &catalogue{out: answer}

	recorder := containerRequest(t, registry, http.MethodPatch, "/containers/"+containerID+"/structure",
		`{"preset":"LIST","settings":{"views":{"offered":["KANBAN"],"default":"KANBAN"},"capture":[]},"reset":["counts"]}`,
		`"3"`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "SetContainerStructure" || registry.in["preset"] != "LIST" ||
		registry.in["expected_version"] != 3 || registry.in["container_id"] != containerID {
		t.Errorf("in = %v", registry.in)
	}
	settings, _ := registry.in["settings"].(map[string]any)
	views, _ := settings["views"].(map[string]any)
	if views["default"] != "KANBAN" {
		t.Errorf("settings = %v", settings)
	}
	// An empty list is a value - "recognise nothing" - and must arrive as one, not vanish.
	if capture, held := settings["capture"].([]any); !held || len(capture) != 0 {
		t.Errorf("capture = %v", settings["capture"])
	}
	if reset, _ := registry.in["reset"].([]any); len(reset) != 1 || reset[0] != "counts" {
		t.Errorf("reset = %v", registry.in["reset"])
	}
	if tag := recorder.Header().Get("ETag"); tag != `"4"` {
		t.Errorf("ETag = %q", tag)
	}
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if kept, _ := body["kept_by_lock"].([]any); len(kept) != 1 || kept[0] != "celebration" {
		t.Errorf("kept_by_lock = %v", body["kept_by_lock"])
	}
}

// UC-WRK-23 check 5 at the door: a widening answers 422 capability_not_supported, and an unknown
// setting in the body is refused before the catalogue is asked.
func TestAWideningAnswers422AndAnUnknownSettingNeverReachesTheCatalogue(t *testing.T) {
	registry := &catalogue{err: shared.ErrCapabilityNotSupported.WithDetail("structure.widens").
		WithParams(map[string]string{"setting": "fields", "item_type": "ACTIVITY", "capability": "NOTES"})}

	recorder := containerRequest(t, registry, http.MethodPatch, "/containers/"+containerID+"/structure",
		`{"settings":{"fields":{"ACTIVITY":{"capabilities":["COMPLETION","NOTES"]}}}}`, "")
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", recorder.Code)
	}
	var problem map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
	if problem["code"] != "capability_not_supported" || problem["detail_code"] != "structure.widens" {
		t.Errorf("problem = %v", problem)
	}

	untouched := &catalogue{out: renamedContainer()}
	recorder = containerRequest(t, untouched, http.MethodPatch, "/containers/"+containerID+"/structure",
		`{"settings":{"colour":"red"}}`, "")
	if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d", recorder.Code)
	}
	if untouched.invoked {
		t.Error("an unknown setting reached the catalogue")
	}
}

// The workspace's structure: a null default preset clears it, an absent one leaves it; the locks
// travel as the whole set; the version comes from If-Match and goes back in the ETag.
func TestAWorkspaceStructureChangeTellsNullFromAbsent(t *testing.T) {
	answer := usecase.Output(structureAnswer())
	answer["default_preset"] = nil
	answer["version"] = 8
	registry := &catalogue{out: answer}

	recorder := containerRequest(t, registry, http.MethodPatch, "/tenant/structure",
		`{"locks":["views","fields.TASK.DUE_DATE"],"default_preset":null}`, `"7"`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "SetWorkspaceStructure" || registry.in["clear_default_preset"] != true ||
		registry.in["expected_version"] != 7 {
		t.Errorf("in = %v", registry.in)
	}
	if locks, _ := registry.in["locks"].([]any); len(locks) != 2 {
		t.Errorf("locks = %v", registry.in["locks"])
	}
	if tag := recorder.Header().Get("ETag"); tag != `"8"` {
		t.Errorf("ETag = %q", tag)
	}

	recorder = containerRequest(t, registry, http.MethodPatch, "/tenant/structure", `{"preset":"TASKS"}`, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if _, sent := registry.in["clear_default_preset"]; sent {
		t.Errorf("an absent default preset was read as null: %v", registry.in)
	}
	if _, sent := registry.in["locks"]; sent {
		t.Errorf("absent locks were read as a set: %v", registry.in)
	}
}

// GET /tenant answers the workspace's structure beside its other settings.
func TestTheWorkspaceReadCarriesItsStructure(t *testing.T) {
	structure := structureAnswer()
	structure["default_preset"] = "TASKS"
	registry := &catalogue{out: usecase.Output{
		"id": "0192f000-0000-7000-8000-00000000000a", "slug": "acme", "display_name": "Acme",
		"status": "ACTIVE", "default_locale": "en", "default_time_zone": "UTC",
		"require_admin_totp": false, "version": 3, "structure": structure,
	}}

	recorder := containerRequest(t, registry, http.MethodGet, "/tenant", "", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	answered, _ := body["structure"].(map[string]any)
	if answered["default_preset"] != "TASKS" || answered["locks"] == nil {
		t.Errorf("structure = %v", body["structure"])
	}
}
