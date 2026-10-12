// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListConsentsName = "ListConsents"

	// ConsentsListedAction is reading the consent register: who keeps their content out of AI is
	// personal data about a choice, read by administrators only.
	ConsentsListedAction audit.Action = "dsr.consents_listed"
)

// ListConsents is the consent register (UC-PRV-05 check 12): for one purpose, every person whose
// newest record is a withdrawal, with since when and who recorded it.
//
// **The one place the list appears.** No account, member list, export to colleagues or AI answer
// says who objected; this read does, to the people who manage members - the ones who answer an
// objection that arrives by mail.
type ListConsents struct {
	Consents   repository.Consents
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// Execute lists the register.
func (h ListConsents) Execute(
	ctx context.Context, actor appshared.ActorContext, purpose string,
) ([]repository.Objector, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ConsentsListedAction,
		TokenScope: privacyRead,
		TargetType: consentTarget,
	}); err != nil {
		return nil, err
	}
	cleaned := strings.TrimSpace(purpose)
	if cleaned == "" {
		return nil, shared.ErrValidation.WithDetail(domain.CodePurposeRequired).
			WithFields(shared.FieldError{Path: "/purpose", Code: domain.CodePurposeRequired})
	}
	var listed []repository.Objector
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		listed, err = h.Consents.Standing(ctx, cleaned)
		return err
	})
	return listed, err
}

// Descriptor registers the register in every channel.
func (h ListConsents) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListConsentsName,
		Summary: "The consent register: for one purpose - `ai_processing` for who keeps their " +
			"own content out of AI - every person whose newest record is a withdrawal, with " +
			"since when and whether they or an administrator recorded it. For the people who " +
			"manage members; the list appears nowhere else.",
		SideEffects: "None. Reads only.",
		TokenScope:  privacyRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "purpose", Kind: usecase.KindString, Required: true,
				Description: "Which purpose, e.g. `ai_processing`.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ConsentsListedAction, TargetType: consentTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListConsents) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	listed, err := h.Execute(ctx, actor, in.String("purpose"))
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(listed))
	for _, objector := range listed {
		row := usecase.Output{
			"account_id":   objector.AccountID.String(),
			"display_name": objector.DisplayName,
			"since":        objector.Since.UTC(),
		}
		if objector.Source != "" {
			row["source"] = objector.Source
		}
		rows = append(rows, row)
	}
	return usecase.Output{"items": rows}, nil
}
