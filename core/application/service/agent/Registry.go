// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package agent is the governed agents module's use cases (M9, F-12): the models a workspace
// registers for its agents (UC-AGT-03), agents registered, approved, put live, stopped and retired
// (UC-AGT-01), the gate every act of a registered agent passes - its policy, budget and kill
// switches, the person it acts for (UC-AGT-02) - the acts held for a person's approval, the
// evaluation an agent passes before it goes live (UC-AGT-04), and the provenance of an accepted AI
// value (UC-AGT-05, ai-first.md §2.1 AI-2).
//
// Administering agents asks MANAGE_MEMBERS at the workspace: an agent is a way into it, as a
// service account is. The person an agent acts for may stop it and decide what it waits on. A
// purpose, a case's words or a held act's input are the workspace's data: never in the audit trail
// or a log (rule 10).
package agent

import (
	"context"
	"errors"
	"time"

	agentrepo "github.com/Jersyfi/hubtask/core/application/repository/agent"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes of the module.
const (
	scopeRead  = "agents:read"
	scopeWrite = "agents:write"
)

const (
	agentTarget   = "agent"
	modelTarget   = "agent_model"
	requestTarget = "agent_request"

	maxListed  = 500
	maxActions = 200
)

// The stable audit codes (audit.md §2).
const (
	ModelRegisteredAction audit.Action = "agent_model.registered"
	ModelChangedAction    audit.Action = "agent_model.changed"
	ModelsReadAction      audit.Action = "agent_model.read"
	RegisteredAction      audit.Action = "agent.registered"
	ChangedAction         audit.Action = "agent.changed"
	ApprovedAction        audit.Action = "agent.approved"
	ActivatedAction       audit.Action = "agent.activated"
	SuspendedAction       audit.Action = "agent.suspended"
	RetiredAction         audit.Action = "agent.retired"
	HaltSetAction         audit.Action = "agent.halt_set"
	ReadAction            audit.Action = "agent.read"
	RequestApprovedAction audit.Action = "agent.request_approved"
	RequestDeclinedAction audit.Action = "agent.request_declined"
	CaseAddedAction       audit.Action = "agent.case_added"
	CaseRemovedAction     audit.Action = "agent.case_removed"
	EvaluatedAction       audit.Action = "agent.evaluated"
	ProvenanceReadAction  audit.Action = "ai.provenance_read"
)

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Catalogue is the registry as this package needs it: the door a held act is performed through
// when a person approves it, and a service account created for a new agent.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Desk is what every use case of the module shares.
type Desk struct {
	Store      agentrepo.Store
	Catalogue  Catalogue
	Authorizer Authorizer
	// Sources is ReadAiSource's use case, run as the registering person to hold a model to what
	// the workspace's AI allows; nil holds it to nothing.
	Sources    usecase.Handler
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Text       text.Normalizer
}

func workspacePath() []identity.Scope { return []identity.Scope{identity.TenantScope()} }

// administer asks what administering agents asks: MANAGE_MEMBERS at the workspace.
func (d Desk) administer(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope, targetType string, id shared.ID) error {
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers, Path: workspacePath(), Action: action,
		TokenScope: scope, TargetType: targetType, TargetID: id,
	})
}

// ownerOrAdminister lets the person an agent acts for through, and asks anybody else what
// administering asks. The token scope is asked of both.
func (d Desk) ownerOrAdminister(ctx context.Context, actor appshared.ActorContext, a domain.Agent, action audit.Action, scope string) error {
	if actor.AccountID == a.OwnerID && actor.OnBehalfOf.IsZero() {
		return actor.RequireScope(scope)
	}
	return d.administer(ctx, actor, action, scope, agentTarget, a.ID)
}

func (d Desk) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID, severity audit.Severity, changes ...audit.Change) error {
	if severity == "" {
		severity = audit.SeverityInfo
	}
	return d.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: d.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: severity,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

func open(field, to string) audit.Change {
	return audit.Change{Field: field, Classification: audit.Open, To: to}
}

// find reads an agent, and answers not found in the words a missing one gets.
func (d Desk) find(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Agent, error) {
	var found domain.Agent
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		found, err = d.Store.FindAgent(ctx, id)
		return err
	})
	return found, err
}

// modelRevision answers the agent's model's revision now, zero for an agent with none.
func (d Desk) modelRevision(ctx context.Context, a domain.Agent) (int, bool, error) {
	if a.ModelID.IsZero() {
		return 0, false, nil
	}
	m, err := d.Store.FindModel(ctx, a.ModelID)
	if errors.Is(err, shared.ErrNotFound) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	return m.Revision, m.Retired, nil
}

func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func moduleOf() module.Key { return module.Agents }

func notAgent(actor appshared.ActorContext, code string) error {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		// No agent approves, puts live or certifies an agent - itself or another (G1, G3).
		return shared.ErrForbidden.WithDetail(code)
	}
	return nil
}
