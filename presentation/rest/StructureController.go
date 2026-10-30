// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// containerStructureResponse maps a container after a structure change, with what a lock kept and the levels clipped.
func containerStructureResponse(out usecase.Output) openapi.Container {
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
	return answer
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
