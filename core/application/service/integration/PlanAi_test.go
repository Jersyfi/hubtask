// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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

// UC-AI-06 check 3: a plan that allows only the workspace's own model offers no installation model.
func TestAPlanThatAllowsOnlyTheOwnModelOffersNone(t *testing.T) {
	own := domain.AiSourcesOwn
	world := &sourceWorld{
		level:   instanceSays(domain.AiSourcesEither, false),
		plan:    domain.AiLayer{Sources: &own},
		offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)"), offeredModel(offeredSmall, "Small (EU)")},
	}
	out := readSource(t, sourceRegistry(t, world))
	if listed, _ := out["offered"].([]usecase.Output); len(listed) != 0 || out["sources"] != "OWN" {
		t.Errorf("under a plan allowing only the own model the answer offers %v (sources %v)", listed, out["sources"])
	}
}

// UC-AI-06 checks 4 and 5: an own model below the plan's minimum is refused naming the plan, and the
// configuration already stored is left as it is - narrowing never deletes it.
func TestAnOwnModelBelowThePlansMinimumIsRefusedAndNothingIsDeleted(t *testing.T) {
	floor := domain.AiMinEEA
	world := &sourceWorld{plan: domain.AiLayer{MinJurisdiction: &floor}}
	writer, store := providerWriter()
	writer.Rules = &AiRules{
		Instance: world, Workspaces: sourceWorkspaces{world},
		Offered: sourceOffered{world}, Choices: sourceChoices{world}, Plans: world,
	}
	command := providerCommand()
	command.Jurisdiction = domain.AiAdequacy
	_, err := ConfigureAiProvider{Writer: writer}.Execute(t.Context(), providerActor(), command)
	got := shared.AsError(err)
	if got == nil || got.DetailCode != "ai.jurisdiction_below_minimum" || got.Params["origin"] != "PLAN" {
		t.Fatalf("got %v, want ai.jurisdiction_below_minimum naming PLAN", err)
	}
	if store.stored != nil || store.sealed != nil {
		t.Error("a refused configuration was stored or sealed")
	}
}
