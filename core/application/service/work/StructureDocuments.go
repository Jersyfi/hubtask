// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// StructureValuesFrom reads the nine settings a document carries, as every structure write here
// reads them: the control plane's installation presets meet the same reading and the same
// refusals as a workspace's.
func StructureValuesFrom(raw map[string]any) (domain.StructureValues, error) {
	return structureValuesFrom(raw)
}

// StructureValuesOutput is the StructureSettings schema, for an answer outside this context.
func StructureValuesOutput(values domain.StructureValues) map[string]any {
	return structureValuesOutput(values)
}

// PresetOutput is the Preset schema, for an answer outside this context.
func PresetOutput(preset domain.Preset) usecase.Output { return presetOutput(preset) }
