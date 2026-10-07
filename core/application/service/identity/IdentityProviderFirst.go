// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The singular surface, kept: removing a route is a breaking change (api-guidelines.md §8).
//
// `/identity-provider` is the provider surface from when a workspace had one provider. It
// **stays**: no route, no field and no sentence of the contract disappears. A caller written
// against it — a script, an older client, the documentation somebody has open —
// keeps working, and what it works on is the workspace's *first* provider.
//
// What it deliberately cannot do is the reason the collection exists: it cannot name a second
// provider, it never answers one the installation offers, and it does not remove. A `DELETE` on a
// route that cannot say *which* is a `DELETE` that eventually removes the wrong one.

const (
	ReadIdentityProviderName           = "ReadIdentityProvider"
	ConfigureFirstIdentityProviderName = "ConfigureFirstIdentityProvider"
)

// ReadIdentityProvider answers the workspace's first provider.
type ReadIdentityProvider struct{ Writer IdentityProviderWriter }

// Execute reads it.
func (h ReadIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (domain.IdentityProvider, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionManageMembers,
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      IdentityProviderReadAction,
		TokenScope:  identityProviderManage,
		TargetType:  identityProviderTarget,
	}); err != nil {
		return domain.IdentityProvider{}, err
	}

	var first domain.IdentityProvider
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			found, err := w.firstOwn(ctx)
			first = found
			return err
		})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return first, nil
}

// firstOwn answers the workspace's own first provider, in the order the collection lists them.
//
// The installation's rows are skipped rather than answered: this route predates them, and a caller
// that has never heard of a level above its workspace must not be handed a row it cannot write.
func (w IdentityProviderWriter) firstOwn(ctx context.Context) (domain.IdentityProvider, error) {
	inForce, err := w.Providers.List(ctx)
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	for _, candidate := range inForce {
		if !candidate.Installation() {
			candidate.OfferedHere = candidate.Enabled
			return candidate, nil
		}
	}
	return domain.IdentityProvider{}, shared.ErrNotFound.
		WithDetail("identity_provider.not_configured")
}

// ConfigureFirstIdentityProvider sets that one, or creates it where there is none.
type ConfigureFirstIdentityProvider struct{ Writer IdentityProviderWriter }

// Execute writes it.
//
// Which of the two it is, is not the caller's to say and never was: the route has no identifier, so
// the use case reads whether there is a first one and adds or replaces accordingly. That is the
// same `ConfigureAt` the collection uses — one set of rules, whichever door they are reached by.
func (h ConfigureFirstIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConfigureIdentityProviderCommand,
) (domain.IdentityProvider, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     IdentityProviderConfiguredAction,
		TokenScope: identityProviderManage,
		TargetType: identityProviderTarget,
	}); err != nil {
		return domain.IdentityProvider{}, err
	}

	// Read outside the write's transaction, as `ConfigureAt` reads nothing before discovery: what
	// this read decides is only which of two writes runs, and both are safe against a row that
	// moved in between — the insert meets the unique index, the update meets "no such row".
	var first shared.ID
	if err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			found, err := w.firstOwn(ctx)
			if err != nil {
				if shared.AsError(err).Category == shared.CategoryNotFound {
					return nil
				}
				return err
			}
			first = found.ID
			return nil
		}); err != nil {
		return domain.IdentityProvider{}, err
	}

	cmd.ID = first
	// The same form behind another door, and it switches nothing either (ADR-0076 §5).
	cmd.switchInList = true
	return w.ConfigureAt(ctx, actor.PersistenceScope(), actor, cmd, actor.TenantID)
}

func (h ReadIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadIdentityProviderName,
		Summary: "The workspace's first identity provider — the singular surface, kept from before " +
			"providers were plural. It reads the workspace's own first one and never a provider " +
			"the installation offers. The client secret is not among the fields, and there is no " +
			"call that answers it.",
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

func (h ReadIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	first, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return ProviderOutput(first), nil
}

func (h ConfigureFirstIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureFirstIdentityProviderName,
		Summary: "Sets the workspace's first identity provider whole, creating it where the " +
			"workspace has none — the singular surface, kept. The same rules the collection " +
			"applies: discovery before anything is stored, the mark held to its issuer, and a " +
			"public provider held to signing in only people who were invited first.",
		SideEffects: "Writes the configuration, seals the client secret, and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Input: []usecase.Field{
			{Name: "issuer", Kind: usecase.KindString, Required: true,
				Description: "The provider's issuer identifier. https, and checked against every token's `iss`."},
			{Name: "client_id", Kind: usecase.KindString, Required: true,
				Description: "This installation's registration with the provider."},
			{Name: "client_secret", Kind: usecase.KindString,
				Description: "Required where there is nothing to keep; absent afterwards keeps the one that is sealed."},
			{Name: "display_name", Kind: usecase.KindString,
				Description: "The name on the button. Absent is the issuer's host."},
			{Name: "kind", Kind: usecase.KindString,
				Description: "The preset: GENERIC, GOOGLE or MICROSOFT. Absent is read from the issuer."},
			{Name: "provisioning", Kind: usecase.KindString,
				Description: "INVITED_ONLY, DOMAINS or ANY — who this provider may bring in."},
			{Name: "position", Kind: usecase.KindInt,
				Description: "The order the buttons are drawn in."},
			{Name: "allowed_email_domains", Kind: usecase.KindList,
				Description: "The domains a verified address must be inside under DOMAINS."},
			EnabledFieldOfAWorkspace,
			ProviderStepUpField,
		},
		StepUp: providerStepUp,
		Audit: usecase.AuditDeclaration{
			Action: IdentityProviderConfiguredAction, TargetType: identityProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's sign-in configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfigureFirstIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := ConfigureCommandOf(in)
	if err != nil {
		return nil, err
	}
	// The route has no identifier, so a body that carried one would be naming a row the route
	// cannot address. Dropped rather than refused: it is not part of this contract's request.
	cmd.ID = shared.ID("")
	configured, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return ProviderOutput(configured), nil
}
