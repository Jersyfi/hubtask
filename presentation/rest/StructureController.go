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
	listPresetsUseCase = "ListPresets"
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

// throughJSON maps a catalogue output onto a generated schema by its JSON spelling. Used for the
// structure documents, whose output is already in the contract's words: a field-by-field copy
// would be a second spelling of eight settings to keep in step.
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
