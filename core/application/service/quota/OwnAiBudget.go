// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetOwnAiBudgetName = "SetOwnAiBudget"
	// OwnAiBudgetSetAction records the workspace's own budget for its own model moving, from what
	// it was to what it is.
	OwnAiBudgetSetAction audit.Action = "ai.own_budget_set"

	ownBudgetField  = "own_tokens_per_day"
	ownBudgetTarget = "ai_budget"
	aiManage        = "ai:manage"
)

// SetOwnAiBudget is the workspace's own optional budget for its own AI model (ADR-0072 §4,
// UC-AI-06 check 6): how many tokens a day the workspace lets its own model spend. The workspace's
// administrator decides it - never the operator, whose budget bounds only the models the
// installation offers. Nothing means no budget, the default.
type SetOwnAiBudget struct {
	Store      repository.Store
	Usage      repository.Usage
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Tenancy    env.TenancyMode
}

// Execute sets the budget (nil clears it) and answers the two AI standings, offered and own.
func (h SetOwnAiBudget) Execute(
	ctx context.Context, actor appshared.ActorContext, tokens *int64,
) ([]Standing, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     OwnAiBudgetSetAction,
		TokenScope: aiManage,
		TargetType: ownBudgetTarget,
	}); err != nil {
		return nil, err
	}
	if tokens != nil && *tokens < 1 {
		return nil, shared.ErrValidation.WithDetail("ai.budget_invalid").
			WithFields(shared.FieldError{Path: "/" + ownBudgetField, Code: "ai.budget_invalid"})
	}

	var standings []Standing
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := h.Store.Overrides(ctx)
		if err != nil {
			return err
		}
		now := h.Clock.Now()
		if err := h.Store.SetOwnAiBudget(ctx, tokens, now); err != nil {
			return err
		}
		if err := h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: OwnAiBudgetSetAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: ownBudgetTarget, TargetID: actor.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: AiOwnTokensPerDay, Classification: audit.Open,
				From: budgetLabel(before.AiOwnTokensPerDay), To: budgetLabel(tokens),
			}),
		}); err != nil {
			return err
		}
		all, err := Standings(ctx, h.Store, h.Usage, h.Tenancy, now)
		if err != nil {
			return err
		}
		for _, standing := range all {
			if standing.Quota == AiTokensPerDay || standing.Quota == AiOwnTokensPerDay {
				standings = append(standings, standing)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return standings, nil
}

func budgetLabel(tokens *int64) string {
	if tokens == nil {
		return "none"
	}
	return strconv.FormatInt(*tokens, 10)
}

// Descriptor is the catalogue entry.
func (h SetOwnAiBudget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetOwnAiBudgetName,
		Summary: "Sets how many tokens a day this workspace's own AI model may spend, in the " +
			"workspace's own day - or, without a number, lets it spend without a budget, the " +
			"default. The installation's budget bounds only the models it offers; this one is the " +
			"workspace's alone.",
		SideEffects: "Writes the budget and an audit entry. A model past it pauses until the " +
			"workspace's next midnight; nothing else changes.",
		TokenScope: aiManage,
		Input: []usecase.Field{
			{Name: ownBudgetField, Kind: usecase.KindInt,
				Description: "Tokens a day, at least 1. Absent or null: no budget."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OwnAiBudgetSetAction, TargetType: ownBudgetTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a budget is workspace configuration, not an item; the history is an item's.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetOwnAiBudget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	var tokens *int64
	if in.Present(ownBudgetField) {
		value := int64(in.Int(ownBudgetField))
		tokens = &value
	}
	standings, err := h.Execute(ctx, actor, tokens)
	if err != nil {
		return nil, err
	}
	return usecase.Output{"data": StandingOutputs(standings)}, nil
}
