// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetAiArrivalProposalsName = "SetAiArrivalProposals"

	// AiArrivalProposalsSetAction is the workspace switching proposals on arrival on or off: who
	// decided that what arrives in the jumble is read by AI without anybody asking, and when.
	AiArrivalProposalsSetAction audit.Action = "ai.arrival_proposals_set"
)

// SetAiArrivalProposals switches whether each new jumble entry gets a proposal without anybody
// asking (UC-JUM-07 check 10).
//
// The workspace's own switch, off by default, and set by nobody above it: what of a workspace's
// content is sent to a model is the workspace's decision alone (P-14), so no installation or plan
// value reaches it and no lock holds it on. In the archived settings beside the opt-out position,
// so a restore puts back what was decided - and a document without the key reads off. It is not
// consent: content is still sent only where the workspace consented and its source answers, which
// the run checks for each arrival.
type SetAiArrivalProposals struct{ Writer AiSourceWriter }

// Execute writes the switch.
func (h SetAiArrivalProposals) Execute(
	ctx context.Context, actor appshared.ActorContext, prepare bool,
) (AiState, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, aiSourceRequest(AiArrivalProposalsSetAction)); err != nil {
		return AiState{}, err
	}
	now := w.Clock.Now()

	var state AiState
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		workspace, err := w.Rules.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		before := workspace.Settings.AiArrivalProposals
		workspace.Settings.AiArrivalProposals = prepare
		written, err := w.Rules.Workspaces.Update(ctx, workspace, workspace.Version, now)
		if err != nil {
			return err
		}
		if !written {
			// Another write moved the workspace since it was read: the caller reads again and
			// decides on what stands now.
			return shared.ErrConflict.WithDetail("workspace.version_conflict")
		}
		if state, err = w.Rules.Resolve(ctx, actor.TenantID); err != nil {
			return err
		}
		if state.KeepingOut, err = w.Rules.keepingOut(ctx); err != nil {
			return err
		}
		return w.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: AiArrivalProposalsSetAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: aiSourceTarget, TargetID: actor.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: "prepare_on_arrival", Classification: audit.Open,
				From: strconv.FormatBool(before), To: strconv.FormatBool(prepare),
			}),
		})
	})
	if err != nil {
		return AiState{}, err
	}
	return state, nil
}

// Descriptor registers the switch in every channel.
func (h SetAiArrivalProposals) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       SetAiArrivalProposalsName,
		Summary: "Switches whether AI prepares a proposal for each new jumble entry as it arrives, " +
			"without anybody asking - off by default, and the workspace's own decision. Content is " +
			"still sent only where the workspace consented and its AI answers, nothing of a person " +
			"who keeps their content out travels, and each arrival counts against the AI budget and " +
			"the daily number of arrivals asked about.",
		SideEffects: "Writes the switch into the workspace's settings and an audit entry.",
		TokenScope:  aiManage,
		Input: []usecase.Field{
			{Name: "prepare", Kind: usecase.KindBool, Required: true,
				Description: "True prepares proposals on arrival, false stops it."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AiArrivalProposalsSetAction, TargetType: aiSourceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's AI configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetAiArrivalProposals) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	state, err := h.Execute(ctx, actor, in.Bool("prepare"))
	if err != nil {
		return nil, err
	}
	return AiSourceOutput(state), nil
}
