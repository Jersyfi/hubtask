// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"errors"
	"slices"
	"strings"

	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

const keyPattern = "^[a-z][a-z0-9_]{0,62}$"

// collectionKeyPattern is a collection block's key: the collection's handle (migration 0204), a
// dot and the block's own key.
const collectionKeyPattern = "^[a-z][a-z0-9]{1,19}\\.[a-z][a-z0-9_]{0,62}$"

// Schema is the document's JSON Schema, generated from the operations that write each section -
// their declared fields, kinds, allowed values and descriptions - rather than kept beside them, so
// it cannot say less or more than the use cases accept (configuration.md §1).
func (d Desk) Schema() map[string]any {
	properties := d.sectionsSchema(sections())
	properties[sectionSettings] = settingsSchema()
	hub := d.sectionsSchema(hubSections(""))
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title":   "Hubtask configuration document",
		"type":    "object", "properties": properties, "additionalProperties": false,
		// A hub's own document, read and written with hub_id (configuration.md §1).
		"$defs": map[string]any{"hub": map[string]any{
			"title": "A hub's configuration document",
			"type":  "object", "properties": hub, "additionalProperties": false,
		}},
	}
}

func (d Desk) sectionsSchema(list []section) map[string]any {
	properties := map[string]any{}
	for _, s := range list {
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
		if s.name == sectionStructure {
			keys := make([]string, 0, len(work.StructureSettingNames()))
			for _, key := range work.StructureSettingNames() {
				keys = append(keys, string(key))
			}
			if s.list == ReadWorkspaceName {
				keys = append(keys, structureLocks, structureDefaultPreset)
			}
			names = map[string]any{"enum": keys}
		}
		switch {
		case s.name == sectionTolerance:
			names = map[string]any{"enum": []string{toleranceKey}}
		case s.collection:
			// A collection's handle, a dot and the block's own key.
			names = map[string]any{"pattern": collectionKeyPattern}
		}
		properties[s.name] = map[string]any{
			"type": "object", "propertyNames": names,
			"additionalProperties": map[string]any{"oneOf": []any{block, map[string]any{"type": "null"}}},
		}
	}
	return properties
}

// attributeSchema reads an attribute's declaration from the first operation of its section that
// takes it.
func (d Desk) attributeSchema(s section, attribute string) map[string]any {
	if s.name == sectionModules {
		return map[string]any{"type": "string", "enum": []string{"ON", "OFF", "INHERIT"},
			"description": "ON or OFF here, or INHERIT to leave the module to the level above."}
	}
	if s.name == sectionTolerance || s.name == sectionCollectionTolerance {
		if attribute == lockAttribute {
			return map[string]any{"type": "boolean", "description": "Fix it for the collections below."}
		}
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 365,
			"description": "The calendar days the forecast end may slip; a removed block leaves it to the level above."}
	}
	if s.name == sectionCollectionStructure {
		return map[string]any{"description": "The collection's own value of the setting, in the shape of " +
			"StructureSettings; a removed block leaves it to the hub."}
	}
	if s.name == sectionStructure {
		if attribute == presetIDAttribute {
			return map[string]any{"type": "string", "format": "uuid", "description": "default_preset only: a " +
				"preset of the workspace's own as the one a new hub starts from, instead of a Hubtask preset's key in value."}
		}
		return map[string]any{"description": "The workspace's own value of the setting, in the shape of " +
			"StructureSettings; a removed block leaves it to the installation's or Hubtask's default. " +
			"locks: the whole list of the workspace's locks (a setting, or fields.<TYPE>.<CAPABILITY>). " +
			"default_preset: the Hubtask preset a new hub starts from (list, tasks, project, flow)."}
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

// settingsSchema is the `settings` section's schema from the catalogue: every setting the
// workspace decides and the document writes, with its kind, bounds and options.
func settingsSchema() map[string]any {
	keys := map[string]any{}
	catalogue, err := settingservice.Catalogue()
	if err == nil {
		for _, definition := range catalogue.Definitions() {
			key := string(definition.Key)
			if !definition.DecidedAt(setting.LevelWorkspace) {
				continue
			}
			if _, held := areaOf(key); !held && definition.Key != domain.AgentApplyKey {
				continue
			}
			if strings.HasPrefix(key, signInPrefix) && !knownSignIn(key) {
				continue
			}
			value := valueSchema(definition)
			attributes := map[string]any{settingValueField: value}
			if key == aiPersonOptOutKey {
				attributes[basisAttribute] = map[string]any{"type": "string", "description": "The legal basis, required with NOT_OFFERED."}
				attributes[contactAttribute] = map[string]any{"type": "string", "description": "Whom people may ask about it."}
			}
			keys[key] = map[string]any{"type": "object", "additionalProperties": false, "properties": attributes}
		}
	}
	return map[string]any{
		"type": "object", "additionalProperties": false, "properties": keys,
		"description": "Catalogue settings the workspace decides itself; a value left to the level above has no block. " +
			"A change to a sign-in switch or a legal link is applied with a fresh proof (step_up).",
	}
}

// valueSchema is a setting's value as JSON Schema.
func valueSchema(definition setting.Definition) map[string]any {
	out := map[string]any{}
	switch definition.Kind {
	case setting.KindCount:
		out["type"] = "integer"
		if definition.Minimum != nil {
			out["minimum"] = *definition.Minimum
		}
		if definition.Maximum != nil {
			out["maximum"] = *definition.Maximum
		}
	case setting.KindFlag:
		out["type"] = "boolean"
	case setting.KindChoices:
		items := map[string]any{"type": "string"}
		if len(definition.Options) > 0 {
			items["enum"] = slices.Clone(definition.Options)
		}
		out["type"], out["items"] = "array", items
	default:
		out["type"] = "string"
		if len(definition.Options) > 0 {
			out["enum"] = slices.Clone(definition.Options)
		}
	}
	if definition.Default != nil {
		out["default"] = definition.Default
	}
	return out
}
