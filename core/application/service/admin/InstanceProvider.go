// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domainidentity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The providers the installation offers every workspace (ADR-0070 §2, SI-10).
//
// **One store, two levels.** A row with no workspace is the installation's: every workspace reads it
// and draws its button, and none may change it - which is the row-level policy of migration 0103 and
// not a rule written here. What is written here is who may change those rows: the scope and the
// operator register, both, exactly as every other operation in this file's neighbourhood.
//
// The writing itself is `identity.IdentityProviderWriter.ConfigureAt`, shared with the workspace's
// own half. The preset's refusals, the discovery call and the sealing are one implementation
// deliberately: a second one would be a second set of rules about which provider may sign in whom.

const (
	ListInstanceIdentityProvidersName     = "ListInstanceIdentityProviders"
	ConfigureInstanceIdentityProviderName = "ConfigureInstanceIdentityProvider"
	RemoveInstanceIdentityProviderName    = "RemoveInstanceIdentityProvider"

	instanceProviderTarget = "identity_provider"
)

// The journal's actions. Offering every workspace a way in is not any one workspace's event, so the
// installation's own evidence is where it is read from (audit.md §6).
const (
	instanceProviderConfiguredAction audit.Action = "instance.provider_configured"
	instanceProviderRemovedAction    audit.Action = "instance.provider_removed"
	instanceProviderReadAction       audit.Action = "instance.provider_read"

	journalProviderConfigured = string(instanceProviderConfiguredAction)
	journalProviderRemoved    = string(instanceProviderRemovedAction)
)

// InstanceProviderWriter is the control plane's half of the provider store.
type InstanceProviderWriter struct {
	// Instance carries the pair every operation here demands - the scope, then the register - and
	// the journal the installation's own events are read from.
	Instance InstanceWriter
	// Providers is the store, read and written under the scope that has no tenant.
	Providers identityrepo.IdentityProviders
	// Configure is the writer the workspace's own half uses. Shared on purpose.
	Configure identityservice.IdentityProviderWriter
}

// ListInstanceIdentityProviders answers what the installation offers.
type ListInstanceIdentityProviders struct{ Writer InstanceProviderWriter }

// Execute reads them.
//
// Under the installation's own scope, which answers exactly the rows that belong to no workspace:
// the read policy admits a tenant's own rows and the tenant-less ones, and there is no tenant here.
func (h ListInstanceIdentityProviders) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domainidentity.IdentityProvider, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return nil, err
	}

	var found []domainidentity.IdentityProvider
	err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Providers.List(ctx)
			found = read
			return err
		})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// ConfigureInstanceIdentityProvider adds or replaces one of them.
type ConfigureInstanceIdentityProvider struct{ Writer InstanceProviderWriter }

// Execute writes it.
//
// The system scope rather than the installation's, for `WriteInstanceSettings`' reason: an
// installation scope is read-only by construction, and this is the scope with no tenant that may
// write. What bounds it is the same as there - every table with a tenant column stays invisible, and
// the provider row this reaches is the one whose tenant is NULL.
func (h ConfigureInstanceIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext,
	cmd identityservice.ConfigureIdentityProviderCommand,
) (domainidentity.IdentityProvider, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return domainidentity.IdentityProvider{}, err
	}

	// A zero tenant is what makes the row the installation's, in the domain and in the statement
	// both: the insert writes `current_tenant_id()`, which is NULL under this scope.
	stored, err := w.Configure.ConfigureAt(
		ctx, persistence.SystemScope(), actor, cmd, shared.ID(""))
	if err != nil {
		return domainidentity.IdentityProvider{}, err
	}

	if err := w.journal(ctx, actor, journalProviderConfigured, stored.ID, stored.Issuer); err != nil {
		return domainidentity.IdentityProvider{}, err
	}
	return stored, nil
}

// RemoveInstanceIdentityProvider takes one away.
type RemoveInstanceIdentityProvider struct{ Writer InstanceProviderWriter }

// Execute removes it.
//
// Every workspace loses that way in at once, which is why it is journalled and why the accounts it
// signed in keep their rows: what they lose is the way back, exactly as when a workspace removes its
// own.
func (h RemoveInstanceIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, stepUpToken string,
) error {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return err
	}
	if err := w.Configure.RemoveAt(
		ctx, persistence.SystemScope(), actor, id, shared.ID(""), stepUpToken); err != nil {
		return err
	}
	return w.journal(ctx, actor, journalProviderRemoved, id, "")
}

// journal writes the installation's own evidence. The issuer travels - it is configuration, not a
// credential - and the sealed secret never does.
func (w InstanceProviderWriter) journal(
	ctx context.Context, actor appshared.ActorContext,
	action string, providerID shared.ID, issuer string,
) error {
	details := map[string]any{"provider_id": providerID.String()}
	if issuer != "" {
		details["issuer"] = issuer
	}
	return w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.Instance.IDs.NewID(), OccurredAt: w.Instance.Clock.Now(), Action: action,
			ActorLabel: actor.AccountName, Details: details,
		})
	})
}

func (h ListInstanceIdentityProviders) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListInstanceIdentityProvidersName,
		Summary: "The providers this installation offers every workspace on it. Every workspace " +
			"reads them and draws their buttons; none may change them. The client secret is not " +
			"among the fields, and there is no call that answers it.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: instanceProviderReadAction, TargetType: instanceProviderTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListInstanceIdentityProviders) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	found, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(found))
	for _, configured := range found {
		rows = append(rows, identityservice.ProviderOutput(configured))
	}
	return usecase.Output{"data": rows}, nil
}

func (h ConfigureInstanceIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureInstanceIdentityProviderName,
		Summary: "Adds or replaces a provider every workspace on this installation may sign in " +
			"through. The same rules as a workspace's own: discovery before anything is stored, " +
			"the mark held to the issuer it belongs to, and a public provider held to signing in " +
			"only people who were invited first. Sending no identifier adds one.",
		SideEffects: "Writes the configuration, seals the client secret, and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString,
				Description: "The provider to replace. Absent adds a new one."},
			{Name: "issuer", Kind: usecase.KindString, Required: true,
				Description: "The provider's issuer identifier. https, and checked against every token's `iss`."},
			{Name: "client_id", Kind: usecase.KindString, Required: true,
				Description: "This installation's registration with the provider."},
			{Name: "client_secret", Kind: usecase.KindString,
				Description: "Sealed on the way in and never answered again. Required when adding."},
			{Name: "display_name", Kind: usecase.KindString,
				Description: "The name on the button. Absent is the issuer's host."},
			{Name: "kind", Kind: usecase.KindString,
				Description: "The preset: GENERIC, GOOGLE or MICROSOFT. Absent is read from the issuer."},
			{Name: "provisioning", Kind: usecase.KindString,
				Description: "INVITED_ONLY, DOMAINS or ANY - who this provider may admit, and what happens to whoever it admitted."},
			{Name: "position", Kind: usecase.KindInt,
				Description: "The order the buttons are drawn in."},
			{Name: "allowed_email_domains", Kind: usecase.KindList,
				Description: "Domains this provider admits under DOMAINS, for a preset with no directory claim."},
			{Name: "allowed_directories", Kind: usecase.KindList,
				Description: "The organisations this provider admits under DOMAINS, in its own identifiers - and required for a multi-directory issuer, which without one is every organisation in the world (ADR-0071)."},
			{Name: "enabled", Kind: usecase.KindBool,
				Description: "Off keeps the configuration and refuses the flow, for every workspace at once."},
			identityservice.ProviderStepUpField,
		},
		StepUp: "changing a way in every workspace is offered",
		Audit: usecase.AuditDeclaration{
			Action: instanceProviderConfiguredAction, TargetType: instanceProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfigureInstanceIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := identityservice.ConfigureCommandOf(in)
	if err != nil {
		return nil, err
	}
	stored, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return identityservice.ProviderOutput(stored), nil
}

func (h RemoveInstanceIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveInstanceIdentityProviderName,
		Summary: "Removes a provider the installation offered every workspace, and its sealed " +
			"secret. Every workspace loses that way in at once; the accounts it signed in keep " +
			"their rows and their live sessions.",
		SideEffects: "Deletes the configuration and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString, Required: true,
				Description: "The provider to remove."},
			identityservice.ProviderStepUpField,
		},
		StepUp: "changing a way in every workspace is offered",
		Audit: usecase.AuditDeclaration{
			Action: instanceProviderRemovedAction, TargetType: instanceProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveInstanceIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id, in.String("step_up_token")); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
