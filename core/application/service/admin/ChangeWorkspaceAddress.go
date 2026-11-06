// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"slices"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// A workspace's owner changes its address, where the installation allows it (multi-tenancy.md
// §3.1, UC-INS-19 check 3): the operator's change - the same checks, lockout rule, proof and
// notices - behind the installation's switch, the owner's role and the cooldown since the last change.

const (
	ChangeWorkspaceAddressName = "ChangeWorkspaceAddress"

	// workspaceManageScope is the token scope that changes how a workspace is set up.
	workspaceManageScope = "workspace:manage"
)

// WorkspaceAuthorizer is the role matrix's decision, as every workspace use case asks it.
type WorkspaceAuthorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// ChangeWorkspaceAddress is the use case.
type ChangeWorkspaceAddress struct {
	Change     ChangeTenantAddress
	Authorizer WorkspaceAuthorizer
}

// Execute changes the caller's own workspace's address, or with DryRun answers what it would be.
func (h ChangeWorkspaceAddress) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeTenantAddressCommand,
) (AddressChange, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     TenantAddressChangedAction,
		TokenScope: workspaceManageScope,
		TargetType: tenantTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return AddressChange{}, err
	}
	c := h.Change
	cmd.TenantID = actor.TenantID
	rule, err := OwnerAddressRuleOf(ctx, c.UnitOfWork, c.Settings, c.Tenancy)
	if err != nil {
		return AddressChange{}, err
	}
	if !rule.Allowed {
		return AddressChange{}, rule.RefuseOwnerChange(time.Time{}, time.Time{})
	}
	if c.Changes == nil {
		return AddressChange{}, shared.ErrInternal.WithDetail("admin.address_changes_missing")
	}

	var (
		owner bool
		last  time.Time
	)
	err = c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		paths, err := c.Owners.OwnerPaths(ctx)
		if err != nil {
			return err
		}
		owner = slices.ContainsFunc(paths, func(path identityrepo.OwnerPath) bool { return path.AccountID == actor.AccountID })
		last, err = c.Changes.LastAddressChange(ctx)
		return err
	})
	if err != nil {
		return AddressChange{}, err
	}
	if !owner {
		return AddressChange{}, shared.ErrForbidden.WithDetail("access.owner_required")
	}
	if err := rule.RefuseOwnerChange(last, c.Clock.Now()); err != nil {
		return AddressChange{}, err
	}
	return c.change(ctx, actor, cmd, &rule)
}

// OwnerAddressRuleOf is what the installation decided about an owner's change: off in single mode
// and wherever it decided nothing.
func OwnerAddressRuleOf(
	ctx context.Context, unitOfWork persistence.UnitOfWork, settings identityrepo.InstanceSettings,
	tenancy env.TenancyMode,
) (domain.OwnerAddressRule, error) {
	if tenancy != env.TenancyMulti {
		return domain.OwnerAddressRule{}, nil
	}
	level, err := installationLevel(ctx, unitOfWork, settings)
	if err != nil {
		return domain.OwnerAddressRule{}, err
	}
	return domain.OwnerAddressRule{
		Allowed: level.OwnersChangeAddress, CooldownDays: level.AddressCooldownDays,
	}, nil
}

// Descriptor is the catalogue entry.
func (h ChangeWorkspaceAddress) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangeWorkspaceAddressName,
		Summary: "The workspace's owner changes its address, where the installation lets owners do " +
			"it and its cooldown since the last change has passed: the same change, checks, " +
			"lockout rule and proof as the operator's. With dry_run it checks and answers what the " +
			"change would be.",
		SideEffects: "Replaces the workspace's main host and short name, ends every person's session, " +
			"mails every member with an address and tells the others at their next sign-in; records " +
			"it in the workspace's trail.",
		TokenScope: workspaceManageScope,
		StepUp:     "always, by password or provider - a passkey belongs to the address that ends",
		Input: []usecase.Field{
			{Name: "slug", Kind: usecase.KindString,
				Description: "The new short name, where workspaces answer beneath the installation's host."},
			{Name: "host", Kind: usecase.KindString,
				Description: "The new host, where the installation assigns hosts."},
			{Name: "dry_run", Kind: usecase.KindBool,
				Description: "Check and answer without changing anything, and without a step-up."},
			{Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof POST /auth/step-up answered, by password or provider."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantAddressChangedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a workspace's address is not an entry's history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ChangeWorkspaceAddress) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	change, err := h.Execute(ctx, actor, ChangeTenantAddressCommand{
		Slug: in.String("slug"), Host: in.String("host"),
		DryRun: in.Bool("dry_run"), StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	out := usecase.Output{
		"from": change.From, "members_mailed": change.Mailed,
		"members_told_at_sign_in": change.ToldAtSignIn, "dry_run": change.DryRun,
	}
	if change.To != "" {
		out["to"] = change.To
	}
	return out, nil
}
