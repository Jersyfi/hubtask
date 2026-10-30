// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Catalogue is the one registry of every setting (F-5, P-17): each area's declarations, which live
// beside the switch they are about, assembled in the order a screen draws the areas - sign-in,
// legal, localisation, quotas, AI, structure, modules, configuration. A screen, the settings search, the
// configuration file and the parity checks read this; none keeps a list of its own.
//
// Assembled here rather than in the domain because the quotas' vocabulary is the quota service's.
func Catalogue() (domain.Catalogue, error) {
	definitions := make([]domain.Definition, 0, 64)
	for _, name := range identity.PolicySwitches() {
		definitions = append(definitions, identity.SwitchDefinition(name))
	}
	for _, link := range identity.LevelLegalSettingNames() {
		definitions = append(definitions, identity.LegalDefinition(link))
	}
	for _, name := range identity.LocalisationKeys() {
		definitions = append(definitions, identity.LocalisationDefinition(name))
	}
	for _, name := range quota.OperatorNames() {
		definitions = append(definitions, quota.Definition(name))
	}
	for _, name := range integration.AiSwitches() {
		definitions = append(definitions, integration.AiDefinition(name))
	}
	for _, name := range work.StructureSettingNames() {
		definitions = append(definitions, work.StructureDefinition(name))
	}
	definitions = append(definitions, module.Definitions()...)
	definitions = append(definitions, config.AgentApplyDefinition())
	definitions = append(definitions, agent.RegisteredOnlyDefinition())
	return domain.NewCatalogue(definitions...)
}
