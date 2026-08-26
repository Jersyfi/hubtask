// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	PlaceLegalHoldName   = "PlaceLegalHold"
	ReleaseLegalHoldName = "ReleaseLegalHold"
	ListLegalHoldsName   = "ListLegalHolds"

	holdTarget = "legal_hold"

	// HoldPlacedAction and HoldReleasedAction are the two entries §4.1 asks for. Both warnings:
	// placing one overrides the workspace's own configured periods, and lifting one is the moment
	// the data under it becomes deletable again. Neither is the machinery doing what it was told -
	// they are both somebody deciding something.
	HoldPlacedAction   audit.Action = "lifecycle.hold_placed"
	HoldReleasedAction audit.Action = "lifecycle.hold_released"
)

// Holds is what the three legal hold use cases share.
type Holds struct {
	Holds      repository.HoldWriter
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// PlaceLegalHold stops anything under it being deleted.
type PlaceLegalHold struct{ Holds Holds }

// ReleaseLegalHold lifts one, and records who and why.
type ReleaseLegalHold struct{ Holds Holds }

// ListLegalHolds answers what is frozen, and what has been.
type ListLegalHolds struct{ Holds Holds }

// PlaceLegalHoldCommand is the input, typed.
type PlaceLegalHoldCommand struct {
	Scope   domain.HoldScope
	ScopeID shared.ID
	Reason  string
}

// Execute places the hold.
//
// The owner's right, and it is the sharpest case for that line in the whole system: a hold overrides
// the tenant's own configured retention periods *and* a person emptying their own trash. Somebody
// who can freeze a workspace's data against the workspace's own decisions is exercising the owner's
// authority, not an administrator's (domain-model.md §3.2).
func (h PlaceLegalHold) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd PlaceLegalHoldCommand,
) (domain.LegalHold, error) {
	if err := h.Holds.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     HoldPlacedAction,
		TokenScope: retentionManage,
		TargetType: holdTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return domain.LegalHold{}, err
	}

	hold, err := domain.NewLegalHold(domain.NewHoldInput{
		ID: h.Holds.IDs.NewID(), Scope: cmd.Scope, ScopeID: cmd.ScopeID,
		Reason: cmd.Reason, PlacedBy: actor.AccountID, Now: h.Holds.Clock.Now(),
	})
	if err != nil {
		return domain.LegalHold{}, err
	}

	err = h.Holds.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Holds.Holds.Place(ctx, hold); err != nil {
			return err
		}
		return h.Holds.record(ctx, actor, HoldPlacedAction, hold, hold.PlacedAt, []audit.Change{
			{Field: "scope_kind", Classification: audit.Open, To: string(hold.Scope)},
			{Field: "scope_id", Classification: audit.Open, To: hold.ScopeID.String()},
			// The reason is the whole point of the entry. Operator content rather than user
			// content: it is what somebody wrote about their own obligation, and an auditor
			// reading the trail without it has a date and no case.
			{Field: "reason", Classification: audit.Open, To: hold.Reason},
		})
	})
	if err != nil {
		return domain.LegalHold{}, err
	}
	return hold, nil
}

// Execute lifts the hold.
func (h ReleaseLegalHold) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, reason string,
) (domain.LegalHold, error) {
	if id.IsZero() {
		return domain.LegalHold{}, shared.ErrValidation.WithDetail(domain.CodeHoldNotFound).
			WithFields(shared.FieldError{Path: "/hold_id", Code: domain.CodeHoldNotFound})
	}
	if err := h.Holds.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     HoldReleasedAction,
		TokenScope: retentionManage,
		TargetType: holdTarget,
		TargetID:   id,
	}); err != nil {
		return domain.LegalHold{}, err
	}

	now := h.Holds.Clock.Now()
	var released domain.LegalHold

	err := h.Holds.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		hold, err := h.Holds.Holds.Find(ctx, id)
		if err != nil {
			return err
		}
		released, err = hold.Release(actor.AccountID, reason, now)
		if err != nil {
			return err
		}

		lifted, err := h.Holds.Holds.Release(ctx, released)
		if err != nil {
			return err
		}
		if !lifted {
			// Somebody else lifted it between the read and the write. A conflict rather than a
			// quiet success, because the caller's reading of who lifted it is now wrong.
			return shared.ErrConflict.WithDetail(domain.CodeHoldAlreadyReleased).
				WithParams(map[string]string{"hold_id": id.String()})
		}

		return h.Holds.record(ctx, actor, HoldReleasedAction, released, now, []audit.Change{
			{Field: "scope_kind", Classification: audit.Open, To: string(released.Scope)},
			{
				Field: "released_reason", Classification: audit.Open,
				To: released.ReleasedReason,
			},
			// The reason it was placed, so that the entry lifting a hold reads without a second
			// lookup: an auditor comparing why it went on with why it came off is doing exactly
			// what this pair of entries is for.
			{Field: "reason", Classification: audit.Open, From: released.Reason},
		})
	})
	if err != nil {
		return domain.LegalHold{}, err
	}
	return released, nil
}

// Execute answers the tenant's holds.
func (h ListLegalHolds) Execute(
	ctx context.Context, actor appshared.ActorContext, includeReleased bool,
) ([]domain.LegalHold, error) {
	if err := h.Holds.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     HoldPlacedAction,
		TokenScope: retentionRead,
		TargetType: holdTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return nil, err
	}

	var holds []domain.LegalHold
	err := h.Holds.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		holds, err = h.Holds.Holds.List(ctx, includeReleased)
		return err
	})
	if err != nil {
		return nil, err
	}
	return holds, nil
}

// record writes the entry a hold owes.
func (h Holds) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	hold domain.LegalHold, at time.Time, changes []audit.Change,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: at,
		Action: action, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: holdTarget, TargetID: hold.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// Descriptor registers placing a hold in all three channels.
func (h PlaceLegalHold) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: PlaceLegalHoldName,
		Summary: "Freezes something against every kind of deletion: a retention rule, a person " +
			"emptying their own trash, a hard delete somebody asked for. A hold on the workspace " +
			"covers everything, one on a hub or a collection covers what is below it, and one on " +
			"an entry covers that entry and what hangs off it. Placing one is not an ordinary " +
			"member's power, because it overrides the workspace's own decisions about its data.",
		SideEffects: "Writes the hold and an audit entry carrying the reason. Nothing is deleted " +
			"or changed; things simply stop being deletable.",
		TokenScope: retentionManage,
		Input: []usecase.Field{
			{
				Name: "scope", Kind: usecase.KindString, Required: true,
				Enum:        []string{"TENANT", "CONTAINER", "ITEM", "ACCOUNT"},
				Description: "What the hold covers.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "Which hub, collection or entry. Left out for a hold on the whole " +
					"workspace, which names nothing because it covers everything.",
			},
			{
				Name: "reason", Kind: usecase.KindString, Required: true,
				Description: "Why. It is what whoever meets the hold months later has to go on, " +
					"and it is written into the audit trail.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: HoldPlacedAction, TargetType: holdTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h PlaceLegalHold) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := PlaceLegalHoldCommand{
		Scope:  domain.HoldScope(in.String("scope")),
		Reason: in.String("reason"),
	}
	if in.Present("scope_id") {
		scopeID, err := in.ID("scope_id")
		if err != nil {
			return nil, err
		}
		cmd.ScopeID = scopeID
	}

	hold, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return holdOutput(hold), nil
}

// Descriptor registers lifting one.
func (h ReleaseLegalHold) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReleaseLegalHoldName,
		Summary: "Lifts a hold, and records who lifted it, when, and why. The hold is not deleted " +
			"- it gains an end - because an auditor has to be able to tell \"there was never a " +
			"hold\" from \"somebody lifted it\". Lifting one twice is refused.",
		SideEffects: "Writes the lifting onto the hold and an audit entry carrying both reasons. " +
			"What was frozen becomes deletable again from the next retention pass.",
		TokenScope: retentionManage,
		Input: []usecase.Field{
			{Name: "hold_id", Kind: usecase.KindID, Required: true, Description: "Which hold."},
			{
				Name: "reason", Kind: usecase.KindString, Required: true,
				Description: "Why it is being lifted. Required: \"released\" with no reason is " +
					"an entry nobody can act on, and this is the moment the data becomes " +
					"deletable again.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: HoldReleasedAction, TargetType: holdTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReleaseLegalHold) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("hold_id")
	if err != nil {
		return nil, err
	}
	hold, err := h.Execute(ctx, actor, id, in.String("reason"))
	if err != nil {
		return nil, err
	}
	return holdOutput(hold), nil
}

// Descriptor registers the listing.
func (h ListLegalHolds) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListLegalHoldsName,
		Summary: "The legal holds this workspace has, newest first. The ones in force by " +
			"default, because that is what \"what is frozen\" means - and the lifted ones when " +
			"asked for, because a hold that has been lifted is what shows it was.",
		SideEffects: "None. Reads only.",
		TokenScope:  retentionRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "include_released", Kind: usecase.KindBool,
				Description: "Show the holds that have been lifted as well.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: HoldPlacedAction, TargetType: holdTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListLegalHolds) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	holds, err := h.Execute(ctx, actor, in.Present("include_released") && in.Bool("include_released"))
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(holds))
	for _, hold := range holds {
		rows = append(rows, holdOutput(hold))
	}
	return usecase.Output{"data": rows}, nil
}

// holdOutput is one hold as the three channels answer it.
func holdOutput(hold domain.LegalHold) usecase.Output {
	out := usecase.Output{
		"id":        hold.ID.String(),
		"reason":    hold.Reason,
		"placed_by": hold.PlacedBy.String(),
		"placed_at": hold.PlacedAt,
		"scope": map[string]any{
			"kind": string(hold.Scope),
			"id":   hold.ScopeID.String(),
		},
	}
	if hold.Released() {
		out["released_by"] = hold.ReleasedBy.String()
		out["released_at"] = hold.ReleasedAt
		out["released_reason"] = hold.ReleasedReason
	}
	return out
}
