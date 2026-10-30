// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

type stamps struct{}

func (stamps) Next() shared.HLC { return shared.HLC{} }

func (c *changes) of(entity string) []changelog.Change {
	var out []changelog.Change
	for _, change := range c.recorded {
		if change.Entity == entity {
			out = append(out, change)
		}
	}
	return out
}

// UC-AGT-01 … 04 offline: a model, an agent, a held act, an evaluation case and a run reach the
// change log under no container; a held act and a run name the agent's owner they reach, a changed
// model records the agents it may stop, and a removed case is recorded.
func TestTheRegistrysWritesAreInTheChangeLog(t *testing.T) {
	f := newFixture()
	log := &changes{}
	f.desk.Changes, f.desk.HLC = log, stamps{}
	ctx := context.Background()
	out, err := (RegisterAgentModel{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
		"key": "local", "name": "Local", "source": "OWN", "model": "m1", "region": "SELF_HOSTED",
	})
	if err != nil {
		t.Fatal(err)
	}
	model := out.String("id")
	f.live(t, f.register(t, usecase.Input{"model_id": model, "scopes": []any{"items:write"}}))
	if got := log.of(AgentEntity); len(got) != 3 || got[2].Payload["status"] != "ACTIVE" || got[2].Payload["owner_id"] != ana.String() {
		t.Fatalf("the agent's changes = %+v, want registered, approved and activated", got)
	}
	if _, err := (ChangeAgentModel{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
		"model_id": model, "name": "Local", "source": "OWN", "model": "m2", "region": "SELF_HOSTED",
	}); err != nil {
		t.Fatal(err)
	}
	if got := log.of(ModelEntity); len(got) != 2 || got[1].Payload["model"] != "m2" {
		t.Errorf("the model's changes = %+v", got)
	}
	if got := log.of(AgentEntity); len(got) != 4 || got[3].Payload["needs_approval"] != true {
		t.Errorf("a changed model recorded its agent as %+v, want it needing approval", got[len(got)-1].Payload)
	}

	held := f.register(t, usecase.Input{"scopes": []any{"items:write"}})
	held = f.live(t, held)
	actor := appshared.ActorContext{Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: held.AccountID, Scopes: []string{"items:write"}}
	next := usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
		return usecase.Output{}, nil
	})
	if _, err := (Gate{Desk: f.desk}).Wrap(nil)(usecase.Descriptor{Name: "UpdateWorkItem"}, next).Invoke(ctx, actor, usecase.Input{"title": "New"}); code(err) != "agents.approval_required" {
		t.Fatalf("the write was not held: %v", err)
	}
	requests := log.of(RequestEntity)
	if len(requests) != 1 || requests[0].Payload["owner_id"] != ana.String() || requests[0].Payload["status"] != "PENDING" || !requests[0].ContainerID.IsZero() {
		t.Fatalf("the held act's change = %+v", requests)
	}
	if _, err := (DecideAgentRequest{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(ana),
		usecase.Input{"request_id": requests[0].EntityID.String(), "approve": false}); err != nil {
		t.Fatal(err)
	}
	if got := log.of(RequestEntity); len(got) != 2 || got[1].Payload["status"] != "DECLINED" {
		t.Errorf("the decided act's change = %+v", got)
	}

	added, err := (AddAgentCase{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
		"agent_id": held.ID.String(), "title": "Printer jams", "expected": map[string]any{"labels": []any{"hw"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := log.of(CaseEntity); len(got) != 1 || got[0].Payload["title"] != "Printer jams" {
		t.Errorf("the case's change = %+v", got)
	}
	if _, err := (RemoveAgentCase{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
		"agent_id": held.ID.String(), "case_id": added.String("id"),
	}); err != nil {
		t.Fatal(err)
	}
	if got := log.of(CaseEntity); len(got) != 2 || got[1].Op != changelog.Delete {
		t.Errorf("the case's removal = %+v", got)
	}
}
