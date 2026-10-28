// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const readSettingsCatalogueUseCase = "ReadSettingsCatalogue"

// ReadSettingsCatalogue answers GET /settings/catalogue: the registry every settings screen is
// drawn from (configuration/ui.md A7, A10).
func (c *RestController) ReadSettingsCatalogue(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, readSettingsCatalogueUseCase, usecase.Input{})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, settingsCatalogueResponse(out))
}

func settingsCatalogueResponse(out usecase.Output) openapi.SettingsCatalogue {
	rows, _ := out["settings"].([]usecase.Output)
	catalogue := openapi.SettingsCatalogue{Settings: make([]openapi.SettingDefinition, 0, len(rows))}
	for _, row := range rows {
		levels, _ := row["levels"].([]string)
		writers, _ := row["writers"].([]usecase.Output)
		lockable, _ := row["lockable"].(bool)
		restrictable, _ := row["restrictable"].(bool)
		definition := openapi.SettingDefinition{
			Key: row.String("key"), Kind: openapi.SettingKind(row.String("kind")),
			Levels: make([]openapi.SettingSource, 0, len(levels)), Lockable: lockable, Restrictable: restrictable,
		}
		for _, level := range levels {
			definition.Levels = append(definition.Levels, openapi.SettingSource(level))
		}
		definition.Writers = make([]struct {
			Level      openapi.SettingSource `json:"level"`
			Permission string                `json:"permission"`
		}, 0, len(writers))
		for _, writer := range writers {
			definition.Writers = append(definition.Writers, struct {
				Level      openapi.SettingSource `json:"level"`
				Permission string                `json:"permission"`
			}{Level: openapi.SettingSource(writer.String("level")), Permission: writer.String("permission")})
		}
		if options, held := row["options"].([]string); held {
			definition.Options = &options
		}
		if minimum, held := row["minimum"].(int); held {
			definition.Minimum = &minimum
		}
		if maximum, held := row["maximum"].(int); held {
			definition.Maximum = &maximum
		}
		if may := row.String("may"); may != "" {
			value := openapi.SettingDefinitionMay(may)
			definition.May = &value
		}
		if preview := row.String("preview"); preview != "" {
			definition.Preview = &preview
		}
		catalogue.Settings = append(catalogue.Settings, definition)
	}
	return catalogue
}
