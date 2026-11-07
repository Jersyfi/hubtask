// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"context"
	"slices"
	"unicode"
	"unicode/utf8"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const ReadSettingsCatalogueName = "ReadSettingsCatalogue"

// ReadSettingsCatalogue answers every declared setting - no place's value - so that a settings
// screen, the settings search (configuration/ui.md A7) and the configuration file's sections
// (A10) are drawn from the registry rather than from a list of their own.
type ReadSettingsCatalogue struct{}

// Execute answers the catalogue to a signed-in caller. The declarations are the product's, the
// same for every workspace, so no permission beyond signing in is asked: what a reader may change
// is answered at the place, by the read of its values.
func (ReadSettingsCatalogue) Execute(_ context.Context, actor appshared.ActorContext) ([]domain.Definition, error) {
	if !actor.IsAuthenticated() {
		return nil, shared.ErrUnauthenticated
	}
	catalogue, err := Catalogue()
	if err != nil {
		return nil, err
	}
	return catalogue.Definitions(), nil
}

// Descriptor is the catalogue entry.
func (h ReadSettingsCatalogue) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadSettingsCatalogueName,
		Summary: "Lists every setting the product declares: its key, the shape of its value, its " +
			"bounds or options, the levels that may decide it, whether a level may lock or " +
			"restrict it, the permission a writer needs at each level, and the operation that " +
			"previews a change of it. It holds no place's value.",
		SideEffects: "None.",
		ReadOnly:    true,
		// Nothing to record: the answer is the product's own declaration, the same for everybody.
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a read changes nothing, and a setting is not an item.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			definitions, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			return CatalogueOutput(definitions), nil
		}),
	}
}

// CatalogueOutput is the answer: `settings`, one row per definition, in the declared order.
func CatalogueOutput(definitions []domain.Definition) usecase.Output {
	rows := make([]usecase.Output, 0, len(definitions))
	for _, definition := range definitions {
		levels := make([]string, 0, len(definition.Levels))
		writers := make([]usecase.Output, 0, len(definition.Writers))
		for _, level := range definition.Levels {
			levels = append(levels, string(level))
			if permission, held := definition.Writers[level]; held {
				writers = append(writers, usecase.Output{"level": string(level), "permission": permission})
			}
		}
		row := usecase.Output{
			"key":          string(definition.Key),
			"kind":         string(definition.Kind),
			"levels":       levels,
			"lockable":     definition.Lockable,
			"restrictable": definition.Restrictable,
			"writers":      writers,
		}
		if definition.Options != nil {
			row["options"] = slices.Clone(definition.Options)
		}
		if definition.Minimum != nil {
			row["minimum"] = *definition.Minimum
		}
		if definition.Maximum != nil {
			row["maximum"] = *definition.Maximum
		}
		if definition.May != domain.MayNeverLock {
			row["may"] = string(definition.May)
		}
		if definition.Preview != "" {
			row["preview"] = operationOf(definition.Preview)
		}
		rows = append(rows, row)
	}
	return usecase.Output{"settings": rows}
}

// operationOf is a use case's operationId: its name in lowerCamelCase, as the registry derives it.
func operationOf(name string) string {
	first, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToLower(first)) + name[size:]
}
