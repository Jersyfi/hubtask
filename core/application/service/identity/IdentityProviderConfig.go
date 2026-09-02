// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ConfigureIdentityProviderName = "ConfigureIdentityProvider"
	ReadIdentityProviderName      = "ReadIdentityProvider"
	RemoveIdentityProviderName    = "RemoveIdentityProvider"

	// identityProviderManage is the registry's scope (api-guidelines.md §7). Deciding which
	// provider vouches for this workspace's people is the same class of act as deciding who may
	// ask them for access, so it shares the OAuth registry's permission and has its own scope.
	identityProviderManage = "identity_provider:manage"

	identityProviderTarget = "identity_provider"
)

// The audit codes of the relying party (H-04). Pointing a workspace at a provider, and taking
// the pointer away, are both events a review looks for by name.
const (
	IdentityProviderConfiguredAction audit.Action = "identity.provider_configured"
	IdentityProviderRemovedAction    audit.Action = "identity.provider_removed"
	IdentityProviderReadAction       audit.Action = "identity.provider_read"
)

// clientSecretPurpose binds the sealed secret to the workspace it belongs to, so a ciphertext
// lifted into another workspace's row does not open (E-02, mfaSecretPurpose's reasoning).
func clientSecretPurpose(tenantID shared.ID) cryptoport.Purpose {
	return cryptoport.Purpose("identity_provider.client_secret:" + tenantID.String())
}

// IdentityProviderWriter is what the configuration use cases share.
type IdentityProviderWriter struct {
	Session   SessionWriter
	Providers repository.IdentityProviders
	// Relying is the port that knows what an issuer is. Configuration uses it for one thing:
	// asking the provider to prove it exists before a workspace is pointed at it.
	Relying    provider.Port
	Authorizer Authorizer
}

// ConfigureIdentityProvider points a workspace at its provider.
type ConfigureIdentityProvider struct{ Writer IdentityProviderWriter }

// ConfigureIdentityProviderCommand is the input, typed.
type ConfigureIdentityProviderCommand struct {
	Issuer              string
	ClientID            string
	ClientSecret        secret.Secret
	AllowedEmailDomains []string
	Enabled             bool
}

// Execute validates, asks the provider to prove it exists, seals the secret and stores the lot.
//
// The order matters. Discovery runs before anything is written, so a typed issuer that answers
// nothing is refused while somebody is still looking at the form - rather than three days later,
// by whoever tried to sign in first.
func (h ConfigureIdentityProvider) Execute(
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

	if cmd.ClientSecret.IsEmpty() {
		return domain.IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.client_secret_required")
	}

	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		TenantID: actor.TenantID, Issuer: cmd.Issuer, ClientID: cmd.ClientID,
		AllowedEmailDomains: cmd.AllowedEmailDomains, Enabled: cmd.Enabled,
		Now: w.Session.Clock.Now(),
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}

	if err := w.Relying.Check(ctx, configured.Issuer); err != nil {
		return domain.IdentityProvider{}, err
	}

	sealed, err := w.Session.Encryptor.Seal(ctx, cmd.ClientSecret, clientSecretPurpose(actor.TenantID))
	if err != nil {
		return domain.IdentityProvider{}, err
	}

	var stored domain.IdentityProvider
	err = w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err = w.Providers.Upsert(ctx, configured, sealed, w.Session.Clock.Now())
		if err != nil {
			return err
		}
		return w.record(ctx, actor, IdentityProviderConfiguredAction, stored)
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return stored, nil
}

// ReadIdentityProvider answers the configuration, never the secret.
type ReadIdentityProvider struct{ Writer IdentityProviderWriter }

// Execute reads it.
func (h ReadIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (domain.IdentityProvider, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		// A-4: which provider vouches for this workspace's people is configuration, and an
		// auditor reads configuration without being able to change it.
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      IdentityProviderReadAction,
		TokenScope:  identityProviderManage,
		TargetType:  identityProviderTarget,
	}); err != nil {
		return domain.IdentityProvider{}, err
	}

	var found domain.IdentityProvider
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			stored, err := w.Providers.Find(ctx)
			found = stored
			return err
		})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return found, nil
}

// RemoveIdentityProvider takes the configuration away.
type RemoveIdentityProvider struct{ Writer IdentityProviderWriter }

// Execute removes it.
//
// The accounts it provisioned keep their rows and their live sessions. What they lose is the way
// to sign in again - which is why the answer is audited and why an administrator doing this to a
// workspace of accounts with no password is doing something worth a record.
func (h RemoveIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext,
) error {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     IdentityProviderRemovedAction,
		TokenScope: identityProviderManage,
		TargetType: identityProviderTarget,
	}); err != nil {
		return err
	}

	return w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := w.Providers.Delete(ctx)
		if err != nil {
			return err
		}
		if !removed {
			// Nothing to remove is not a failure: the caller asked for it to be gone and it is.
			return nil
		}
		return w.record(ctx, actor, IdentityProviderRemovedAction,
			domain.IdentityProvider{TenantID: actor.TenantID})
	})
}

// record writes the trail entry. The issuer and the client id travel with it - they are
// configuration, not credentials - and the secret never does.
func (w IdentityProviderWriter) record(
	ctx context.Context, actor appshared.ActorContext,
	action audit.Action, configured domain.IdentityProvider,
) error {
	changes := []audit.Change{}
	if configured.Issuer != "" {
		changes = append(changes,
			audit.Change{Field: "issuer", Classification: audit.Open, To: configured.Issuer},
			audit.Change{Field: "client_id", Classification: audit.Open, To: configured.ClientID},
			audit.Change{Field: "allowed_email_domains", Classification: audit.Open,
				To: strconv.Itoa(len(configured.AllowedEmailDomains))},
			audit.Change{Field: "enabled", Classification: audit.Open,
				To: strconv.FormatBool(configured.Enabled)})
	}
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Session.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: identityProviderTarget,
		// The workspace is the target: there is one provider and its identity is the workspace's.
		TargetID: actor.TenantID,
		Context:  audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:  audit.Changes(changes...),
	})
}

// providerOutput is the read shape, and the secret is not in it - here as much as in the REST
// answer, because the registry serves MCP and automation from the same map.
func providerOutput(configured domain.IdentityProvider) usecase.Output {
	out := usecase.Output{
		"issuer":                configured.Issuer,
		"client_id":             configured.ClientID,
		"allowed_email_domains": configured.AllowedEmailDomains,
		"enabled":               configured.Enabled,
		"created_at":            configured.CreatedAt,
		"version":               configured.Version,
	}
	if !configured.UpdatedAt.IsZero() {
		out["updated_at"] = configured.UpdatedAt
	}
	return out
}

func (h ConfigureIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureIdentityProviderName,
		Summary: "Points this workspace at the identity provider its people sign in through: " +
			"the issuer, the client registration, and the email domains inside which an " +
			"arriving address may claim an account that already exists. Discovery runs before " +
			"anything is stored, so an issuer that answers nothing is refused here.",
		SideEffects: "Writes the configuration, seals the client secret, and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Input: []usecase.Field{
			{Name: "issuer", Kind: usecase.KindString, Required: true,
				Description: "The provider's issuer identifier. https, and checked against every token's `iss`."},
			{Name: "client_id", Kind: usecase.KindString, Required: true,
				Description: "This installation's registration with the provider."},
			{Name: "client_secret", Kind: usecase.KindString, Required: true,
				Description: "Sealed on the way in and never answered again."},
			{Name: "allowed_email_domains", Kind: usecase.KindList,
				Description: "Domains a verified address may link within. Empty links nothing."},
			{Name: "enabled", Kind: usecase.KindBool,
				Description: "Off keeps the configuration and refuses the flow."},
		},
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

func (h ConfigureIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	domains, err := in.StringList("allowed_email_domains")
	if err != nil {
		return nil, err
	}
	enabled := true
	if in.Present("enabled") {
		enabled = in.Bool("enabled")
	}
	configured, err := h.Execute(ctx, actor, ConfigureIdentityProviderCommand{
		Issuer:              in.String("issuer"),
		ClientID:            in.String("client_id"),
		ClientSecret:        secret.New(in.String("client_secret")),
		AllowedEmailDomains: domains,
		Enabled:             enabled,
	})
	if err != nil {
		return nil, err
	}
	return providerOutput(configured), nil
}

func (h ReadIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadIdentityProviderName,
		Summary: "How this workspace signs people in through its own provider: the issuer, the " +
			"client id, the linking domains and whether it is switched on. The client secret " +
			"is not among the fields, and there is no call that answers it.",
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
	configured, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return providerOutput(configured), nil
}

func (h RemoveIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveIdentityProviderName,
		Summary: "Removes the workspace's identity provider and its sealed secret. The accounts " +
			"it provisioned keep their rows and their live sessions; what they lose is the way " +
			"to sign in again.",
		SideEffects: "Deletes the configuration and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Audit: usecase.AuditDeclaration{
			Action: IdentityProviderRemovedAction, TargetType: identityProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's sign-in configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, actor); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
