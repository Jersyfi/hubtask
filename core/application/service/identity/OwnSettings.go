// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const ReadOwnSettingsName = "ReadOwnSettings"

// ReadOwnSettings answers the settings a person decides for themselves as the catalogue resolves
// them (F-5, model.md §4): each value in force with the level it comes from, so a preferences
// screen names "the workspace's" from the server's answer rather than from a chain of its own.
//
// Like GetOwnAccount it asks no permission: the values are the caller's own, and the workspace's
// and the installation's defaults are what every member is spoken to in anyway.
type ReadOwnSettings struct {
	Accounts   repository.Accounts
	UnitOfWork persistence.UnitOfWork
	// Resolver reads the workspace's and the installation's defaults, the levels above the person.
	Resolver SignInPolicyResolver
}

// Descriptor registers the use case in all three channels.
func (h ReadOwnSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadOwnSettingsName,
		Summary: "Reads the settings the caller decides for themselves - language, time zone, " +
			"first day of the week - as they are in force, each with the level its value comes " +
			"from: the caller's own (PERSON), the workspace's, the installation's or Hubtask's " +
			"default.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: AccountReadAction, TargetType: accountTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := actor.RequireScope(accountsRead); err != nil {
				return nil, err
			}
			if actor.AccountID.IsZero() {
				return nil, shared.ErrForbidden.WithDetail("accounts.self_required")
			}
			var account domain.Account
			err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				found, err := h.Accounts.Find(ctx, actor.AccountID)
				account = found
				return err
			})
			if err != nil {
				return nil, err
			}
			resolved, err := h.Resolver.Resolve(ctx, actor.TenantID)
			if err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			for _, resolution := range domain.OwnLocalisation(resolved.InstallationLocalisation, resolved.Workspace, account) {
				row := usecase.Output{"setting": string(resolution.Key), "source": string(resolution.Source)}
				if value, _ := resolution.Effective.(string); value != "" {
					row["value"] = value
				}
				rows = append(rows, row)
			}
			return usecase.Output{"data": rows}, nil
		}),
	}
}
