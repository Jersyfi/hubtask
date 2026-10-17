// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
)

// UC-AI-06 checks 2 and 3, UC-INS-13 check 2 (multi-tenancy.md §4.2): the AI rule resolves
// installation -> plan -> workspace. A plan narrows and locks; the workspace's screen says the plan
// decided and offers only what the plan allows.
func TestTheAiRuleResolvesThroughThePlan(t *testing.T) {
	offered := domain.AiSourcesOffered
	world := &sourceWorld{
		level:   instanceSays(domain.AiSourcesEither, false),
		plan:    domain.AiLayer{Sources: &offered, Locks: map[domain.AiSwitch]bool{domain.AiSwitchSources: true}},
		offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)")},
	}
	registry := sourceRegistry(t, world)
	out := readSource(t, registry)
	if out["sources"] != "OFFERED" || out["installation_sources"] != "OFFERED" ||
		out["sources_origin"] != "PLAN" || out["sources_lock"] != "PLAN" {
		t.Errorf("the plan's rule reads %v", out)
	}
	if listed, _ := out["offered"].([]usecase.Output); len(listed) != 1 {
		t.Errorf("the offered models under the plan's rule read %v", listed)
	}

}

// A plan written before the installation narrowed cannot widen it: the installation's own value
// stands, and the screen says so.
func TestAPlanCannotWidenTheInstallationsAiRule(t *testing.T) {
	either := domain.AiSourcesEither
	world := &sourceWorld{level: instanceSays(domain.AiSourcesOwn, false), plan: domain.AiLayer{Sources: &either}}
	out := readSource(t, sourceRegistry(t, world))
	if out["sources"] != "OWN" || out["sources_origin"] != "INSTANCE" {
		t.Errorf("a wider plan reads %v", out)
	}
}
