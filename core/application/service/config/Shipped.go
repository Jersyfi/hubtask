// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"embed"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
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
//
// Hubtask's document templates follow them (bundledTemplates): offered to every reader who may
// read the templates at all, a preset withheld or not.
func (d Desk) presetTemplates(ctx context.Context, actor appshared.ActorContext) ([]domain.Template, error) {
	bundled, err := bundledTemplates()
	if err != nil {
		return nil, err
	}
	answer, err := d.Catalogue.Invoke(ctx, work.ListPresetsName, actor, nil)
	if err != nil {
		if withheld(err) {
			return bundled, nil
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
	return append(out, bundled...), nil
}

// shipped holds Hubtask's document templates, one file each: a template of the HUBTASK origin is
// the release's, read once (templates.md §2). Its words are the template's own, which the
// workspace takes as its own vocabulary and renames at will - the template is content, not a
// sentence the backend says (rule 8).
//
//go:embed shipped/*.json
var shipped embed.FS

var bundled = sync.OnceValues(func() ([]domain.Template, error) {
	entries, err := shipped.ReadDir("shipped")
	if err != nil {
		return nil, err
	}
	out := make([]domain.Template, 0, len(entries))
	for _, entry := range entries {
		raw, err := shipped.ReadFile("shipped/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var file struct {
			Key      string         `json:"key"`
			Version  int            `json:"version"`
			Document map[string]any `json:"document"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, err
		}
		document := domain.Document{}
		for name, rawSection := range file.Document {
			section := domain.Section{}
			blocks, _ := rawSection.(map[string]any)
			for key, rawBlock := range blocks {
				block, _ := rawBlock.(map[string]any)
				section[key] = domain.Block(block)
			}
			document[name] = section
		}
		out = append(out, domain.Template{
			Key: file.Key, Version: file.Version, Name: file.Key, Origin: domain.OriginHubtask,
			Document: document, CreatedAt: shippedAt,
		})
	}
	return out, nil
})

// bundledTemplates answers Hubtask's document templates.
func bundledTemplates() ([]domain.Template, error) { return bundled() }

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
	if templates, err := bundledTemplates(); err == nil {
		for _, template := range templates {
			if key == template.Key {
				return true
			}
		}
	}
	for _, prefix := range []string{installationPrefix, workspacePrefix, ownPrefix} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
