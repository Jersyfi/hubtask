// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The installation's presets (UC-INS-18): four handlers over four use cases. The bodies carry the
// use cases' own field names, so a body becomes their input through its JSON spelling.
const (
	listInstallationPresetsUseCase    = "ListInstallationPresets"
	createInstallationPresetUseCase   = "CreateInstallationPreset"
	changeInstallationPresetUseCase   = "ChangeInstallationPreset"
	withdrawInstallationPresetUseCase = "WithdrawInstallationPreset"
)

// ListInstallationPresets answers GET /admin/presets.
func (c *RestController) ListInstallationPresets(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), listInstallationPresetsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var answer struct {
		Data []openapi.InstallationPreset `json:"data"`
	}
	if err := throughJSON(out, &answer); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if answer.Data == nil {
		answer.Data = []openapi.InstallationPreset{}
	}
	writeJSON(w, r, http.StatusOK, answer)
}

// CreateInstallationPreset answers POST /admin/presets.
func (c *RestController) CreateInstallationPreset(w http.ResponseWriter, r *http.Request) {
	var body openapi.InstallationPresetCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	c.answerInstallationPreset(w, r, http.StatusCreated, createInstallationPresetUseCase, body, nil, nil)
}

// ChangeInstallationPreset answers PATCH /admin/presets/{presetId}.
func (c *RestController) ChangeInstallationPreset(
	w http.ResponseWriter, r *http.Request, presetID openapi.PresetId, params openapi.ChangeInstallationPresetParams,
) {
	var body openapi.InstallationPresetUpdate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	id := presetID.String()
	c.answerInstallationPreset(w, r, http.StatusOK, changeInstallationPresetUseCase, body, &id, params.IfMatch)
}

// WithdrawInstallationPreset answers DELETE /admin/presets/{presetId}.
func (c *RestController) WithdrawInstallationPreset(
	w http.ResponseWriter, r *http.Request, presetID openapi.PresetId, params openapi.WithdrawInstallationPresetParams,
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
	if _, err := c.UseCases.Invoke(r.Context(), withdrawInstallationPresetUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// answerInstallationPreset invokes a use case that answers one preset, with the body's fields as
// its input, and writes the preset with its version as the tag.
func (c *RestController) answerInstallationPreset(
	w http.ResponseWriter, r *http.Request, status int, name string, body any, id *string, ifMatch *string,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	in := usecase.Input{}
	if err := throughJSON(body, &in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if id != nil {
		in["preset_id"] = *id
	}
	if version, ok := versionFromIfMatch(ifMatch); ok {
		in["expected_version"] = version
	}
	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var answer openapi.InstallationPreset
	if err := throughJSON(out, &answer); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("ETag", etag(answer.Version))
	writeJSON(w, r, status, answer)
}
