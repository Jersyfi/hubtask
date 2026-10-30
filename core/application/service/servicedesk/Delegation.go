// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// A person's own delegation of their approvals while away (UC-SVC-10): set, read and ended by
// that person alone, so each asks no permission of the catalogue.

// DelegationOutput is a delegation as every door answers it (schema ApprovalDelegation).
func DelegationOutput(d domain.Delegation) usecase.Output {
	return usecase.Output{"delegate_id": d.DelegateID.String(), "from": d.From.UTC(), "until": d.Until.UTC()}
}

func (a Approvals) delegationAudit(ctx context.Context, actor appshared.ActorContext, action audit.Action, changes ...audit.Change) error {
	return a.Desk.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: a.Desk.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo, ActorKind: actor.Kind, ActorID: actor.AccountID,
		ActorLabel: actor.AccountName, TargetType: delegationTarget, TargetID: actor.AccountID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)}, Changes: audit.Changes(changes...),
	})
}

// SetApprovalDelegation hands the reader's approvals to somebody from one moment to another.
type SetApprovalDelegation struct{ Approvals Approvals }

// Descriptor is the catalogue entry.
func (h SetApprovalDelegation) Descriptor() usecase.Descriptor {
	d := descriptor(usecase.Descriptor{
		Name: SetApprovalDelegationName,
		Summary: "Hands the reader's approvals to somebody while they are away, from one moment to another of at most a " +
			"year; never to themselves. A new delegation replaces the earlier one. The delegate decides for them, and " +
			"the trail names both.",
		SideEffects: "Writes the delegation and an audit entry.", TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{Name: "delegate_id", Kind: usecase.KindID, Required: true, Description: "Who decides meanwhile."},
			{Name: "from", Kind: usecase.KindString, Format: usecase.FormatDateTime, Required: true, Description: "From when, an RFC 3339 moment."},
			{Name: "until", Kind: usecase.KindString, Format: usecase.FormatDateTime, Required: true, Description: "Until when, an RFC 3339 moment."},
		},
		Audit: usecase.AuditDeclaration{Action: DelegationSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			delegate, err := in.ID("delegate_id")
			if err != nil {
				return nil, err
			}
			from, err := momentOf(in, "from")
			if err != nil {
				return nil, err
			}
			until, err := momentOf(in, "until")
			if err != nil {
				return nil, err
			}
			delegation, err := domain.NewDelegation(actor.TenantID, actor.AccountID, delegate, from, until)
			if err != nil {
				return nil, err
			}
			a := h.Approvals
			err = a.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := a.Store.Delegate(ctx, delegation); err != nil {
					return err
				}
				return a.delegationAudit(ctx, actor, DelegationSetAction,
					audit.Change{Field: "delegate_id", Classification: audit.Open, To: delegate.String()})
			})
			if err != nil {
				return nil, err
			}
			return DelegationOutput(delegation), nil
		}),
	}, delegationTarget)
	d.Permission = ""
	return d
}

// ReadApprovalDelegation answers the reader's delegation, or null.
type ReadApprovalDelegation struct{ Approvals Approvals }

// Descriptor is the catalogue entry.
func (h ReadApprovalDelegation) Descriptor() usecase.Descriptor {
	d := descriptor(usecase.Descriptor{
		Name: ReadApprovalDelegationName, Summary: "The reader's delegation of their approvals, or null.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: DelegationReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			var out usecase.Output
			err := h.Approvals.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				found, err := h.Approvals.Store.DelegationOf(ctx, actor.AccountID)
				if errors.Is(err, shared.ErrNotFound) {
					return nil
				}
				if err == nil {
					out = DelegationOutput(found)
				}
				return err
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"delegation": out}, nil
		}),
	}, delegationTarget)
	d.Permission = ""
	return d
}

// EndApprovalDelegation ends the reader's delegation: they decide again themselves.
type EndApprovalDelegation struct{ Approvals Approvals }

// Descriptor is the catalogue entry.
func (h EndApprovalDelegation) Descriptor() usecase.Descriptor {
	d := descriptor(usecase.Descriptor{
		Name: EndApprovalDelegationName, Summary: "Ends the reader's delegation; they decide their approvals again.",
		SideEffects: "Removes the delegation and writes an audit entry.", TokenScope: itemsWriteScope,
		Audit: usecase.AuditDeclaration{Action: DelegationEndedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			a := h.Approvals
			err := a.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				ended, err := a.Store.EndDelegation(ctx, actor.AccountID)
				if err != nil {
					return err
				}
				if !ended {
					return shared.ErrNotFound.WithDetail("approvals.no_delegation")
				}
				return a.delegationAudit(ctx, actor, DelegationEndedAction)
			})
			return nil, err
		}),
	}, delegationTarget)
	d.Permission = ""
	return d
}

func momentOf(in usecase.Input, field string) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, in.String(field))
	if err != nil {
		return time.Time{}, malformed("/"+field, "approvals.moment_malformed")
	}
	return at, nil
}
