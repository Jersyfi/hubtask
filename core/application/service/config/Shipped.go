// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// shippedAt is the day this build's version of Hubtask's own templates was written: a template of
// the HUBTASK origin changes with a release, never at run time.
var shippedAt = time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)

// The key a preset's template carries by origin (configuration/templates.md §1): Hubtask's by
// its own key, a saved one by its identifier. A workspace template may take none of them.
const (
	installationPrefix = "installation_"
	workspacePrefix    = "preset_"
	ownPrefix          = "own_"
)

// presetTemplates is every preset the reader may start from as a template bundling only the
// structure part, read through ListPresets so it offers what the presets screen offers - held to
// the installation's bound, a person's own to that person alone - and none is lost (templates.md
// §1). Hubtask's carry their key as the name, which a client names in the reader's language
// (rule 8).
func (d Desk) presetTemplates(ctx context.Context, actor appshared.ActorContext) ([]domain.Template, error) {
	answer, err := d.Catalogue.Invoke(ctx, work.ListPresetsName, actor, nil)
	if err != nil {
		if withheld(err) {
			return nil, nil
		}
		return nil, err
	}
	normal, err := normalise(map[string]any(answer))
	if err != nil {
		return nil, err
	}
	rows, _ := normal.(map[string]any)["data"].([]any)
	out := make([]domain.Template, 0, len(rows))
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		template, ok := presetTemplate(row)
		if ok {
			out = append(out, template)
		}
	}
	return out, nil
}

func presetTemplate(row map[string]any) (domain.Template, bool) {
	structure := domain.Section{}
	settings, _ := row["settings"].(map[string]any)
	for setting, value := range settings {
		structure[setting] = domain.Block{settingValueField: value}
	}
	template := domain.Template{Version: 1, Document: domain.Document{sectionStructure: structure}, CreatedAt: shippedAt}
	if version, ok := row["version"].(float64); ok && version >= 1 {
		template.Version = int(version)
	}
	id := strings.ReplaceAll(text(row["id"]), "-", "")
	switch workmodel.PresetOrigin(text(row["origin"])) {
	case workmodel.PresetOriginHubtask:
		template.Key, template.Origin = strings.ToLower(text(row["key"])), domain.OriginHubtask
		template.Name = template.Key
	case workmodel.PresetOriginInstallation:
		template.Key, template.Origin, template.Name = installationPrefix+id, domain.OriginInstallation, text(row["name"])
	case workmodel.PresetOriginWorkspace:
		template.Key, template.Origin, template.Name = workspacePrefix+id, domain.OriginWorkspace, text(row["name"])
	case workmodel.PresetOriginOwn:
		template.Key, template.Origin, template.Name = ownPrefix+id, domain.OriginOwn, text(row["name"])
	default:
		return domain.Template{}, false
	}
	return template, domain.ValidKey(template.Key)
}

// presetTemplateOf answers one preset's template by key, at a version the preset is at.
func (d Desk) presetTemplateOf(ctx context.Context, actor appshared.ActorContext, key string, version int) (domain.Template, bool, error) {
	templates, err := d.presetTemplates(ctx, actor)
	if err != nil {
		return domain.Template{}, false, err
	}
	for _, t := range templates {
		if t.Key == key && (version == 0 || version == t.Version) {
			return t, true, nil
		}
	}
	return domain.Template{}, false, nil
}

// reservedKey reports whether a key is one a preset's template carries: Hubtask's four, or one
// of the prefixes a saved preset's template is named by.
func reservedKey(key string) bool {
	for _, preset := range workmodel.BuiltInPresets() {
		if key == strings.ToLower(string(preset.Key)) {
			return true
		}
	}
	for _, prefix := range []string{installationPrefix, workspacePrefix, ownPrefix} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
