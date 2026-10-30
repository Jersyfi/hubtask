// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The entities the registry's rows are in the change log and the replica as (offline-sync.md
// §3.1). Each is filed under no container. An agent, a held act and an evaluation run reach the
// agent's owner and whoever administers agents (MANAGE_MEMBERS at the workspace) - the record
// names the owner as `owner_id` -, a model and an evaluation case only the administrators, as the
// use cases that list them answer.
const (
	AgentEntity   = "agent"
	ModelEntity   = "agent_model"
	RequestEntity = "agent_request"
	CaseEntity    = "agent_eval_case"
	RunEntity     = "agent_eval_run"
)

// changed records one row's change inside the caller's transaction; a nil payload is its removal.
// Without a change log nothing is recorded, a test's convenience.
func (d Desk) changed(ctx context.Context, actor appshared.ActorContext, entity string, id shared.ID, payload usecase.Output) error {
	if d.Changes == nil || d.HLC == nil {
		return nil
	}
	op, body := changelog.Upsert, map[string]any(payload)
	if payload == nil {
		op = changelog.Delete
	}
	return d.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: entity, EntityID: id, Op: op,
		ActorID: actor.AccountID, HLC: d.HLC.Next(), Payload: body,
	})
}

// agentChanged records an agent as it now stands, with its model's revision now.
func (d Desk) agentChanged(ctx context.Context, actor appshared.ActorContext, a domain.Agent) error {
	if d.Changes == nil || d.HLC == nil {
		return nil
	}
	revision, _, err := d.modelRevision(ctx, a)
	if err != nil {
		return err
	}
	return d.changed(ctx, actor, AgentEntity, a.ID, AgentOutput(a, revision))
}

// modelChanged records a model and every agent using it, whose approval it may no longer cover.
func (d Desk) modelChanged(ctx context.Context, actor appshared.ActorContext, m domain.Model) error {
	if d.Changes == nil || d.HLC == nil {
		return nil
	}
	if err := d.changed(ctx, actor, ModelEntity, m.ID, ModelOutput(m)); err != nil {
		return err
	}
	agents, err := d.Store.ListAgents(ctx, "", m.ID, maxListed)
	if err != nil {
		return err
	}
	for _, a := range agents {
		if err := d.agentChanged(ctx, actor, a); err != nil {
			return err
		}
	}
	return nil
}

// requestChanged re-reads a held act and records it with its agent's owner.
func (d Desk) requestChanged(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	if d.Changes == nil || d.HLC == nil {
		return nil
	}
	r, err := d.Store.FindRequest(ctx, id)
	if err != nil {
		return err
	}
	a, err := d.Store.FindAgent(ctx, r.AgentID)
	if err != nil {
		return err
	}
	return d.changed(ctx, actor, RequestEntity, r.ID, RequestRecord(r, a.OwnerID))
}

// RequestRecord is a held act as a device keeps it, naming the agent's owner it reaches.
func RequestRecord(r domain.Request, owner shared.ID) usecase.Output {
	out := RequestOutput(r)
	out["owner_id"] = idOrNil(owner)
	return out
}

// RunRecord is an evaluation run as a device keeps it, naming the agent's owner it reaches.
func RunRecord(r domain.Run, owner shared.ID) usecase.Output {
	out := RunOutput(r)
	out["owner_id"] = idOrNil(owner)
	return out
}

// runChanged records an evaluation run with its agent's owner.
func (d Desk) runChanged(ctx context.Context, actor appshared.ActorContext, run domain.Run) error {
	if d.Changes == nil || d.HLC == nil {
		return nil
	}
	a, err := d.Store.FindAgent(ctx, run.AgentID)
	if err != nil {
		return err
	}
	return d.changed(ctx, actor, RunEntity, run.ID, RunRecord(run, a.OwnerID))
}
