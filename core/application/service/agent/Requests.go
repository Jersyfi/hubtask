// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	ListAgentRequestsName  = "ListAgentRequests"
	DecideAgentRequestName = "DecideAgentRequest"
	ListAgentActionsName   = "ListAgentActions"
	ReadAgentMeteringName  = "ReadAgentMetering"
)

// ListAgentRequests answers the acts waiting for a person.
type ListAgentRequests struct{ Desk Desk }

func (h ListAgentRequests) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListAgentRequestsName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "The acts agents attempted that wait for a person, newest first, with the use case and " +
			"its input - the preview of what approving would do. Whoever administers agents reads all of " +
			"them; anybody else the requests of the agents acting for them.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "status", Kind: usecase.KindString, Enum: []string{"PENDING", "APPROVED", "DECLINED", "FAILED"}, Description: "PENDING without."},
			{Name: "agent_id", Kind: usecase.KindID, Description: "Only this agent's."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: requestTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			agent, err := optionalID(in, "agent_id")
			if err != nil {
				return nil, err
			}
			status := domain.RequestStatus(in.String("status"))
			if status == "" {
				status = domain.RequestPending
			}
			if err := actor.RequireScope(scopeRead); err != nil {
				return nil, err
			}
			all := d.administer(ctx, actor, ReadAction, scopeRead, requestTarget, "") == nil
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				owned := map[shared.ID]bool{}
				if !all {
					agents, err := d.Store.ListAgents(ctx, actor.AccountID, "", maxListed)
					if err != nil {
						return err
					}
					for _, a := range agents {
						owned[a.ID] = true
					}
				}
				requests, err := d.Store.ListRequests(ctx, status, agent, maxListed)
				if err != nil {
					return err
				}
				for _, r := range requests {
					if all || owned[r.AgentID] {
						rows = append(rows, RequestOutput(r))
					}
				}
				return nil
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// DecideAgentRequest approves or declines a held act. Approving performs it as the approving
// person, through the registry like any act of theirs: nobody approves what they could not do.
type DecideAgentRequest struct{ Desk Desk }

func (h DecideAgentRequest) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       DecideAgentRequestName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Approves or declines an act an agent attempted that waits for a person. Approving " +
			"performs it as the approving person, with their rights and their audit entries, and " +
			"answers what it answered; one their rights refuse is recorded FAILED with the refusal's " +
			"code. The agent's owner or whoever administers agents decides; no agent does.",
		SideEffects: "Performs the held use case on approval; writes the request and an audit entry.",
		TokenScope:  scopeWrite,
		Input: []usecase.Field{
			{Name: "request_id", Kind: usecase.KindID, Required: true},
			{Name: "approve", Kind: usecase.KindBool, Required: true, Description: "true performs the act, false declines it."},
		},
		Audit:    usecase.AuditDeclaration{Action: RequestApprovedAction, TargetType: requestTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "the request is no entry; the act it performs writes its own."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("request_id")
			if err != nil {
				return nil, err
			}
			approve := in.Bool("approve")
			action := RequestApprovedAction
			status := domain.RequestApproved
			if !approve {
				action, status = RequestDeclinedAction, domain.RequestDeclined
			}
			if err := notAgent(actor, "agents.agent_decides_not"); err != nil {
				return nil, err
			}
			var r domain.Request
			var a domain.Agent
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				if r, err = d.Store.FindRequest(ctx, id); err != nil {
					return err
				}
				a, err = d.Store.FindAgent(ctx, r.AgentID)
				return err
			})
			if err != nil {
				return nil, err
			}
			if err := d.ownerOrAdminister(ctx, actor, a, action, scopeWrite); err != nil {
				return nil, err
			}
			// Claimed first, so two people approving perform it once.
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				decided, err := r.Decide(status, actor.AccountID, d.Clock.Now())
				if err != nil {
					return err
				}
				written, err := d.Store.DecideRequest(ctx, decided, r.Version)
				if err != nil {
					return err
				}
				if !written {
					return shared.ErrConflict.WithDetail("agents.request_decided")
				}
				r = decided
				if err := d.requestChanged(ctx, actor, id); err != nil {
					return err
				}
				return d.record(ctx, actor, action, requestTarget, id, audit.SeverityNotice,
					open("agent_id", a.ID.String()), open("use_case", r.UseCase), open("class", string(r.Class)))
			})
			if err != nil {
				return nil, err
			}
			out := RequestOutput(r)
			if !approve {
				return out, nil
			}
			answer, err := d.Catalogue.Invoke(ctx, r.UseCase, actor, usecase.Input(r.Input))
			if err != nil {
				code := shared.AsError(err).DetailCode
				if code == "" {
					code = shared.AsError(err).Code
				}
				if failed := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
					if err := d.Store.FailRequest(ctx, id, code); err != nil {
						return err
					}
					return d.requestChanged(ctx, actor, id)
				}); failed != nil {
					return nil, failed
				}
				return nil, err
			}
			out["result"] = answer
			return out, nil
		}),
	}
}

// ListAgentActions answers an agent's ledger.
type ListAgentActions struct{ Desk Desk }

func (h ListAgentActions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListAgentActionsName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "An agent's ledger, newest first: every act it attempted - the use case, its class, " +
			"whether it ran, ran with its owner told, waited or was refused and why, and the tokens a " +
			"run spent. Never an input or an answer. Its owner and whoever administers agents read it.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Required: true},
			{Name: "before", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "Only acts before this moment, for the next page."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			var before *time.Time
			if raw := in.String("before"); raw != "" {
				at, err := time.Parse(time.RFC3339, raw)
				if err != nil {
					return nil, shared.ErrValidation.WithDetail("agents.before_invalid").
						WithFields(shared.FieldError{Path: "/before", Code: "agents.before_invalid"})
				}
				before = &at
			}
			a, err := d.find(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := d.ownerOrAdminister(ctx, actor, a, ReadAction, scopeRead); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				actions, err := d.Store.ListActions(ctx, id, before, maxActions)
				for _, act := range actions {
					rows = append(rows, usecase.Output{
						"id": act.ID.String(), "use_case": act.UseCase, "class": string(act.Class),
						"decision": string(act.Decision), "code": act.Code, "tokens": act.Tokens,
						"request_id": idOrNil(act.RequestID), "at": act.At.UTC(),
					})
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// ReadAgentMetering answers what the workspace's agents did and spent per day (G10).
type ReadAgentMetering struct{ Desk Desk }

func (h ReadAgentMetering) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReadAgentMeteringName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Per agent and day over the last days (30 without, at most 90): the acts that ran, " +
			"waited and were refused, and the tokens spent. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Description: "Only this agent."},
			{Name: "days", Kind: usecase.KindInt, Description: "How many days back; 30 without, at most 90."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			agent, err := optionalID(in, "agent_id")
			if err != nil {
				return nil, err
			}
			days := in.Int("days")
			if days <= 0 {
				days = 30
			}
			if days > 90 {
				days = 90
			}
			if err := d.administer(ctx, actor, ReadAction, scopeRead, agentTarget, agent); err != nil {
				return nil, err
			}
			_, today := windows(d.Clock.Now())
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				metering, err := d.Store.Metering(ctx, agent, today.AddDate(0, 0, -(days-1)), 90*maxListed)
				for _, m := range metering {
					rows = append(rows, usecase.Output{
						"agent_id": m.AgentID.String(), "day": m.Day.Format(time.DateOnly), "allowed": m.Allowed,
						"held": m.Held, "refused": m.Refused, "tokens": m.Tokens,
					})
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// RequestOutput is a held act as every door answers it.
func RequestOutput(r domain.Request) usecase.Output {
	input := r.Input
	if input == nil {
		input = map[string]any{}
	}
	return usecase.Output{
		"id": r.ID.String(), "agent_id": r.AgentID.String(), "use_case": r.UseCase, "class": string(r.Class),
		"input": input, "status": string(r.Status), "decided_by": idOrNil(r.DecidedBy),
		"decided_at": timeOrNil(r.DecidedAt), "code": r.Code, "created_at": r.CreatedAt.UTC(),
	}
}
