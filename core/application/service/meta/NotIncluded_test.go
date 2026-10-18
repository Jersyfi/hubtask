// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type planEntitlements struct {
	leftOut entitlement.LeftOut
	err     error
	asked   []appshared.ActorContext
}

func (p *planEntitlements) LeftOut(_ context.Context, actor appshared.ActorContext) (entitlement.LeftOut, error) {
	p.asked = append(p.asked, actor)
	return p.leftOut, p.err
}

func withPlan(plan *planEntitlements) GetCapabilities {
	h := handler(profiles{list: systemDefaults()}, &unitOfWork{})
	h.Actions = []string{"CREATE_ITEM", "CREATE_RULE", "SEND_WEBHOOK"}
	h.ActionEntitlements = map[string]entitlement.Feature{
		"CREATE_RULE": entitlement.AutomationRules, "SEND_WEBHOOK": entitlement.Webhooks,
	}
	h.Entitlements = plan
	return h
}

// A signed-in caller is told what their workspace's plan leaves out, for their own workspace, and
// the rule editor is not offered an action whose feature that is (UC-INS-13 check 3, P-05).
func TestTheManifestSaysWhatThePlanLeavesOut(t *testing.T) {
	plan := &planEntitlements{leftOut: entitlement.LeftOut{entitlement.Webhooks}}
	capabilities, err := withPlan(plan).Execute(t.Context(), member())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(capabilities.NotIncluded, entitlement.LeftOut{entitlement.Webhooks}) {
		t.Errorf("not included %v", capabilities.NotIncluded)
	}
	if !slices.Equal(capabilities.AutomationActions, []string{"CREATE_ITEM", "CREATE_RULE"}) {
		t.Errorf("offered actions %v", capabilities.AutomationActions)
	}
	if len(plan.asked) != 1 || plan.asked[0].TenantID != member().TenantID {
		t.Errorf("asked for %v", plan.asked)
	}
}

// Nothing left out is an empty set, answered, and every action offered; anonymous, nothing is asked
// and nothing answered.
func TestAPlanLeavingOutNothingAndAnAnonymousCaller(t *testing.T) {
	plan := &planEntitlements{}
	capabilities, err := withPlan(plan).Execute(t.Context(), member())
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.NotIncluded == nil || len(capabilities.NotIncluded) != 0 || len(capabilities.AutomationActions) != 3 {
		t.Errorf("nothing left out: %v, %v", capabilities.NotIncluded, capabilities.AutomationActions)
	}

	anonymous, err := withPlan(plan).Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil {
		t.Fatal(err)
	}
	if anonymous.NotIncluded != nil || len(plan.asked) != 1 {
		t.Errorf("anonymous: %v, asked %d times", anonymous.NotIncluded, len(plan.asked))
	}
}

func TestAnUnreadablePlanFailsTheManifest(t *testing.T) {
	_, err := withPlan(&planEntitlements{err: shared.ErrUnavailable}).Execute(t.Context(), member())
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("an unreadable plan answered %v", err)
	}
}
