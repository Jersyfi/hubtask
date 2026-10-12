// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-PRV-04 check 4 and UC-PRV-03 check 10 against PostgreSQL as the application role: a rule
// acting as a restricted account is withheld run after run, an administrator reading the rule's
// runs sees SKIPPED with the code that says why, and after more runs than the streak that switches
// a rule off it is still enabled with no failure counted.

type noActions struct{ calls int }

func (d *noActions) Dispatch(
	context.Context, appshared.ActorContext, string, map[string]any, map[string]any,
) (usecase.Output, error) {
	d.calls++
	return usecase.Output{}, nil
}

func (*noActions) ForAction(string) (string, bool) { return "items:write", true }

type permissiveRoles struct{ permissive }

func (permissiveRoles) RoleOf(
	context.Context, appshared.ActorContext, shared.ID, []identity.Scope,
) (identity.Role, bool, error) {
	return identity.RoleOwner, true, nil
}

func TestAnAdministratorSeesWhyARestrictedRulesRunsDidNothing(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, nil)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, rule.ID, true, rule.Version, time.Now().UTC())
	}); err != nil {
		t.Fatalf("enabling: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET status = 'RESTRICTED' WHERE id = $1`,
		rule.RunAs.String()); err != nil {
		t.Fatalf("restricting the rule's account: %v", err)
	}

	actions := &noActions{}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	engine := automationservice.RunRule{
		Rules: automationRules(), Runs: automationRuns(), Failures: automationRuns(),
		Events: postgres.NewOutbox(jobQueue(t)), Dispatcher: actions, Scopes: actions,
		Entries: itemRepo(), Restrictions: privacyRepo(),
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: generator{t},
	}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantA}
	runs := automationservice.MaxConsecutiveFailures + 1
	for i := 0; i < runs; i++ {
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			_, err := engine.Execute(ctx, system, automationservice.Command{
				RuleID: rule.ID, Trigger: domain.TriggerEvent, Occasion: freshID(t).String(),
			})
			return err
		}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if actions.calls != 0 {
		t.Errorf("%d actions ran as a restricted account", actions.calls)
	}

	registry, err := usecase.NewRegistry(nil, automationservice.ListRuleRuns{Reader: automationservice.Reader{
		Runs: automationRuns(), Rules: automationRules(), Authorizer: permissiveRoles{}, UnitOfWork: unitOfWork,
	}}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	administrator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA, AccountName: "Admin",
		Scopes: []string{"automation:manage"},
	}
	out, err := registry.Invoke(ctx, automationservice.ListRuleRunsName, administrator, usecase.Input{
		"rule_id": rule.ID.String(),
	})
	if err != nil {
		t.Fatalf("reading the runs: %v", err)
	}
	listed, _ := out["data"].([]usecase.Output)
	if len(listed) != runs {
		t.Fatalf("the log holds %d runs, want %d: %v", len(listed), runs, out)
	}
	for _, run := range listed {
		if run["status"] != string(domain.RunSkipped) || run["error_code"] != automationservice.CodeRunAsRestricted {
			t.Errorf("a run reads %v / %v, want SKIPPED / %s", run["status"], run["error_code"],
				automationservice.CodeRunAsRestricted)
		}
	}

	if failures := countIn(ctx, t, `SELECT failure_count FROM automation_rule WHERE id = $1`, rule.ID.String()); failures != 0 {
		t.Errorf("the rule counts %d failures", failures)
	}
	if enabled := countIn(ctx, t, `SELECT count(*) FROM automation_rule WHERE id = $1 AND enabled`, rule.ID.String()); enabled != 1 {
		t.Error("the rule switched itself off over a restriction")
	}
}
