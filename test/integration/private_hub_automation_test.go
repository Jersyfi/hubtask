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
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// D4 through the dispatcher, against PostgreSQL as the application role: an event of a private hub
// travels from the outbox to the rule engine, and only the rule whose run_as reaches the hub is
// queued - a workspace-wide rule acting for the workspace's owner, who holds nothing in the hub,
// never sees it. A rule beside it on a shared hub is unchanged.
func TestARuleSeesAPrivateHubOnlyThroughItsRunAs(t *testing.T) {
	ctx := context.Background()
	w := seedOrphanWorld(ctx, t)
	w.grant(ctx, t, w.person, identity.HubScope(w.hub), identity.RoleOwner)

	rule := func(runAs shared.ID) domain.Rule {
		built, err := domain.NewRule(domain.NewRuleInput{
			ID: freshID(t), TenantID: w.tenant, Name: freshName(t),
			Scope: domain.Scope{Type: domain.ScopeTenant}, RunAs: runAs,
			Trigger:   domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemCreated},
			Actions:   []domain.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}}},
			CreatedBy: runAs, Now: time.Now().UTC().Truncate(time.Microsecond),
		})
		if err != nil {
			t.Fatalf("building the rule: %v", err)
		}
		if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
			if err := automationRules().Insert(ctx, built); err != nil {
				return err
			}
			return automationRules().SetEnabled(ctx, built.ID, true, built.Version, time.Now().UTC())
		}); err != nil {
			t.Fatalf("writing the rule: %v", err)
		}
		return built
	}
	owners, members := rule(w.owner), rule(w.person)

	var item work.WorkItem
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		var err error
		item, err = itemRepo().Find(ctx, w.item)
		return err
	}); err != nil {
		t.Fatalf("reading the entry: %v", err)
	}
	envelope, err := event.NewItemCreated(freshID(t), item,
		event.Actor{Kind: appshared.ActorUser, ID: w.person}, time.Now().UTC(), event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	dispatchOnce(ctx, t, w.tenant, automationservice.MatchRules{
		Rules: automationRuns(), Containers: containerRepo(), Jobs: jobQueue(t),
		Clock: clockadapter.System{}, Reach: realAuthoriser(ctx, t),
	})

	queued := func(rule domain.Rule) int {
		return countIn(ctx, t, `
			SELECT count(*) FROM job
			WHERE tenant_id = $1 AND kind = 'automation.run'
			  AND payload->>'rule_id' = $2 AND payload->>'event_id' = $3`,
			w.tenant.String(), rule.ID.String(), envelope.ID.String())
	}
	if runs := queued(members); runs != 1 {
		t.Errorf("the rule acting for the hub's member was queued %d times, want 1", runs)
	}
	if runs := queued(owners); runs != 0 {
		t.Errorf("the rule acting for the workspace's owner was queued %d times for a private hub", runs)
	}
}
