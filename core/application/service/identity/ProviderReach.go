// SPDX-License-Identifier: BUSL-1.1
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
)

// The password switch's count (ADR-0078 §1, UC-ID-12, P-04): before an administrator switches the
// password off, the screen says how many people here no provider switched on here signs in. Each of
// them connects one afterwards - through *Forgot your password?*, or with the password at the
// provider's first arrival - and none of them is locked out; the number is what the switch costs
// them, said where the decision is made.

const CountAccountsWithoutProviderName = "CountAccountsWithoutProvider"

// CountAccountsWithoutProvider answers the number, never the people.
type CountAccountsWithoutProvider struct {
	Writer IdentityProviderWriter
	// External counts the accounts no provider in a list signs in.
	External repository.ExternalAccounts
}

// Execute counts the active people without an identity at a provider that is a way in here now.
func (h CountAccountsWithoutProvider) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (int, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		// A-4, the providers' list's reading: how people here sign in is configuration, and an
		// auditor reads configuration without being able to change it.
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      IdentityProviderReadAction,
		TokenScope:  identityProviderManage,
		TargetType:  identityProviderTarget,
	}); err != nil {
		return 0, err
	}
	var counted int
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			listed, err := w.Providers.List(ctx)
			if err != nil {
				return err
			}
			offered, err := w.withOffers(ctx, listed)
			if err != nil {
				return err
			}
			// The one reading of "a way in here" (offeredHere), as the sign-in card draws it.
			on := make([]shared.ID, 0, len(offered))
			for _, row := range offered {
				if row.OfferedHere {
					on = append(on, row.ID)
				}
			}
			counted, err = h.External.CountWithoutIdentityAt(ctx, on)
			return err
		})
	if err != nil {
		return 0, err
	}
	return counted, nil
}

// Descriptor is the catalogue entry.
func (h CountAccountsWithoutProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CountAccountsWithoutProviderName,
		Summary: "How many active people of this workspace no provider switched on here signs in: " +
			"the number the password switch says before the password is switched off. Invited " +
			"accounts and service accounts are not counted. A number, never a list.",
		SideEffects: "None. Reads only.",
		TokenScope:  identityProviderManage,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: IdentityProviderReadAction, TargetType: identityProviderTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CountAccountsWithoutProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	counted, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{"count": counted}, nil
}
