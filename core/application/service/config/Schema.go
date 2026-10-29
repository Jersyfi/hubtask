// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"errors"
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const keyPattern = "^[a-z][a-z0-9_]{0,62}$"

// Schema is the document's JSON Schema, generated from the operations that write each section -
// their declared fields, kinds, allowed values and descriptions - rather than kept beside them, so
// it cannot say less or more than the use cases accept (configuration/model.md §4).
func (d Desk) Schema() map[string]any {
	properties := map[string]any{}
	for _, s := range sections() {
		attributes := map[string]any{}
		for _, attribute := range s.attributes {
			attributes[attribute] = d.attributeSchema(s, attribute)
		}
		block := map[string]any{"type": "object", "properties": attributes, "additionalProperties": false}
		names := map[string]any{"pattern": keyPattern}
		if s.name == sectionModules {
			keys := make([]string, 0, len(module.Keys()))
			for _, key := range module.Keys() {
				keys = append(keys, strings.ToLower(string(key)))
			}
			names = map[string]any{"enum": keys}
		}
		properties[s.name] = map[string]any{
			"type": "object", "propertyNames": names,
			"additionalProperties": map[string]any{"oneOf": []any{block, map[string]any{"type": "null"}}},
		}
	}
	definition := domain.AgentApplyDefinition()
	properties[sectionSettings] = map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			string(domain.AgentApplyKey): map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					settingValueField: map[string]any{
						"type": "string", "enum": slices.Clone(definition.Options), "default": definition.Default,
						"description": "What an agent's apply does: APPROVAL keeps it for a person, ALLOWED applies it, NEVER refuses it.",
					},
				},
			},
		},
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title":   "Hubtask configuration document",
		"type":    "object", "properties": properties, "additionalProperties": false,
	}
}

// attributeSchema reads an attribute's declaration from the first operation of its section that
// takes it.
func (d Desk) attributeSchema(s section, attribute string) map[string]any {
	if s.name == sectionModules {
		return map[string]any{"type": "string", "enum": []string{"ON", "OFF", "INHERIT"},
			"description": "ON or OFF here, or INHERIT to leave the module to the level above."}
	}
	candidates := []string{s.create, s.change, s.separate[attribute]}
	if s.ops != nil {
		candidates = []string{"CreateWorkflow", "SaveWorkflowDraft"}
	}
	for _, name := range candidates {
		descriptor, found := d.Catalogue.Lookup(name)
		if !found {
			continue
		}
		for _, field := range descriptor.Input {
			if field.Name != attribute {
				continue
			}
			out := map[string]any{"type": jsonType(field.Kind), "description": field.Description}
			if len(field.Enum) > 0 {
				out["enum"] = slices.Clone(field.Enum)
			}
			if s.isFixed(attribute) {
				out["description"] = field.Description + " Set when the block is created; fixed afterwards."
			}
			return out
		}
	}
	return map[string]any{}
}

func jsonType(kind usecase.Kind) string {
	switch kind {
	case usecase.KindBool:
		return "boolean"
	case usecase.KindInt:
		return "integer"
	case usecase.KindObject:
		return "object"
	case usecase.KindList, usecase.KindIDList:
		return "array"
	default:
		return "string"
	}
}

func asError(err error, target **shared.Error) bool { return errors.As(err, target) }
