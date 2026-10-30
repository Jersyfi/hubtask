// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"errors"
	"slices"
	"time"

	agentrepo "github.com/Jersyfi/hubtask/core/application/repository/agent"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Gate holds every act of a registered agent to its governance (UC-AGT-02): on the registry
// every door comes through - REST, MCP, `hubctl`, a rule's action - one place, as the module and
// plan gates are. An actor that is no registered agent passes untouched.
//
// An admitted act runs with the agent's scopes narrowed to those it may use and with the person it
// acts for named, so the authoriser asks of both (G2). An act its policy holds is stored with its
// input and answered `agents.approval_required` with the request's identifier; a person performs
// it by approving (DecideAgentRequest). Every act is a row of the ledger: what it was, its class,
// what became of it - never its input or answer (rule 10).
type Gate struct{ Desk Desk }

// whileNotLive are the acts an agent may attempt before it is live: reading itself and its
// evaluation set, and answering that set (UC-AGT-04).
var whileNotLive = []string{ReadAgentName, ListAgentCasesName, RecordAgentEvaluationName}

type admittedKey struct{}

// admitted marks a context whose act the gate already judged: the use cases the act calls on the
// way - an apply's blocks, an acceptance's write - are part of it, not new acts.
func admitted(ctx context.Context) bool { v, _ := ctx.Value(admittedKey{}).(bool); return v }

// Wrap puts the gate inside the composition root's middleware.
func (g Gate) Wrap(observe usecase.Middleware) usecase.Middleware {
	return func(descriptor usecase.Descriptor, next usecase.Handler) usecase.Handler {
		class := ClassOf(descriptor)
		name := descriptor.Name
		handler := usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if admitted(ctx) {
				return next.Invoke(ctx, actor, in)
			}
			if !mayBeAgent(actor) {
				if err := g.refuseUnregistered(ctx, actor, name); err != nil {
					return nil, err
				}
				return next.Invoke(ctx, actor, in)
			}
			a, found, err := g.agentOf(ctx, actor)
			if err != nil {
				return nil, err
			}
			if !found {
				if err := g.refuseUnregistered(ctx, actor, name); err != nil {
					return nil, err
				}
				return next.Invoke(ctx, actor, in)
			}
			if slices.Contains(whileNotLive, name) && a.Status != domain.StatusRetired {
				return next.Invoke(context.WithValue(ctx, admittedKey{}, true), g.narrowed(actor, a), in)
			}
			decision, request, refusal, err := g.judge(ctx, actor, a, name, class, in)
			if err != nil {
				return nil, err
			}
			switch decision {
			case domain.DecisionRefused:
				return nil, refusal
			case domain.DecisionHeld:
				return nil, shared.ErrForbidden.WithDetail("agents.approval_required").
					WithParams(map[string]string{"request_id": request.String(), "use_case": name})
			}
			return next.Invoke(context.WithValue(ctx, admittedKey{}, true), g.narrowed(actor, a), in)
		})
		if observe == nil {
			return handler
		}
		return observe(descriptor, handler)
	}
}

// refuseUnregistered refuses an act through MCP by a caller no agent names - a person's own token,
// an unregistered service account - where the workspace takes registered agents only
// (agents.registered_only, UC-AGT-02). Every other door, and every workspace that left it off, is
// untouched. The refusal is in the trail, as every refusal of the authoriser is.
func (g Gate) refuseUnregistered(ctx context.Context, actor appshared.ActorContext, name string) error {
	if actor.Channel != string(work.ChannelMCP) || !actor.IsAuthenticated() || actor.TenantID.IsZero() {
		return nil
	}
	var on bool
	err := g.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		r, err := g.Desk.registeredOnly(ctx, actor.TenantID)
		on = domain.RegisteredOnlyOf(r)
		return err
	})
	if err != nil || !on {
		return err
	}
	if g.Desk.Audit != nil {
		_ = g.Desk.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: g.Desk.Clock.Now(), Action: UnregisteredRefusedAction,
			Outcome: audit.OutcomeDenied, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: policyTarget, TargetID: actor.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(open("use_case", name)),
		})
	}
	return shared.ErrForbidden.WithDetail("agents.registration_required")
}

// ClassOf is the class of an act (G3): a read; an act that cannot be taken back - destructive, or
// asking a fresh sign-in; or a write that can.
func ClassOf(d usecase.Descriptor) domain.Class {
	switch {
	case d.ReadOnly:
		return domain.ClassRead
	case d.Destructive || d.StepUp != "":
		return domain.ClassIrreversible
	}
	return domain.ClassReversible
}

// mayBeAgent is whether the actor can be a registered agent: one acting as a service account or
// through MCP. A person signed in, a rule's run or the system is none.
func mayBeAgent(actor appshared.ActorContext) bool {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return false
	}
	return actor.Kind == appshared.ActorServiceAccount || actor.Kind == appshared.ActorAIAgent
}

func (g Gate) agentOf(ctx context.Context, actor appshared.ActorContext) (domain.Agent, bool, error) {
	var a domain.Agent
	err := g.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		a, err = g.Desk.Store.FindAgentByAccount(ctx, actor.AccountID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return domain.Agent{}, false, nil
	}
	return a, err == nil, err
}

// narrowed is the actor an admitted act runs as: the agent's scopes only, the person it acts for
// named, and marked an agent's act wherever it came from.
func (g Gate) narrowed(actor appshared.ActorContext, a domain.Agent) appshared.ActorContext {
	actor.Scopes = a.Narrow(actor.Scopes)
	actor.OnBehalfOf = a.OwnerID
	actor.AsAgent = a.ID
	actor.Kind = appshared.ActorAIAgent
	actor.Present = false
	return actor
}

// judge decides an act and writes it to the ledger - and a held act's request - in one
// transaction, so a burst of acts counts each one.
func (g Gate) judge(
	ctx context.Context, actor appshared.ActorContext, a domain.Agent, name string, class domain.Class, in usecase.Input,
) (domain.Decision, shared.ID, error, error) {
	d := g.Desk
	var decision domain.Decision
	var refusal error
	var request shared.ID
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		situation, err := d.situation(ctx, a)
		if err != nil {
			return err
		}
		now := d.Clock.Now()
		decision, refusal = a.Admit(class, situation)
		action := domain.Action{ID: d.IDs.NewID(), AgentID: a.ID, UseCase: name, Class: class, Decision: decision, At: now}
		if refusal != nil {
			action.Code = shared.AsError(refusal).DetailCode
		}
		if decision == domain.DecisionHeld {
			request = d.IDs.NewID()
			action.RequestID = request
			if err := d.Store.InsertRequest(ctx, domain.Request{
				ID: request, AgentID: a.ID, UseCase: name, Class: class, Input: heldInput(in),
				Status: domain.RequestPending, CreatedAt: now, Version: 1,
			}); err != nil {
				return err
			}
			if err := d.requestChanged(ctx, actor, request); err != nil {
				return err
			}
		}
		if decision == domain.DecisionNotified && situation.Usage.NotifiedLastHour == 0 && d.Notices != nil {
			if err := d.Notices.AgentActed(ctx, actor.TenantID, a.ID, a.OwnerID, now); err != nil {
				return err
			}
		}
		return d.Store.RecordAction(ctx, action)
	})
	return decision, request, refusal, err
}

// heldInput is what a held act keeps: its input without a proof of a fresh sign-in, which is the
// agent's and would not be the approving person's.
func heldInput(in usecase.Input) map[string]any {
	kept := make(map[string]any, len(in))
	for key, value := range in {
		if key != "step_up_token" {
			kept[key] = value
		}
	}
	return kept
}

// windows are the moments the budget counts from: the start of this hour, and of this day (UTC).
func windows(now time.Time) (hour, day time.Time) {
	now = now.UTC()
	return now.Truncate(time.Hour), time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func haltOf(halted bool, by shared.ID, at time.Time) agentrepo.Halt {
	return agentrepo.Halt{Halted: halted, SetBy: by, SetAt: at}
}
