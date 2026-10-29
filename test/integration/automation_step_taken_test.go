// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
)

// A STEP_TAKEN rule keeps its workflow and step through the trigger column, the lookup by kind
// finds it in its own workspace only, and a run it starts is one rule_run.trigger takes (migration
// 0218, UC-FLW-04 check 4).
func TestAStepRuleAndItsRunSurviveTheColumns(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, func(rule *domain.Rule) {
		rule.Trigger = domain.Trigger{Kind: domain.TriggerStepTaken, Workflow: "purchase", Step: "order"}
	})
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, rule.ID, true, 1, time.Now().UTC())
	}); err != nil {
		t.Fatalf("enabling: %v", err)
	}

	for tenant, want := range map[string]bool{"own": true, "other": false} {
		at := tenantA
		if tenant == "other" {
			at = tenantB
		}
		var found []domain.Rule
		if err := read(ctx, t, at, func(ctx context.Context) error {
			var err error
			found, err = automationRuns().ByTriggerKind(ctx, domain.TriggerStepTaken)
			return err
		}); err != nil {
			t.Fatalf("looking up: %v", err)
		}
		seen := false
		for _, each := range found {
			if each.ID == rule.ID {
				seen = true
				if each.Trigger.Workflow != "purchase" || each.Trigger.Step != "order" {
					t.Errorf("the trigger reads back %+v", each.Trigger)
				}
			}
		}
		if seen != want {
			t.Errorf("the %s workspace finds the rule: %v, want %v", tenant, seen, want)
		}
	}

	run, err := domain.StartRun(domain.NewRunInput{
		ID: freshID(t), TenantID: tenantA, RuleID: rule.ID, EventID: freshID(t),
		Trigger: domain.TriggerStepTaken, SubjectID: freshID(t),
		Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("starting the run: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRuns().Start(ctx, run)
	}); err != nil {
		t.Fatalf("the run of a step is refused by the column: %v", err)
	}
}
