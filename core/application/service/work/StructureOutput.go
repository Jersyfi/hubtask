// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// structureValuesOutput is the StructureSettings schema: the settings these values decide, in the
// contract's words. Every channel - REST, MCP, an automation result and the change log - answers
// this one shape.
func structureValuesOutput(values domain.StructureValues) map[string]any {
	out := map[string]any{}
	for _, setting := range values.Decided() {
		out[string(setting)] = structureSettingOutput(setting, values)
	}
	return out
}

// structureSettingOutput is one setting's value as the contract spells it.
func structureSettingOutput(setting domain.StructureSetting, values domain.StructureValues) any {
	switch setting {
	case domain.SettingLevels:
		levels := make([]any, 0, len(values.Levels))
		for _, level := range values.Levels {
			entry := map[string]any{"type": string(level.Type), "name": nil}
			if level.Name != "" {
				entry["name"] = level.Name
			}
			levels = append(levels, entry)
		}
		return levels
	case domain.SettingFields:
		fields := make(map[string]any, len(values.Fields))
		for itemType, level := range values.Fields {
			capabilities := make([]any, 0, len(level.Capabilities))
			for _, capability := range level.Capabilities {
				capabilities = append(capabilities, string(capability))
			}
			fields[string(itemType)] = map[string]any{"capabilities": capabilities}
		}
		return fields
	case domain.SettingViews:
		if values.Views == nil {
			return nil
		}
		offered := make([]any, 0, len(values.Views.Offered))
		for _, layout := range values.Views.Offered {
			offered = append(offered, string(layout))
		}
		return map[string]any{"offered": offered, "default": string(values.Views.Default)}
	case domain.SettingCounts:
		if values.Counts == nil {
			return nil
		}
		counts := map[string]any{"figure": string(values.Counts.Figure), "field_key": nil}
		if values.Counts.FieldKey != "" {
			counts["field_key"] = values.Counts.FieldKey
		}
		return counts
	case domain.SettingCompletion:
		return string(values.Completion)
	case domain.SettingCapture:
		tokens := make([]any, 0, len(values.Capture))
		for _, token := range values.Capture {
			tokens = append(tokens, string(token))
		}
		return tokens
	case domain.SettingCelebration:
		return string(values.Celebration)
	case domain.SettingAutomation:
		behaviours := make([]any, 0, len(values.Automation))
		for _, behaviour := range values.Automation {
			behaviours = append(behaviours, string(behaviour))
		}
		return behaviours
	}
	return nil
}

// presetOutput is the Preset schema.
func presetOutput(preset domain.Preset) usecase.Output {
	return map[string]any{
		"key":      string(preset.Key),
		"settings": structureValuesOutput(preset.Values),
	}
}
