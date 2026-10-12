// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// A person's own view of keeping their content out of AI (UC-PRV-05 checks 4, 7, 8, 9): whether
// there is anything to switch, the workspace's position and its basis, their own choice, and the
// notice they are owed when their objection stopped taking effect.

const (
	ReadOwnAiPositionName     = "ReadOwnAiPosition"
	AcknowledgeAiPositionName = "AcknowledgeAiPosition"

	aiObjectionTarget = "account"

	// The scope a person's own privacy choices ride on, the consent's own.
	privacyManageScope = "privacy:manage"
	privacyReadScope   = "privacy:read"

	OwnAiPositionReadAction audit.Action = "ai.own_position_read"
	AiNoticeAckAction       audit.Action = "ai.notice_acknowledged"
)

// AiObjectionReader is what the person's two use cases share.
type AiObjectionReader struct {
	Rules      *AiRules
	People     identityrepo.People
	Consents   privacyrepo.Consents
	Notices    privacyrepo.AiNotices
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// OwnAiPosition is one person's view.
type OwnAiPosition struct {
	// Shown is whether the person has anything here at all: AI answers this workspace and more
	// than one person works in it.
	Shown        bool
	PersonOptOut domain.AiPersonOptOut
	Basis        domain.AiOptOutBasis
	Contact      string
	Since        time.Time
	// KeepingOut is the person's own choice - their newest ai_processing record is a withdrawal -
	// and KeepingOutSince when they made it. Whether it takes effect is InEffect.
	KeepingOut      bool
	KeepingOutSince time.Time
	InEffect        bool
	// NoticePending is the one-time notice: their objection stopped taking effect because the
	// workspace made AI part of everybody's work, and they have not acknowledged that yet.
	NoticePending bool
}

func (r AiObjectionReader) read(ctx context.Context, actor appshared.ActorContext) (OwnAiPosition, error) {
	if actor.AccountID.IsZero() {
		return OwnAiPosition{}, shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	var own OwnAiPosition
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		state, err := r.Rules.Resolve(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		selection, err := r.Rules.Select(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		people, err := r.People.CountPeople(ctx)
		if err != nil {
			return err
		}
		own = OwnAiPosition{
			Shown:        selection.Refusal == "" && people > 1,
			PersonOptOut: state.Rule.PersonOptOut, Basis: state.Rule.Basis,
			Contact: state.OptOutContact, Since: state.OptOutSince,
		}
		latest, err := r.Consents.Latest(ctx, actor.AccountID, privacydomain.PurposeAi)
		switch {
		case errors.Is(err, shared.ErrNotFound):
		case err != nil:
			return err
		default:
			own.KeepingOut = !latest.Granted
			if own.KeepingOut {
				own.KeepingOutSince = latest.GrantedAt
			}
		}
		own.InEffect = own.KeepingOut && people > 1 && state.Rule.OptOutOffered()
		if own.KeepingOut && !state.Rule.OptOutOffered() && !state.OptOutSince.IsZero() && r.Notices != nil {
			noticed, err := r.Notices.NoticedAt(ctx, actor.AccountID)
			if err != nil {
				return err
			}
			own.NoticePending = noticed.Before(state.OptOutSince)
		}
		return nil
	})
	return own, err
}

// ReadOwnAiPosition answers the person's view.
type ReadOwnAiPosition struct{ Reader AiObjectionReader }

// Execute reads it. No permission beyond being somebody: it is the caller's own choice, and the
// workspace's position is what every person here is entitled to see (P-14).
func (h ReadOwnAiPosition) Execute(ctx context.Context, actor appshared.ActorContext) (OwnAiPosition, error) {
	if actor.AccountID.IsZero() {
		return OwnAiPosition{}, shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	if err := actor.RequireScope(privacyReadScope); err != nil {
		return OwnAiPosition{}, err
	}
	return h.Reader.read(ctx, actor)
}

// OwnAiPositionOutput is the read shape.
func OwnAiPositionOutput(own OwnAiPosition) usecase.Output {
	out := usecase.Output{
		"shown":          own.Shown,
		"person_opt_out": string(own.PersonOptOut),
		"keeping_out":    own.KeepingOut,
		"in_effect":      own.InEffect,
		"notice_pending": own.NoticePending,
	}
	if own.Basis != "" {
		out["basis"] = string(own.Basis)
	}
	if own.Contact != "" {
		out["contact"] = own.Contact
	}
	if !own.Since.IsZero() {
		out["since"] = own.Since
	}
	if !own.KeepingOutSince.IsZero() {
		out["keeping_out_since"] = own.KeepingOutSince
	}
	return out
}

func (h ReadOwnAiPosition) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadOwnAiPositionName,
		Summary: "The caller's own view of keeping their content out of AI: whether there is " +
			"anything to choose here, whether the workspace lets each person keep their content " +
			"out or made AI part of everybody's work - with the legal basis and whom to ask - " +
			"the caller's own choice, whether it takes effect, and whether they still have to be " +
			"told that it stopped taking effect.",
		SideEffects: "None. Reads only.",
		TokenScope:  privacyReadScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: OwnAiPositionReadAction, TargetType: aiObjectionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadOwnAiPosition) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	own, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return OwnAiPositionOutput(own), nil
}

// AcknowledgeAiPosition records that the person read the notice. A read receipt rather than an
// act: it is not audited, and acknowledging twice is acknowledging once.
type AcknowledgeAiPosition struct{ Reader AiObjectionReader }

// Execute records it and answers the view afterwards.
func (h AcknowledgeAiPosition) Execute(ctx context.Context, actor appshared.ActorContext) (OwnAiPosition, error) {
	if actor.AccountID.IsZero() {
		return OwnAiPosition{}, shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	if err := actor.RequireScope(privacyManageScope); err != nil {
		return OwnAiPosition{}, err
	}
	r := h.Reader
	if err := r.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return r.Notices.Notice(ctx, actor.AccountID, r.Clock.Now())
	}); err != nil {
		return OwnAiPosition{}, err
	}
	return r.read(ctx, actor)
}

func (h AcknowledgeAiPosition) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AcknowledgeAiPositionName,
		Summary: "Records that the caller read the notice that their choice to keep their content " +
			"out of AI stopped taking effect, because the workspace made AI part of everybody's " +
			"work. Acknowledging twice is acknowledging once.",
		SideEffects: "Records the moment the notice was read. Nothing else changes.",
		TokenScope:  privacyManageScope,
		Audit: usecase.AuditDeclaration{
			Action: AiNoticeAckAction, TargetType: aiObjectionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A read receipt of a notice about the caller's own choice, not a change to an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AcknowledgeAiPosition) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	own, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return OwnAiPositionOutput(own), nil
}
