// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The structure use cases: presets and the settings of hubs, collections and the workspace.
const (
	listPresetsUseCase           = "ListPresets"
	setContainerStructureUseCase = "SetContainerStructure"
	setWorkspaceStructureUseCase = "SetWorkspaceStructure"
	savePresetUseCase            = "SavePreset"
	changePresetUseCase          = "ChangePreset"
	withdrawPresetUseCase        = "WithdrawPreset"
	closeStarterUseCase          = "CloseStarter"
)

// ListPresets answers GET /presets.
func (c *RestController) ListPresets(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listPresetsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	presets := make([]openapi.Preset, 0, len(rows))
	for _, row := range rows {
		var preset openapi.Preset
		if err := throughJSON(row, &preset); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		presets = append(presets, preset)
	}
	writeJSON(w, r, http.StatusOK, presets)
}

// SetContainerStructure answers PATCH /containers/{containerId}/structure.
//
// The body is decoded into the generated type first, so an unknown key is refused here as on every
// other route; the settings then travel to the catalogue in the contract's words, the same document
// an MCP client sends.
func (c *RestController) SetContainerStructure(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, params openapi.SetContainerStructureParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.StructureChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in, err := structureChangeInput(body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in["container_id"] = containerID.String()
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), setContainerStructureUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	answer := containerResponse(out)
	if kept, held := out["kept_by_lock"].([]any); held {
		keys := make([]string, 0, len(kept))
		for _, key := range kept {
			if text, ok := key.(string); ok {
				keys = append(keys, text)
			}
		}
		answer.KeptByLock = &keys
	}
	if clipped, held := out["clipped_by_levels"].([]any); held {
		settings := make([]openapi.StructureSettingName, 0, len(clipped))
		for _, setting := range clipped {
			if text, ok := setting.(string); ok {
				settings = append(settings, openapi.StructureSettingName(text))
			}
		}
		answer.ClippedByLevels = &settings
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, answer)
}

// structureChangeInput is a structure change as the catalogue reads it: the preset, the settings as
// a document, and the settings to reset.
func structureChangeInput(body openapi.StructureChange) (usecase.Input, error) {
	in := usecase.Input{}
	if body.Preset != nil {
		in["preset"] = string(*body.Preset)
	}
	if body.PresetId != nil {
		in["preset_id"] = body.PresetId.String()
	}
	if body.Settings != nil {
		var settings map[string]any
		if err := throughJSON(body.Settings, &settings); err != nil {
			return nil, err
		}
		in["settings"] = settings
	}
	if body.Reset != nil {
		reset := make([]any, 0, len(*body.Reset))
		for _, name := range *body.Reset {
			reset = append(reset, string(name))
		}
		in["reset"] = reset
	}
	return in, nil
}

// SetWorkspaceStructure answers PATCH /tenant/structure.
//
// `default_preset: null` removes the default, and an absent key leaves it: the two are told apart by
// presence, which the generated type cannot do.
func (c *RestController) SetWorkspaceStructure(
	w http.ResponseWriter, r *http.Request, params openapi.SetWorkspaceStructureParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.WorkspaceStructureChange
	sent, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in, err := structureChangeInput(openapi.StructureChange{
		Settings: body.Settings, Reset: body.Reset, PresetId: body.PresetId,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if body.Preset != nil {
		in["preset"] = string(*body.Preset)
	}
	if body.Locks != nil {
		locks := make([]any, 0, len(*body.Locks))
		for _, key := range *body.Locks {
			locks = append(locks, key)
		}
		in["locks"] = locks
	}
	if sent["default_preset"] {
		if body.DefaultPreset == nil {
			in["clear_default_preset"] = true
		} else {
			in["default_preset"] = string(*body.DefaultPreset)
		}
	}
	if sent["default_preset_id"] {
		if body.DefaultPresetId == nil {
			in["clear_default_preset"] = true
		} else {
			in["default_preset_id"] = body.DefaultPresetId.String()
		}
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), setWorkspaceStructureUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var answer openapi.WorkspaceStructure
	if err := throughJSON(out, &answer); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, answer)
}

// throughJSON maps a catalogue output onto a generated schema by its JSON spelling. Used for the
// structure documents, whose output is already in the contract's words: a field-by-field copy
// would be a second spelling of eight settings to keep in step.
// SavePreset answers POST /presets.
func (c *RestController) SavePreset(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.PresetCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), savePresetUseCase, actorOf(r), usecase.Input{
		"name": body.Name, "origin": string(body.Origin), "container_id": body.ContainerId.String(),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writePreset(w, r, http.StatusCreated, out)
}

// ChangePreset answers PATCH /presets/{presetId}.
func (c *RestController) ChangePreset(
	w http.ResponseWriter, r *http.Request, presetID openapi.PresetId, params openapi.ChangePresetParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.PresetUpdate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"preset_id": presetID.String(), "container_id": optionalUUIDField(body.ContainerId)}
	if body.Name != nil {
		in["name"] = *body.Name
	}
	if body.Settings != nil {
		var settings map[string]any
		if err := throughJSON(body.Settings, &settings); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		in["settings"] = settings
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	out, err := c.UseCases.Invoke(r.Context(), changePresetUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writePreset(w, r, http.StatusOK, out)
}

// WithdrawPreset answers DELETE /presets/{presetId}.
func (c *RestController) WithdrawPreset(
	w http.ResponseWriter, r *http.Request, presetID openapi.PresetId, params openapi.WithdrawPresetParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	in := usecase.Input{"preset_id": presetID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	if _, err := c.UseCases.Invoke(r.Context(), withdrawPresetUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CloseStarter answers POST /tenant/starter:close.
func (c *RestController) CloseStarter(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	if _, err := c.UseCases.Invoke(r.Context(), closeStarterUseCase, actorOf(r), usecase.Input{}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writePreset(w http.ResponseWriter, r *http.Request, status int, out usecase.Output) {
	var preset openapi.Preset
	if err := throughJSON(out, &preset); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, status, preset)
}

func throughJSON(value any, target any) error {
	encoded, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(encoded, target)
	}
	if err != nil {
		return shared.ErrInternal.WithDetail("rest.output_unmappable").WithCause(err)
	}
	return nil
}
