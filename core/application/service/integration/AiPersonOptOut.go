// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetAiPersonOptOutName = "SetAiPersonOptOut"

	// AiPersonOptOutSetAction is the workspace's position on people keeping their own content out
	// of AI, with the legal basis it named - never whom to ask, which is free text (rule 10).
	AiPersonOptOutSetAction audit.Action = "ai.person_opt_out_set"
)

// SetAiPersonOptOut sets whether each person may keep their own content out of AI, or makes AI
// part of everybody's work on a legal basis the workspace names (data-protection.md §4.1).
//
// The position lives in the archived workspace settings beside the narrowing: it is the
// controller's documented decision, and a restore puts back what was decided. Moving into
// NOT_OFFERED stamps the moment, which is what makes each objecting person's notice pending.
type SetAiPersonOptOut struct{ Writer AiSourceWriter }

// SetAiPersonOptOutCommand is the input, typed.
type SetAiPersonOptOutCommand struct {
	PersonOptOut domain.AiPersonOptOut
	Basis        domain.AiOptOutBasis
	Contact      string
}

// Execute writes the position.
func (h SetAiPersonOptOut) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetAiPersonOptOutCommand,
) (AiState, error) {
	w := h.Writer
	request := aiSourceRequest(AiPersonOptOutSetAction)
	if err := w.Authorizer.Authorize(ctx, actor, request); err != nil {
		return AiState{}, err
	}
	position, err := domain.NewAiPosition(cmd.PersonOptOut, cmd.Basis, cmd.Contact)
	if err != nil {
		return AiState{}, err
	}
	now := w.Clock.Now()

	var state AiState
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := w.Rules.Resolve(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if err := before.Rule.PositionChange(position); err != nil {
			return err
		}
		workspace, err := w.Rules.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		settings := workspace.Settings
		settings.AiPersonOptOut = string(position.PersonOptOut)
		settings.AiOptOutBasis = string(position.Basis)
		settings.AiOptOutContact = position.Contact
		settings.AiOptOutSince = sinceFor(before, position, now)
		workspace.Settings = settings
		written, err := w.Rules.Workspaces.Update(ctx, workspace, workspace.Version, now)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrConflict.WithDetail("workspace.version_conflict")
		}
		if state, err = w.Rules.Resolve(ctx, actor.TenantID); err != nil {
			return err
		}
		if state.KeepingOut, err = w.Rules.keepingOut(ctx); err != nil {
			return err
		}
		// Whose entries may be embedded moves with the position, so the pass is seeded by the
		// write that moved it (ADR-0072 §6's discipline).
		if before.Rule.PersonOptOut != state.Rule.PersonOptOut && w.Reindex != nil {
			if err := w.Reindex.Reseed(ctx, actor.TenantID); err != nil {
				return err
			}
		}
		// Back to letting each person keep their content out: the standing objections take effect
		// again in this transaction, their vectors with them.
		if !before.Rule.OptOutOffered() && state.Rule.OptOutOffered() {
			if err := h.forgetStanding(ctx); err != nil {
				return err
			}
		}
		return h.record(ctx, actor, before, state)
	})
	if err != nil {
		return AiState{}, err
	}
	return state, nil
}

// sinceFor keeps the moment AI became part of everybody's work while it stays so - a changed basis
// or contact tells nobody again - and stamps a new one when the workspace moves into it.
func sinceFor(before AiState, position domain.AiPosition, now time.Time) time.Time {
	if position.PersonOptOut != domain.AiOptOutNotOffered {
		return time.Time{}
	}
	if !before.Rule.OptOutOffered() && !before.OptOutSince.IsZero() {
		return before.OptOutSince
	}
	return now.UTC()
}

func (h SetAiPersonOptOut) record(ctx context.Context, actor appshared.ActorContext, before, after AiState) error {
	changes := []audit.Change{
		{Field: "person_opt_out", Classification: audit.Open,
			From: string(before.Rule.PersonOptOut), To: string(after.Rule.PersonOptOut)},
	}
	if before.Rule.Basis != after.Rule.Basis {
		changes = append(changes, audit.Change{Field: "basis", Classification: audit.Open,
			From: string(before.Rule.Basis), To: string(after.Rule.Basis)})
	}
	return h.Writer.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: h.Writer.Clock.Now(),
		Action: AiPersonOptOutSetAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: aiSourceTarget, TargetID: actor.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// Descriptor registers the position in every channel.
func (h SetAiPersonOptOut) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetAiPersonOptOutName,
		Summary: "Sets whether each person may keep their own content out of AI (OFFERED, the " +
			"default) or makes AI part of everybody's work (NOT_OFFERED) on a legal basis this " +
			"workspace names: an employment contract, a works agreement or a legal obligation. " +
			"Under NOT_OFFERED an objection recorded earlier stops taking effect, and each person " +
			"it affects is told once. Refused under the installation's or a plan's lock.",
		SideEffects: "Writes the position into the workspace's settings and an audit entry; queues " +
			"rebuilding search by meaning when whose entries may be embedded changes.",
		TokenScope: aiManage,
		Input: []usecase.Field{
			{Name: "person_opt_out", Kind: usecase.KindString, Required: true,
				Enum:        enumOf(domain.AiPersonOptOutValues()),
				Description: "OFFERED or NOT_OFFERED."},
			{Name: "basis", Kind: usecase.KindString,
				Enum:        enumOf(domain.AiOptOutBases()),
				Description: "The legal basis, required with NOT_OFFERED."},
			{Name: "contact", Kind: usecase.KindString,
				Description: "Whom people may ask about it, up to 200 characters. Optional."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AiPersonOptOutSetAction, TargetType: aiSourceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's AI configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetAiPersonOptOut) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	state, err := h.Execute(ctx, actor, SetAiPersonOptOutCommand{
		PersonOptOut: domain.AiPersonOptOut(in.String("person_opt_out")),
		Basis:        domain.AiOptOutBasis(in.String("basis")),
		Contact:      in.String("contact"),
	})
	if err != nil {
		return nil, err
	}
	return AiSourceOutput(state), nil
}

func (h SetAiPersonOptOut) forgetStanding(ctx context.Context) error {
	w := h.Writer
	if w.Embeddings == nil || w.Rules.Consents == nil {
		return nil
	}
	standing, err := w.Rules.Consents.Standing(ctx, privacydomain.PurposeAi)
	if err != nil || len(standing) == 0 {
		return err
	}
	creators := make([]shared.ID, 0, len(standing))
	for _, objector := range standing {
		creators = append(creators, objector.AccountID)
	}
	_, err = w.Embeddings.Forget(ctx, creators)
	return err
}
