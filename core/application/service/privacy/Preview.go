// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// PreviewErasureName is the use case that answers, before an erasure starts, what the legal holds
// in force would keep of it (UC-PRV-03 check 11).
const PreviewErasureName = "PreviewErasure"

// PreviewErasure answers what each hold would keep, through the same decision the erasure takes:
// two readings of one rule would show a person one thing and do another.
type PreviewErasure struct {
	Cases Cases
	// Eraser reads the holds and the person's rows the way the erasure does. Only its reading half
	// is used here.
	Eraser Eraser
}

// PreviewCommand is the input, typed.
type PreviewCommand struct {
	RequestID shared.ID
	// Mode is the mode the preview is for; the case's own when empty, else ANONYMIZE.
	Mode domain.ErasureMode
}

// Preview is the answer: the mode it was worked out for, and what each hold would keep.
type Preview struct {
	Mode domain.ErasureMode
	Kept []domain.Kept
	// Lines say, kind by kind, what the erasure would do; none for a case by address.
	Lines []domain.Line
}

// Execute works the preview out.
//
// The list's right: whoever may read the cases may read what a hold would keep of one - the holds
// themselves are no secret to them, and the hold's reason is not in the answer. An installation-
// wide case asks for the instance scope, as every step of one does.
func (h PreviewErasure) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd PreviewCommand,
) (Preview, error) {
	if cmd.RequestID.IsZero() {
		return Preview{}, shared.ErrValidation.WithDetail(domain.CodeRequestNotFound).
			WithFields(shared.FieldError{Path: "/request_id", Code: domain.CodeRequestNotFound})
	}
	if err := h.Cases.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RequestRecordedAction,
		TokenScope: privacyRead,
		TargetType: requestTarget,
		TargetID:   cmd.RequestID,
	}); err != nil {
		return Preview{}, err
	}

	var preview Preview
	err := h.Cases.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		request, err := h.Cases.Requests.Find(ctx, cmd.RequestID)
		if err != nil {
			return err
		}
		if request.Scope == domain.ScopeInstallation {
			if err := requireInstanceScope(actor); err != nil {
				return err
			}
		}
		switch {
		case request.Kind != domain.KindErasure:
			return shared.ErrConflict.WithDetail(domain.CodeNotAnErasure).
				WithParams(map[string]string{"request_id": request.ID.String()})
		case request.Status.Closed():
			return shared.ErrConflict.WithDetail(domain.CodeRequestClosed).
				WithParams(map[string]string{"request_id": request.ID.String()})
		}

		preview.Mode = cmd.Mode
		if preview.Mode == "" {
			preview.Mode = request.ErasureMode
		}
		if preview.Mode == "" {
			preview.Mode = domain.DefaultErasureMode
		}
		if request.SubjectAccountID.IsZero() {
			// A case by address: the question eraseAddress asks, so the dialog names the hold that
			// would keep what the address sent. Without an address nothing is erased or kept.
			kept, err := h.Eraser.addressKept(ctx, request)
			preview.Kept = kept
			return err
		}
		plan, err := h.Eraser.decide(ctx, request.SubjectAccountID, preview.Mode)
		if err != nil {
			return err
		}
		preview.Kept, preview.Lines = plan.Kept, plan.Lines
		return nil
	})
	if err != nil {
		return Preview{}, err
	}
	return preview, nil
}

// Descriptor registers the preview in all three channels.
func (h PreviewErasure) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PreviewErasureName,
		Permission: service.PermissionManageMembers,
		Summary: "Before an erasure is started, what the legal holds in force would keep of it: a " +
			"hold wins over an erasure as far as it reaches, so what it covers is kept and the case " +
			"closes as partly completed. One line per kind of data the erasure handles - what goes, what " +
			"stays without the person's name, what a hold keeps - counted where the rows are read. Per hold, how many of the person's comments, assignments, " +
			"entries and intake it would keep, and whether it keeps the account itself - which only " +
			"a hold on the person or on the whole workspace does. Answered for the mode named, else " +
			"the case's own, else ANONYMIZE.",
		SideEffects: "None. Reads only.",
		TokenScope:  privacyRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "request_id", Kind: usecase.KindID, Required: true, Description: "Which case."},
			{
				Name: "mode", Kind: usecase.KindString,
				Enum:        []string{string(domain.ModeAnonymize), string(domain.ModeFullDelete)},
				Description: "The mode to work it out for. The case's own when left out.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RequestRecordedAction, TargetType: requestTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h PreviewErasure) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("request_id")
	if err != nil {
		return nil, err
	}
	preview, err := h.Execute(ctx, actor, PreviewCommand{
		RequestID: id, Mode: domain.ErasureMode(in.String("mode")),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"mode": string(preview.Mode), "kept": keptOutput(preview.Kept), "lines": linesOutput(preview.Lines),
	}, nil
}

// linesOutput is what the erasure does, kind by kind; a count only where the preview reads it.
func linesOutput(lines []domain.Line) []map[string]any {
	out := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		row := map[string]any{"kind": string(line.Kind), "fate": string(line.Fate), "count": nil}
		if line.Count != nil {
			row["count"] = *line.Count
		}
		out = append(out, row)
	}
	return out
}
