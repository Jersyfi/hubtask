// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// shippedVersion is the version of Hubtask's own templates this build carries, and shippedAt the
// day it was written: a template of the HUBTASK origin changes with a release, never at run time.
const shippedVersion = 1

var shippedAt = time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)

// shipped is Hubtask's templates (configuration/templates.md §2): today the four presets, each a
// template that bundles only the structure part. Build data rather than rows, as the presets are;
// the name is the key, which a client names in the reader's language (rule 8).
func shipped() []domain.Template {
	presets := workmodel.BuiltInPresets()
	out := make([]domain.Template, 0, len(presets))
	for _, preset := range presets {
		structure := domain.Section{}
		for setting, value := range work.StructureValuesOutput(preset.Values) {
			structure[setting] = domain.Block{settingValueField: value}
		}
		document, _ := normalise(map[string]any{sectionStructure: structure})
		key := strings.ToLower(string(preset.Key))
		out = append(out, domain.Template{
			Key: key, Version: shippedVersion, Name: key, Origin: domain.OriginHubtask,
			Document: documentFrom(document), CreatedAt: shippedAt,
		})
	}
	return out
}

// shippedTemplate answers one of Hubtask's templates by key, at a version this build carries.
func shippedTemplate(key string, version int) (domain.Template, bool) {
	for _, t := range shipped() {
		if t.Key == key && (version == 0 || version == t.Version) {
			return t, true
		}
	}
	return domain.Template{}, false
}

// documentFrom reads a normalised document; a shape normalise produced is always one.
func documentFrom(normal any) domain.Document {
	document, _ := documentOf(normal)
	return document
}
