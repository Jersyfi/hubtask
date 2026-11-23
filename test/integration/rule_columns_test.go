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
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	automationadapter "github.com/Jersyfi/hubtask/infrastructure/automation"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ruleActions is the composition root's bridge from the engine to the registry (dispatchActions
// and actionScopes in cmd/server), over the catalogue a test hands it.
type ruleActions struct{ catalogue *usecase.Registry }

func (a ruleActions) Dispatch(
	ctx context.Context, runAs appshared.ActorContext, kind string, params, supplied map[string]any,
) (usecase.Output, error) {
	return automationadapter.NewActionDispatcher(a.catalogue).
		Dispatch(ctx, runAs, automationadapter.Action{Kind: kind, Params: params}, supplied)
}

func (a ruleActions) ForAction(kind string) (string, bool) {
	descriptor, found := a.catalogue.ByAutomationAction(kind)
	return descriptor.TokenScope, found
}

// UC-WRK-31 check 6, where a rule creates the collection: the engine runs CREATE_CONTAINER as the
// rule's account through the registry into CreateContainer, against PostgreSQL, and a collection
// whose board opens first starts with its columns named in the workspace's language - not in the
// language of the person whose action started the run. Not covered: the queue runner that starts
// the run in production, and an event source; the run is started directly, as a manual one.
func TestARuleCreatesACollectionWithColumnsInTheWorkspacesLanguage(t *testing.T) {
	ctx := context.Background()
	registry := structureCatalogue(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `UPDATE tenant SET default_locale = 'de' WHERE id = $1`, tenant.String()); err != nil {
		t.Fatalf("setting the workspace's language: %v", err)
	}
	hub, err := registry.Invoke(ctx, "CreateContainer", owner, usecase.Input{
		"type": "HUB", "name": freshName(t), "preset": "FLOW",
	})
	if err != nil {
		t.Fatalf("creating the hub: %v", err)
	}

	runAs := seedServiceAccount(ctx, t, tenant)
	if _, err := admin.Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'ADMIN')`,
		freshID(t).String(), tenant.String(), runAs.String()); err != nil {
		t.Fatalf("granting the rule's account: %v", err)
	}
	name := freshName(t)
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: freshID(t), TenantID: tenant, Name: freshName(t),
		Scope: domain.Scope{Type: domain.ScopeTenant}, RunAs: runAs,
		Trigger: domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemOverdue},
		Actions: []domain.Action{{Kind: "CREATE_CONTAINER", Params: map[string]any{
			"type": "COLLECTION", "parent_id": hub.String("id"), "name": name,
		}}},
		CreatedBy: owner.AccountID, Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if err := automationRules().Insert(ctx, rule); err != nil {
			return err
		}
		return automationRules().SetEnabled(ctx, rule.ID, true, rule.Version, time.Now().UTC())
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	actions := ruleActions{catalogue: registry}
	engine := automationservice.RunRule{
		Rules: automationRules(), Runs: automationRuns(), Failures: automationRuns(),
		Events: postgres.NewOutbox(jobQueue(t)), Dispatcher: actions, Scopes: actions,
		Entries: itemRepo(), UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock: clockadapter.System{}, IDs: generator{t},
	}
	// The person whose action started the run reads English; the workspace's language is German.
	starter := appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: tenant, Locale: "en", AccountLocale: "en",
	}
	var run domain.Run
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		run, err = engine.Execute(ctx, starter, automationservice.Command{
			RuleID: rule.ID, Trigger: domain.TriggerEvent, Occasion: freshID(t).String(),
		})
		return err
	}); err != nil {
		t.Fatalf("running the rule: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("the run %s: %+v", run.Status, run.ActionResults)
	}

	var collection string
	if err := admin.QueryRow(ctx, `SELECT id FROM container WHERE tenant_id = $1 AND name = $2`,
		tenant.String(), name).Scan(&collection); err != nil {
		t.Fatalf("finding the rule's collection: %v", err)
	}
	columns := columnsOf(ctx, t, collection)
	if len(columns) != 3 || columns[0].name != "Zu erledigen" || columns[1].name != "In Arbeit" ||
		columns[2].name != "Erledigt" {
		t.Errorf("the rule's collection starts with %+v, want the three columns in German", columns)
	}
}
