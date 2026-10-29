// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// structureValuesFrom reads the settings a write sends, in the contract's words (StructureSettings),
// as every channel delivers them: a decoded JSON document. Only the shape is read here - an unknown
// key or a value of the wrong kind is refused - and what the values may be is the domain's to judge
// (StructureBound.Validate). A null is refused rather than read as "inherit": resetting a setting is
// asked for by name, in `reset`.
func structureValuesFrom(raw map[string]any) (domain.StructureValues, error) {
	var values domain.StructureValues
	for key, value := range raw {
		setting, err := domain.ParseStructureSetting(key)
		if err != nil {
			return domain.StructureValues{}, shapeInvalid(key)
		}
		switch setting {
		case domain.SettingLevels:
			values.Levels, err = levelsFrom(value)
		case domain.SettingFields:
			values.Fields, err = fieldsFrom(value)
		case domain.SettingViews:
			values.Views, err = viewsFrom(value)
		case domain.SettingCounts:
			values.Counts, err = countsFrom(value)
		case domain.SettingCompletion:
			var text string
			text, err = textFrom(value, key)
			values.Completion = domain.CompletionPolicy(text)
		case domain.SettingCapture:
			var tokens []string
			tokens, err = textsFrom(value, key)
			values.Capture = make([]domain.CaptureToken, 0, len(tokens))
			for _, token := range tokens {
				values.Capture = append(values.Capture, domain.CaptureToken(token))
			}
		case domain.SettingCelebration:
			var text string
			text, err = textFrom(value, key)
			values.Celebration = domain.CelebrationStyle(text)
		case domain.SettingAutomation:
			var behaviours []string
			behaviours, err = textsFrom(value, key)
			values.Automation = make([]domain.AutomationBehaviour, 0, len(behaviours))
			for _, behaviour := range behaviours {
				values.Automation = append(values.Automation, domain.AutomationBehaviour(behaviour))
			}
		case domain.SettingEntryTypes:
			values.EntryTypes, err = entryTypesFrom(value)
		}
		if err != nil {
			return domain.StructureValues{}, err
		}
	}
	return values, nil
}

// settingsResetFrom reads the settings a write resets to the layer above.
func settingsResetFrom(in usecase.Input) ([]domain.StructureSetting, error) {
	names, err := in.StringList("reset")
	if err != nil {
		return nil, err
	}
	reset := make([]domain.StructureSetting, 0, len(names))
	for _, name := range names {
		setting, err := domain.ParseStructureSetting(name)
		if err != nil {
			return nil, shapeInvalid("reset")
		}
		reset = append(reset, setting)
	}
	return reset, nil
}

func levelsFrom(value any) ([]domain.Level, error) {
	items, held := value.([]any)
	if !held {
		return nil, shapeInvalid(string(domain.SettingLevels))
	}
	levels := make([]domain.Level, 0, len(items))
	for _, item := range items {
		entry, held := item.(map[string]any)
		if !held {
			return nil, shapeInvalid(string(domain.SettingLevels))
		}
		itemType, err := textFrom(entry["type"], string(domain.SettingLevels))
		if err != nil {
			return nil, err
		}
		level := domain.Level{Type: domain.ItemType(itemType)}
		if name, present := entry["name"]; present && name != nil {
			if level.Name, err = textFrom(name, string(domain.SettingLevels)); err != nil {
				return nil, err
			}
		}
		if name, present := entry["done_name"]; present && name != nil {
			if level.DoneName, err = textFrom(name, string(domain.SettingLevels)); err != nil {
				return nil, err
			}
		}
		for key := range entry {
			if key != "type" && key != "name" && key != "done_name" {
				return nil, shapeInvalid(string(domain.SettingLevels))
			}
		}
		levels = append(levels, level)
	}
	return levels, nil
}

func fieldsFrom(value any) (map[domain.ItemType]domain.LevelFields, error) {
	entries, held := value.(map[string]any)
	if !held {
		return nil, shapeInvalid(string(domain.SettingFields))
	}
	fields := make(map[domain.ItemType]domain.LevelFields, len(entries))
	for itemType, entry := range entries {
		level, held := entry.(map[string]any)
		if !held {
			return nil, shapeInvalid(string(domain.SettingFields))
		}
		for key := range level {
			if key != "capabilities" && key != "categories" {
				return nil, shapeInvalid(string(domain.SettingFields))
			}
		}
		capabilities, err := textsFrom(level["capabilities"], string(domain.SettingFields))
		if err != nil {
			return nil, err
		}
		carried := make([]domain.Capability, 0, len(capabilities))
		for _, capability := range capabilities {
			carried = append(carried, domain.Capability(capability))
		}
		fields[domain.ItemType(itemType)] = domain.LevelFields{Capabilities: carried}
		if raw, present := level["categories"]; present && raw != nil {
			ids, err := textsFrom(raw, string(domain.SettingFields))
			if err != nil {
				return nil, err
			}
			categories := make([]shared.ID, 0, len(ids))
			for _, id := range ids {
				categories = append(categories, shared.ID(id))
			}
			fields[domain.ItemType(itemType)] = domain.LevelFields{Capabilities: carried, Categories: categories}
		}
	}
	return fields, nil
}

func entryTypesFrom(value any) (map[domain.ItemType][]string, error) {
	entries, held := value.(map[string]any)
	if !held {
		return nil, shapeInvalid(string(domain.SettingEntryTypes))
	}
	offered := make(map[domain.ItemType][]string, len(entries))
	for level, raw := range entries {
		keys, err := textsFrom(raw, string(domain.SettingEntryTypes))
		if err != nil {
			return nil, err
		}
		offered[domain.ItemType(level)] = keys
	}
	return offered, nil
}

func viewsFrom(value any) (*domain.Views, error) {
	entry, held := value.(map[string]any)
	if !held || len(entry) != 2 {
		return nil, shapeInvalid(string(domain.SettingViews))
	}
	offered, err := textsFrom(entry["offered"], string(domain.SettingViews))
	if err != nil {
		return nil, err
	}
	fallback, err := textFrom(entry["default"], string(domain.SettingViews))
	if err != nil {
		return nil, err
	}
	views := &domain.Views{Default: domain.ViewLayout(fallback)}
	for _, layout := range offered {
		views.Offered = append(views.Offered, domain.ViewLayout(layout))
	}
	return views, nil
}

func countsFrom(value any) (*domain.Counts, error) {
	entry, held := value.(map[string]any)
	if !held {
		return nil, shapeInvalid(string(domain.SettingCounts))
	}
	figure, err := textFrom(entry["figure"], string(domain.SettingCounts))
	if err != nil {
		return nil, err
	}
	counts := &domain.Counts{Figure: domain.Figure(figure)}
	for key, field := range entry {
		switch key {
		case "figure":
		case "field_key":
			if field != nil {
				if counts.FieldKey, err = textFrom(field, string(domain.SettingCounts)); err != nil {
					return nil, err
				}
			}
		default:
			return nil, shapeInvalid(string(domain.SettingCounts))
		}
	}
	return counts, nil
}

func textFrom(value any, setting string) (string, error) {
	text, held := value.(string)
	if !held {
		return "", shapeInvalid(setting)
	}
	return text, nil
}

func textsFrom(value any, setting string) ([]string, error) {
	switch list := value.(type) {
	case []string:
		return list, nil
	case []any:
		texts := make([]string, 0, len(list))
		for _, item := range list {
			text, held := item.(string)
			if !held {
				return nil, shapeInvalid(setting)
			}
			texts = append(texts, text)
		}
		return texts, nil
	}
	return nil, shapeInvalid(setting)
}

func shapeInvalid(setting string) error {
	return shared.ErrValidation.
		WithDetail("structure.setting_invalid").
		WithParams(map[string]string{"setting": setting}).
		WithFields(shared.FieldError{Path: "/settings/" + setting, Code: "structure.setting_invalid"})
}
