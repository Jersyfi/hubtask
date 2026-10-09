// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ExtendDataSubjectRequestName = "ExtendDataSubjectRequest"

	// RequestExtendedAction is the entry an extension writes, in every workspace it touches
	// (data-protection.md §4.1).
	RequestExtendedAction audit.Action = "dsr.extended"
)

// ExtendDataSubjectRequest extends a case's deadline once (Art. 12(3)).
//
// Its own use case rather than a field of the update, because the update is destructive - it can
// start an erasure - and moving a date is not: an agent may extend a deadline without the
// capability that lets it destroy work.
type ExtendDataSubjectRequest struct{ Cases Cases }

// ExtendCommand is the input, typed.
type ExtendCommand struct {
	RequestID  shared.ID
	DueOn      domain.Day
	Reason     domain.ExtensionReason
	InformedOn domain.Day
}

// Execute extends the case.
//
// `MANAGE_MEMBERS`, the line every other step of a case that destroys nothing sits on. The case is
// read, decided on, written and recorded in one transaction; the write is conditional on what was
// decided, so of two extensions racing each other, or an extension racing a completion, exactly one
// wins, and the loser is answered with what is true by then.
func (h ExtendDataSubjectRequest) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ExtendCommand,
) (domain.Request, error) {
	if cmd.RequestID.IsZero() {
		return domain.Request{}, shared.ErrValidation.WithDetail(domain.CodeRequestNotFound)
	}
	if err := h.Cases.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RequestExtendedAction,
		TokenScope: privacyManage,
		TargetType: requestTarget,
		TargetID:   cmd.RequestID,
	}); err != nil {
		return domain.Request{}, err
	}

	now := h.Cases.Clock.Now()
	var extended domain.Request
	err := h.Cases.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Cases.Requests.Find(ctx, cmd.RequestID)
		if err != nil {
			return err
		}
		if current.Scope == domain.ScopeInstallation {
			if err := requireInstanceScope(actor); err != nil {
				return err
			}
		}
		zone, err := h.Cases.zone(ctx)
		if err != nil {
			return err
		}
		in := domain.ExtendInput{
			DueOn: cmd.DueOn, Reason: cmd.Reason, InformedOn: cmd.InformedOn, Zone: zone, Now: now,
		}
		if extended, err = current.Extend(in); err != nil {
			return err
		}

		written, err := h.Cases.Requests.Extend(ctx, extended, now)
		if err != nil {
			return err
		}
		if !written {
			return h.lost(ctx, cmd.RequestID, in)
		}

		if err := h.fanOut(ctx, actor, extended); err != nil {
			return err
		}
		return h.Cases.record(ctx, actor, RequestExtendedAction, extended, audit.SeverityNotice,
			extensionChanges(extended))
	})
	if err != nil {
		return domain.Request{}, err
	}
	return extended, nil
}

// lost answers an extension whose conditional write found the case moved under it: read again in
// the same transaction, and refused for whatever is true now - closed, already extended, past its
// deadline, or gone.
func (h ExtendDataSubjectRequest) lost(ctx context.Context, id shared.ID, in domain.ExtendInput) error {
	now, err := h.Cases.Requests.Find(ctx, id)
	if err != nil {
		return err
	}
	if _, err := now.Extend(in); err != nil {
		return err
	}
	// The case reads as extendable and the write still found nothing: it moved twice under us.
	// Already extended is the one condition a second reader cannot undo.
	return shared.ErrConflict.WithDetail(domain.CodeAlreadyExtended)
}

// extensionChanges is what the entry records: the reason and both dates, never the notes.
func extensionChanges(extended domain.Request) []audit.Change {
	return []audit.Change{
		{Field: "kind", Classification: audit.Open, To: string(extended.Kind)},
		{
			Field: "due_at", Classification: audit.Open,
			From: extended.OriginalDueAt.UTC().Format(time.RFC3339),
			To:   extended.DueAt.UTC().Format(time.RFC3339),
		},
		{Field: "extension_reason", Classification: audit.Open, To: string(extended.ExtensionReason)},
		{Field: "informed_on", Classification: audit.Open, To: extended.InformedOn.String()},
	}
}

// Descriptor registers the extension in all three channels.
func (h ExtendDataSubjectRequest) Descriptor() usecase.Descriptor {
	reasons := make([]string, 0, len(domain.ExtensionReasons()))
	for _, reason := range domain.ExtensionReasons() {
		reasons = append(reasons, string(reason))
	}
	return usecase.Descriptor{
		Name: ExtendDataSubjectRequestName,
		Summary: "Extends a data subject request's deadline once, as Art. 12(3) GDPR allows: an " +
			"open case before its original deadline, to a day at most three months after the day " +
			"it was received, naming the reason and the day the person was informed. The case " +
			"answers extendable_until while this can succeed.",
		SideEffects: "Writes the extended deadline beside the original and an audit entry with the " +
			"reason and both dates. Nobody is written to: the controller informs the person. An " +
			"installation-wide case is also recorded in every workspace the person is a member of.",
		TokenScope: privacyManage,
		Input: []usecase.Field{
			{Name: "request_id", Kind: usecase.KindID, Required: true, Description: "Which case."},
			{
				Name: "due_on", Kind: usecase.KindString, Required: true,
				Description: "The new deadline as a day, YYYY-MM-DD; the case is due by the end " +
					"of it in the workspace's time zone.",
			},
			{
				Name: "reason", Kind: usecase.KindString, Required: true, Enum: reasons,
				Description: "Why: the request's complexity, or the number of requests.",
			},
			{
				Name: "informed_on", Kind: usecase.KindString, Required: true,
				Description: "The day the person was informed of the extension, YYYY-MM-DD.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RequestExtendedAction, TargetType: requestTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ExtendDataSubjectRequest) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("request_id")
	if err != nil {
		return nil, err
	}
	cmd := ExtendCommand{RequestID: id, Reason: domain.ExtensionReason(in.String("reason"))}
	if cmd.DueOn, err = parseDay(in.String("due_on"), "due_on"); err != nil {
		return nil, err
	}
	if cmd.InformedOn, err = parseDay(in.String("informed_on"), "informed_on"); err != nil {
		return nil, err
	}

	request, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	// An extended case cannot be extended again, so it answers no extendable_until.
	return RequestOutput(request, domain.Day{}, nil), nil
}
