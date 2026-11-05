// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ReadSignInDetailsName is the people list's read of each person's address and second factor.
const ReadSignInDetailsName = "ReadSignInDetails"

// ReadSignInDetails answers, for the people list's rows, each person's mail address and whether an
// authenticator app stands - what AccountSummary leaves out because every member may read it.
// Asked of MANAGE_MEMBERS at the workspace: the right that invites a person by that address and
// resets that factor, so the row offers the reset only where there is something to reset.
type ReadSignInDetails struct {
	Details    repository.SignInDetails
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// Execute reads the details of the people named; anyone else named is left out.
func (h ReadSignInDetails) Execute(
	ctx context.Context, actor appshared.ActorContext, accountIDs []shared.ID,
) ([]repository.SignInDetail, error) {
	if len(accountIDs) == 0 {
		return nil, shared.ErrValidation.WithDetail("accounts.account_required").
			WithFields(shared.FieldError{Path: "/ids", Code: "accounts.account_required"})
	}
	if len(accountIDs) > MaxResolvedAccounts {
		return nil, shared.ErrValidation.
			WithDetail("accounts.too_many").WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/ids", Code: "accounts.too_many"})
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     AccountReadAction,
		TokenScope: membersRead,
		TargetType: accountTarget,
	}); err != nil {
		return nil, err
	}
	var details []repository.SignInDetail
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		details, err = h.Details.SignInDetails(ctx, accountIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return details, nil
}

// Descriptor registers the use case in all three channels.
func (h ReadSignInDetails) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReadSignInDetailsName,
		Permission: service.PermissionManageMembers,
		Summary: "Reads, for up to 200 people, the mail address each is mailed by (none for a " +
			"managed account) and whether a confirmed authenticator app stands. Service accounts, " +
			"erased accounts and identifiers of nobody here are left out.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "ids", Kind: usecase.KindIDList, Required: true, Description: "Whose details, 1 to 200 identifiers."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadSignInDetails) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	ids, err := in.IDList("ids")
	if err != nil {
		return nil, err
	}
	details, err := h.Execute(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(details))
	for _, d := range details {
		row := usecase.Output{"account_id": d.AccountID.String(), "email": nil, "authenticator_app": d.AuthenticatorApp}
		if d.Email != "" {
			row["email"] = d.Email
		}
		rows = append(rows, row)
	}
	return usecase.Output{"data": rows}, nil
}
