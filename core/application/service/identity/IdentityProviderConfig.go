// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
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
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ListIdentityProvidersName       = "ListIdentityProviders"
	ConfigureIdentityProviderName   = "ConfigureIdentityProvider"
	RemoveIdentityProviderName      = "RemoveIdentityProvider"
	ListIdentityProviderPresetsName = "ListIdentityProviderPresets"

	// identityProviderManage is the registry's scope (api-guidelines.md §7). Deciding which
	// provider vouches for this workspace's people is the same class of act as deciding who may
	// ask them for access, so it shares the OAuth registry's permission and has its own scope.
	identityProviderManage = "identity_provider:manage"

	identityProviderTarget = "identity_provider"
)

// MaxProvidersPerWorkspace bounds the collection. Plural is two or three - a company with staff at
// Google and a subsidiary at Microsoft - and a sign-in card with ten buttons on it has stopped being
// a sign-in card. The installation's own rows are not counted against it: they are not a workspace's
// to be limited by.
const MaxProvidersPerWorkspace = 10

// The audit codes of the relying party (H-04). Pointing a workspace at a provider, and taking
// the pointer away, are both events a review looks for by name.
const (
	IdentityProviderConfiguredAction audit.Action = "identity.provider_configured"
	IdentityProviderRemovedAction    audit.Action = "identity.provider_removed"
	IdentityProviderReadAction       audit.Action = "identity.provider_read"
)

// ClientSecretPurpose binds a sealed client secret to the level it belongs to, so a ciphertext
// lifted from one workspace's row into another's does not open (E-02, mfaSecretPurpose's reasoning).
//
// The level rather than the row, and a zero tenant is the installation's own - which is what lets
// the control plane's providers use the same function as a workspace's. Exported for the one caller
// outside this package, which is the control plane's half of the same store.
func ClientSecretPurpose(tenantID shared.ID) cryptoport.Purpose {
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
	// Workspaces is where a workspace's own switches for the installation's providers live. Nil
	// answers "nothing is taken", which is the honest reading on a build wired without it.
	Workspaces repository.Workspaces
	// RedirectURL is this installation's own callback, which every registration form at every
	// provider asks for. It is what the presets' instructions are rendered with.
	RedirectURL string
}

// ListIdentityProviders answers the ways in, never a secret.
type ListIdentityProviders struct{ Writer IdentityProviderWriter }

// Execute reads them.
func (h ListIdentityProviders) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.IdentityProvider, error) {
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
		return nil, err
	}

	var found []domain.IdentityProvider
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			read, err := w.Providers.List(ctx)
			if err != nil {
				return err
			}
			found, err = w.withOffers(ctx, read)
			return err
		})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// withOffers fills in whether each row is a way into *this* workspace.
//
// Read once for the whole listing rather than per row: it is one settings document, and a read per
// provider would be three reads of the same thing on a screen that shows three.
func (w IdentityProviderWriter) withOffers(
	ctx context.Context, rows []domain.IdentityProvider,
) ([]domain.IdentityProvider, error) {
	var settings domain.WorkspaceSettings
	if w.Workspaces != nil {
		workspace, err := w.Workspaces.Find(ctx)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		settings = workspace.Settings
	}
	answered := make([]domain.IdentityProvider, 0, len(rows))
	for _, row := range rows {
		row.OfferedHere = offeredHere(row, settings)
		answered = append(answered, row)
	}
	return answered, nil
}

// ConfigureIdentityProvider writes one, new or existing.
//
// **One use case for both**, which is decision 2's reasoning applied to a second thing that has a
// rule: the preset's refusals, the discovery check and the audit entry would otherwise be written
// twice and drift once. Which of the two it is, is whether an identifier came with it.
type ConfigureIdentityProvider struct{ Writer IdentityProviderWriter }

// ConfigureIdentityProviderCommand is the input, typed.
type ConfigureIdentityProviderCommand struct {
	// ID names the row to replace. Zero is a new one.
	ID                  shared.ID
	Issuer              string
	ClientID            string
	ClientSecret        secret.Secret
	DisplayName         string
	Kind                string
	Provisioning        string
	Position            int
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
	return w.ConfigureAt(ctx, actor.PersistenceScope(), actor, cmd, actor.TenantID)
}

// ConfigureAt is the whole of the write at one level, and the two levels share it: one set of the
// preset's refusals, one discovery call, one seal, one audit entry.
//
// Exported for the control plane's half of this same store
// (`admin.ConfigureInstanceIdentityProvider`), which is the one caller outside this package.
// Authorisation is *not* in here - each level checks its own, and this is what both do afterwards.
func (w IdentityProviderWriter) ConfigureAt(
	ctx context.Context, scope persistence.Scope, actor appshared.ActorContext,
	cmd ConfigureIdentityProviderCommand, tenantID shared.ID,
) (domain.IdentityProvider, error) {
	creating := cmd.ID.IsZero()
	if creating && cmd.ClientSecret.IsEmpty() {
		return domain.IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.client_secret_required")
	}

	id := cmd.ID
	if creating {
		id = w.Session.IDs.NewID()
	}

	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: id, TenantID: tenantID, Issuer: cmd.Issuer, ClientID: cmd.ClientID,
		DisplayName: cmd.DisplayName, Kind: cmd.Kind, Provisioning: cmd.Provisioning,
		Position: cmd.Position, AllowedEmailDomains: cmd.AllowedEmailDomains,
		Enabled: cmd.Enabled, Now: w.Session.Clock.Now(),
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}

	if err := w.Relying.Check(ctx, configured.Issuer); err != nil {
		return domain.IdentityProvider{}, err
	}

	// Sealed outside the transaction: the envelope is somebody else's key store in some
	// deployments, and a transaction held open across a network call is a connection nobody else
	// can have.
	var sealed *cryptoport.Sealed
	if !cmd.ClientSecret.IsEmpty() {
		envelope, err := w.Session.Encryptor.Seal(ctx, cmd.ClientSecret, ClientSecretPurpose(tenantID))
		if err != nil {
			return domain.IdentityProvider{}, err
		}
		sealed = &envelope
	}

	var stored domain.IdentityProvider
	err = w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if creating {
			counted, err := w.Providers.Count(ctx)
			if err != nil {
				return err
			}
			if counted >= MaxProvidersPerWorkspace {
				return shared.ErrValidation.
					WithDetail("identity_provider.too_many").
					WithParams(map[string]string{"limit": strconv.Itoa(MaxProvidersPerWorkspace)})
			}
			stored, err = w.Providers.Insert(ctx, configured, *sealed)
			if err != nil {
				return err
			}
			return w.record(ctx, actor, tenantID, IdentityProviderConfiguredAction, stored)
		}

		written, found, err := w.Providers.Update(ctx, configured, sealed, w.Session.Clock.Now())
		if err != nil {
			return err
		}
		if !found {
			// Gone, or the installation's own reached for by a workspace - which the write policy
			// refuses underneath rather than this check. One answer for both: a workspace learns
			// that the row is not its to change and nothing about what else is there.
			return shared.ErrNotFound.WithDetail("identity_provider.not_found")
		}
		stored = written
		return w.record(ctx, actor, tenantID, IdentityProviderConfiguredAction, stored)
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return stored, nil
}

// RemoveIdentityProvider takes one away.
type RemoveIdentityProvider struct{ Writer IdentityProviderWriter }

// Execute removes it.
//
// The accounts it provisioned keep their rows and their live sessions. What they lose is the way
// to sign in again - which is why the answer is audited and why an administrator doing this to a
// workspace of accounts with no password is doing something worth a record.
func (h RemoveIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
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
	return w.RemoveAt(ctx, actor.PersistenceScope(), actor, id, actor.TenantID)
}

// RemoveAt is shared with the control plane's half, for `ConfigureAt`'s reason.
func (w IdentityProviderWriter) RemoveAt(
	ctx context.Context, scope persistence.Scope, actor appshared.ActorContext,
	id shared.ID, tenantID shared.ID,
) error {
	if id.IsZero() {
		return shared.ErrValidation.WithDetail("identity_provider.not_found")
	}
	return w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		removed, err := w.Providers.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			// Nothing to remove is not a failure: the caller asked for it to be gone and it is.
			// A workspace reaching for the installation's row lands here too, because the write
			// policy matched nothing.
			return nil
		}
		return w.record(ctx, actor, tenantID, IdentityProviderRemovedAction,
			domain.IdentityProvider{ID: id, TenantID: tenantID})
	})
}

// record writes the trail entry. The issuer and the client id travel with it - they are
// configuration, not credentials - and the secret never does.
func (w IdentityProviderWriter) record(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
	action audit.Action, configured domain.IdentityProvider,
) error {
	changes := []audit.Change{}
	if configured.Issuer != "" {
		changes = append(changes,
			audit.Change{Field: "issuer", Classification: audit.Open, To: configured.Issuer},
			audit.Change{Field: "client_id", Classification: audit.Open, To: configured.ClientID},
			audit.Change{Field: "kind", Classification: audit.Open, To: string(configured.Kind)},
			audit.Change{Field: "provisioning", Classification: audit.Open,
				To: string(configured.Provisioning)},
			audit.Change{Field: "allowed_email_domains", Classification: audit.Open,
				To: strconv.Itoa(len(configured.AllowedEmailDomains))},
			audit.Change{Field: "enabled", Classification: audit.Open,
				To: strconv.FormatBool(configured.Enabled)})
	}
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   tenantID,
		OccurredAt: w.Session.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: identityProviderTarget,
		// The row is the target since SI-10: there are several, and which one changed is the
		// first thing a reader of the trail needs.
		TargetID: configured.ID,
		Context:  audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:  audit.Changes(changes...),
	})
}

// ProviderOutput is the read shape, and the secret is not in it - here as much as in the REST
// answer, because the registry serves MCP and automation from the same map.
//
// Exported for the control plane's half of this store: the installation's providers answer the same
// fields, and two projections of one row would drift on the day a field is added.
func ProviderOutput(configured domain.IdentityProvider) usecase.Output {
	out := usecase.Output{
		"id":                    configured.ID.String(),
		"issuer":                configured.Issuer,
		"client_id":             configured.ClientID,
		"display_name":          configured.DisplayName,
		"kind":                  string(configured.Kind),
		"provisioning":          string(configured.Provisioning),
		"position":              configured.Position,
		"scope":                 providerScope(configured),
		"allowed_email_domains": configured.AllowedEmailDomains,
		"enabled":               configured.Enabled,
		"offered_here":          configured.OfferedHere,
		"created_at":            configured.CreatedAt,
		"version":               configured.Version,
	}
	if !configured.UpdatedAt.IsZero() {
		out["updated_at"] = configured.UpdatedAt
	}
	return out
}

// providerScope is what a reader needs to tell its own choice from the one it inherited: a row of
// the level above is offered to it and is not its to change.
func providerScope(configured domain.IdentityProvider) string {
	if configured.Installation() {
		return ProviderScopeInstallation
	}
	return ProviderScopeWorkspace
}

// The two levels a provider can belong to, as the contract spells them.
const (
	ProviderScopeWorkspace    = "workspace"
	ProviderScopeInstallation = "installation"
)

// presetOutput is a preset as a screen needs it, with the instructions already carrying this
// installation's own callback - the one value every registration form at every provider asks for.
func presetOutput(preset domain.ProviderPreset, redirectURL string) usecase.Output {
	scopes := make([]any, 0, len(preset.Scopes))
	for _, scope := range preset.Scopes {
		scopes = append(scopes, scope)
	}
	out := usecase.Output{
		"kind":               string(preset.Kind),
		"scopes":             scopes,
		"addresses_verified": preset.AddressesVerified,
		"public":             preset.Public,
		"redirect_uri":       redirectURL,
		"instructions":       preset.Instructions,
		"provisioning":       provisioningOf(preset),
	}
	if preset.Particular != "" {
		out["particular"] = preset.Particular
	}
	return out
}

// provisioningOf is the modes a preset permits, in the order they tighten. What it is for is a
// screen that can offer only what would be accepted, rather than one that offers three and has two
// refused.
func provisioningOf(preset domain.ProviderPreset) []any {
	if preset.Public {
		return []any{string(domain.ProvisionInvitedOnly)}
	}
	modes := []any{}
	if preset.AddressesVerified {
		modes = append(modes, string(domain.ProvisionInvitedOnly))
	}
	return append(modes, string(domain.ProvisionDomains), string(domain.ProvisionAny))
}

// ListIdentityProviderPresets answers what can be configured and what it takes.
//
// Not behind a permission: it is three constants and this installation's own callback address,
// which the sign-in card shows anyway. What it is behind is having an account at all - there is no
// reason for a signed-out visitor to read registration instructions.
type ListIdentityProviderPresets struct{ Writer IdentityProviderWriter }

// Execute answers them.
func (h ListIdentityProviderPresets) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.ProviderPreset, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionManageMembers,
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      IdentityProviderReadAction,
		TokenScope:  identityProviderManage,
		TargetType:  identityProviderTarget,
	}); err != nil {
		return nil, err
	}
	return domain.ProviderPresets(), nil
}

func (h ListIdentityProviders) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListIdentityProvidersName,
		Summary: "The providers people can sign in to this workspace through: the workspace's " +
			"own and the ones its installation offers every workspace, each with its issuer, the " +
			"mark it draws, whom it lets in, and whether it is switched on. The client secret is " +
			"not among the fields, and there is no call that answers it.",
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

func (h ListIdentityProviders) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	found, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(found))
	for _, configured := range found {
		rows = append(rows, ProviderOutput(configured))
	}
	return usecase.Output{"data": rows}, nil
}

func (h ConfigureIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureIdentityProviderName,
		Summary: "Adds or replaces one of the providers this workspace signs its people in " +
			"through: the issuer, the client registration, whom the provider may bring in, and " +
			"the email domains inside which an arriving address may claim an account that " +
			"already exists. Discovery runs before anything is stored, so an issuer that answers " +
			"nothing is refused here. Sending no identifier adds one; sending one replaces it.",
		SideEffects: "Writes the configuration, seals the client secret, and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString,
				Description: "The provider to replace. Absent adds a new one."},
			{Name: "issuer", Kind: usecase.KindString, Required: true,
				Description: "The provider's issuer identifier. https, and checked against every token's `iss`."},
			{Name: "client_id", Kind: usecase.KindString, Required: true,
				Description: "This installation's registration with the provider."},
			{Name: "client_secret", Kind: usecase.KindString,
				Description: "Sealed on the way in and never answered again. Required when adding; absent when replacing keeps the one that is sealed."},
			{Name: "display_name", Kind: usecase.KindString,
				Description: "The name on the button. Absent is the issuer's host."},
			{Name: "kind", Kind: usecase.KindString,
				Description: "The preset: GENERIC, GOOGLE or MICROSOFT. Absent is read from the issuer."},
			{Name: "provisioning", Kind: usecase.KindString,
				Description: "INVITED_ONLY, DOMAINS or ANY - how freely an arriving subject may claim an account that already exists."},
			{Name: "position", Kind: usecase.KindInt,
				Description: "The order the buttons are drawn in."},
			{Name: "allowed_email_domains", Kind: usecase.KindList,
				Description: "Domains a verified address may link within, under DOMAINS. Empty links nothing."},
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
	cmd, err := ConfigureCommandOf(in)
	if err != nil {
		return nil, err
	}
	configured, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return ProviderOutput(configured), nil
}

// ConfigureCommandOf reads the input map, and the control plane's half reads it with the same
// function: two readings of ten fields would be two places for a default to differ.
func ConfigureCommandOf(in usecase.Input) (ConfigureIdentityProviderCommand, error) {
	domains, err := in.StringList("allowed_email_domains")
	if err != nil {
		return ConfigureIdentityProviderCommand{}, err
	}
	enabled := true
	if in.Present("enabled") {
		enabled = in.Bool("enabled")
	}
	cmd := ConfigureIdentityProviderCommand{
		Issuer:              in.String("issuer"),
		ClientID:            in.String("client_id"),
		ClientSecret:        secret.New(in.String("client_secret")),
		DisplayName:         in.String("display_name"),
		Kind:                in.String("kind"),
		Provisioning:        in.String("provisioning"),
		Position:            in.Int("position"),
		AllowedEmailDomains: domains,
		Enabled:             enabled,
	}
	id, err := in.ID("id")
	if err != nil {
		return ConfigureIdentityProviderCommand{}, err
	}
	cmd.ID = id
	return cmd, nil
}

func (h RemoveIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveIdentityProviderName,
		Summary: "Removes one of the workspace's identity providers and its sealed secret. The " +
			"accounts it provisioned keep their rows and their live sessions; what they lose is " +
			"the way to sign in again.",
		SideEffects: "Deletes the configuration and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString, Required: true,
				Description: "The provider to remove."},
		},
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
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

func (h ListIdentityProviderPresets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListIdentityProviderPresetsName,
		Summary: "The providers Hubtask has a preset for, and what registering with each of them " +
			"takes: the scopes it asks for, whether it may sign in people nobody invited, the one " +
			"thing about it that is not like the others, and this installation's own callback " +
			"address - which is the value every registration form asks for.",
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

func (h ListIdentityProviderPresets) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	presets, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(presets))
	for _, preset := range presets {
		rows = append(rows, presetOutput(preset, h.Writer.RedirectURL))
	}
	return usecase.Output{"data": rows}, nil
}
