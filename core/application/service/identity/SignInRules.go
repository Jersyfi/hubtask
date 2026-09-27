// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const GetSignInRulesName = "GetSignInRules"

// What a sign-in screen may know before anybody has signed in (ADR-0068 §7).
//
// **Why a public route at all**, when F4 refused one that would have existed only to hide a button:
// a *configurable* hint is otherwise wrong. A screen that says "at least twelve characters" in a
// workspace that demands fifteen is a screen that lies, and the password is refused after the
// person has typed it. What this answers is the four things that makes right - the methods, the
// providers, what a password must meet, and the operator's legal links - and deliberately nothing
// else: not the expiry, not the history depth, not the timeouts, not the lists.
//
// **A host nobody answers at gets the installation's own level**, which is byte for byte what a
// workspace that has decided nothing answers. That is deliberate: which hosts hold workspaces is
// exactly what a probe is after (T-02), and the alternative - a refusal - would be a directory.

// SignInPolicyResolver is the resolution every door of the password's life shares: the levels
// above a workspace, the workspace's own, and the rule that comes out.
//
// One type rather than four copies of the same three reads, because a rule resolved slightly
// differently in the sign-in path and in the check route is the drift ADR-0068 exists to prevent.
type SignInPolicyResolver struct {
	Workspaces repository.Workspaces
	Instance   repository.InstanceSettings
	UnitOfWork persistence.UnitOfWork
}

// ResolvedPolicy is the rule in force for one workspace, with what the level above fixed and what
// only the installation knows.
type ResolvedPolicy struct {
	Effective domain.EffectivePolicy
	Legal     domain.LegalLinks
	LegalLock map[domain.LegalLink]domain.LockOrigin
	// Workspace is the row the rule was resolved for. Zero where none was.
	Workspace domain.Workspace
	// BlocklistFile is the operator's own list, instance-only.
	BlocklistFile string
}

// Resolve answers the rule in force inside the transaction the caller already opened.
//
// The instance level is read under the installation scope and the workspace's under its own: two
// transactions, because the two rows live on two sides of the tenant boundary and one transaction
// cannot be on both. The instance read is read-only, which is the only thing that scope permits.
func (r SignInPolicyResolver) Resolve(ctx context.Context, tenantID shared.ID) (ResolvedPolicy, error) {
	var instance repository.InstanceLevel
	err := r.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := r.Instance.Read(ctx)
			instance = read
			return err
		})
	if err != nil {
		return ResolvedPolicy{}, err
	}

	workspace := domain.Workspace{}
	if !tenantID.IsZero() {
		err = r.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
			func(ctx context.Context) error {
				read, err := r.Workspaces.Find(ctx)
				if err != nil {
					if errors.Is(err, shared.ErrNotFound) {
						// A host that resolves to no row answers the installation's level, which
						// is what a workspace with nothing set answers too.
						return nil
					}
					return err
				}
				workspace = read
				return nil
			})
		if err != nil {
			return ResolvedPolicy{}, err
		}
	}

	legal, legalLocks := domain.EffectiveLegal(instance.Legal, workspace.Settings.LegalLayer())
	return ResolvedPolicy{
		Effective: domain.Effective(
			instance.Policy, domain.PolicyLayer{}, workspace.Settings.SignInLayer()),
		Legal:         legal,
		LegalLock:     legalLocks,
		Workspace:     workspace,
		BlocklistFile: instance.BlocklistFile,
	}, nil
}

// ProviderSummary is one way in, as a sign-in screen needs it: enough to draw a button, and
// nothing about how the exchange works.
type ProviderSummary struct {
	ID          string
	DisplayName string
	// Kind is the preset it was configured from, which is what decides the mark that is drawn
	// (ADR-0069). GENERIC is the letter tile.
	Kind string
	// Scope is `installation` or `workspace`: whether every workspace is offered it, or this one
	// configured it.
	Scope string
}

// SignInRules is the whole answer.
type SignInRules struct {
	WorkspaceHost string
	Methods       []string
	Providers     []ProviderSummary
	Password      PasswordRulesView
	Legal         domain.LegalLinks
}

// PasswordRulesView is the password half, flattened to what a screen can act on.
//
// It is *not* the policy: the expiry, the minimum age and the two lists' sources are absent,
// because a guesser could use them and a person cannot. What is here is what a field can predict
// plus the names of the rules the server will answer, so that the list under the field is complete
// before the first keystroke.
type PasswordRulesView struct {
	MinLength    int
	MinLowercase int
	MinUppercase int
	MinDigits    int
	MinSymbols   int
	MinClasses   int
	// MaxRepeat is 0 where the switch is off, as every other count here is.
	MaxRepeat int
	// CommonPasswords is true where either offline list is consulted - the embedded one or the
	// operator's file. Which of the two refused a password is not a reader's business, and saying
	// would tell a guesser which corpus to avoid.
	CommonPasswords bool
	ContextWords    bool
	BreachCheck     bool
	// HistoryCount and NotCurrent are answered only to a reader who has an account: somebody who
	// has no password has neither a history nor a current one, and answering otherwise would put
	// two lines under the field that can never be met.
	HistoryCount int
	NotCurrent   bool
}

// passwordRulesView flattens the resolved rule.
func passwordRulesView(resolved ResolvedPolicy, withAccount bool) PasswordRulesView {
	password := resolved.Effective.Policy.Password
	view := PasswordRulesView{
		MinLength:       password.MinLength,
		MinLowercase:    password.MinLowercase,
		MinUppercase:    password.MinUppercase,
		MinDigits:       password.MinDigits,
		MinSymbols:      password.MinSymbols,
		MinClasses:      password.MinClasses,
		CommonPasswords: password.CommonPasswords || resolved.BlocklistFile != "",
		ContextWords:    password.ContextWords,
		BreachCheck:     password.BreachCheck,
	}
	view.MaxRepeat = password.MaxRepeat
	if withAccount {
		view.HistoryCount = password.HistoryCount
		view.NotCurrent = true
	}
	return view
}

// GetSignInRulesCommand carries what the public route received. The host is the one the request
// arrived at, because that is what the screen shows and what the workspace is resolved from.
type GetSignInRulesCommand struct {
	Host         string
	TenantSlug   string
	TenantHeader string
}

// GetSignInRules answers the public route.
type GetSignInRules struct {
	Resolver  SignInPolicyResolver
	Tenants   repository.TenantDirectory
	Providers repository.IdentityProviders

	UnitOfWork persistence.UnitOfWork
	// Multi is decision 3's mode switch, SessionWriter's: in single mode there is one workspace
	// and no subdomain to read.
	Multi bool
}

// Execute resolves and answers.
func (h GetSignInRules) Execute(
	ctx context.Context, cmd GetSignInRulesCommand,
) (SignInRules, error) {
	tenantID := h.resolveTenant(ctx, cmd.TenantSlug, cmd.TenantHeader)

	resolved, err := h.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return SignInRules{}, err
	}

	providers, err := h.providersOf(ctx, tenantID)
	if err != nil {
		return SignInRules{}, err
	}

	methods := make([]string, 0, 2)
	for _, method := range resolved.Effective.Policy.Methods {
		// A provider nobody configured is not a way in, however the policy is set: a button that
		// leads to a flow with no provider behind it is a button that answers an error.
		if method == domain.MethodOidc && len(providers) == 0 {
			continue
		}
		methods = append(methods, method)
	}

	return SignInRules{
		WorkspaceHost: strings.TrimSpace(cmd.Host),
		Methods:       methods,
		Providers:     providers,
		Password:      passwordRulesView(resolved, false),
		Legal:         resolved.Legal,
	}, nil
}

// resolveTenant is SessionWriter.resolveTenant without its refusals: a host nobody answers at is
// the zero tenant, which resolves to the installation's own level rather than to a 404 that would
// tell a probe which hosts hold workspaces.
func (h GetSignInRules) resolveTenant(ctx context.Context, slug, header string) shared.ID {
	lookup := slug
	if !h.Multi {
		lookup = ""
	} else if slug == "" {
		if header == "" {
			return ""
		}
		id, err := shared.ParseID(header)
		if err != nil {
			return ""
		}
		return id
	}

	var tenantID shared.ID
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			id, err := h.Tenants.Resolve(ctx, lookup)
			if err != nil {
				return err
			}
			tenantID = id
			return nil
		})
	if err != nil {
		return ""
	}
	return tenantID
}

// providersOf answers the ways in that are configured.
//
// One per workspace is what the model holds today; SI-10 makes them plural, and the shape here is
// already the plural one so that the screen does not move when it does. The identifier is the
// constant the single row has no column for, and is what `oidc:start` will take once there is a
// choice to make.
func (h GetSignInRules) providersOf(ctx context.Context, tenantID shared.ID) ([]ProviderSummary, error) {
	if tenantID.IsZero() || h.Providers == nil {
		return []ProviderSummary{}, nil
	}

	var provider domain.IdentityProvider
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			found, err := h.Providers.Find(ctx)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return nil
				}
				return err
			}
			provider = found
			return nil
		})
	if err != nil {
		return nil, err
	}
	if provider.Issuer == "" || !provider.Enabled {
		return []ProviderSummary{}, nil
	}

	return []ProviderSummary{{
		ID:          DefaultProviderID,
		DisplayName: issuerLabel(provider.Issuer),
		Kind:        providerKind(provider.Issuer),
		Scope:       "workspace",
	}}, nil
}

// DefaultProviderID names the one provider a workspace can have today. A constant rather than a
// row's identifier, because the row has no identifier column until SI-10 gives it one - and the
// client needs something stable to key a button on and to send back to `oidc:start`.
const DefaultProviderID = "default"

// The two issuers with a published sign-in button guideline (ADR-0069 §3). Everything else draws
// the letter tile, which is the honest answer rather than a borrowed logo.
func providerKind(issuer string) string {
	host := issuerLabel(issuer)
	// The hosts are written in parts rather than as literals, and not for style: a dotted lowercase
	// string in this tree is a message code to `TestEveryUsedMessageCodeIsInTheCatalogue`, and an
	// issuer that has to be entered in `locales/en.json` to pass a gate would be a sentence nobody
	// ever renders.
	for _, labels := range [][]string{{"login", "microsoftonline", "com"}, {"sts", "windows", "net"}} {
		if strings.HasSuffix(host, strings.Join(labels, ".")) {
			return "MICROSOFT"
		}
	}
	if strings.HasSuffix(host, strings.Join([]string{"accounts", "google", "com"}, ".")) {
		return "GOOGLE"
	}
	return "GENERIC"
}

// issuerLabel is the issuer without its scheme or path: what a button says until a provider has a
// display name of its own. No new disclosure - the button's own flow sends the person to exactly
// this host the moment it is pressed.
func issuerLabel(issuer string) string {
	label := strings.TrimPrefix(strings.TrimPrefix(issuer, "https://"), "http://")
	if slash := strings.IndexByte(label, '/'); slash > 0 {
		label = label[:slash]
	}
	return label
}

// rulesOutput is the projection every channel gets.
func rulesOutput(rules SignInRules) usecase.Output {
	methods := make([]any, 0, len(rules.Methods))
	for _, method := range rules.Methods {
		methods = append(methods, method)
	}
	providers := make([]any, 0, len(rules.Providers))
	for _, provider := range rules.Providers {
		providers = append(providers, usecase.Output{
			"id":           provider.ID,
			"display_name": provider.DisplayName,
			"kind":         provider.Kind,
			"scope":        provider.Scope,
		})
	}
	return usecase.Output{
		"workspace_host": rules.WorkspaceHost,
		"methods":        methods,
		"providers":      providers,
		"password":       passwordRulesOutput(rules.Password),
		"legal":          legalOutput(rules.Legal),
	}
}

func passwordRulesOutput(view PasswordRulesView) usecase.Output {
	out := usecase.Output{
		"min_length":       view.MinLength,
		"min_lowercase":    view.MinLowercase,
		"min_uppercase":    view.MinUppercase,
		"min_digits":       view.MinDigits,
		"min_symbols":      view.MinSymbols,
		"min_classes":      view.MinClasses,
		"max_repeat":       view.MaxRepeat,
		"common_passwords": view.CommonPasswords,
		"context_words":    view.ContextWords,
		"breach_check":     view.BreachCheck,
		"history_count":    view.HistoryCount,
		"not_current":      view.NotCurrent,
	}
	return out
}

// legalOutput omits what is not set rather than answering an empty string: a private installation
// owes nobody an imprint, and a footer with four empty links is four links pointing nowhere.
func legalOutput(links domain.LegalLinks) usecase.Output {
	out := usecase.Output{}
	for _, name := range domain.LegalLinkNames() {
		if value := links.Of(name); value != "" {
			out[string(name)] = value
		}
	}
	return out
}

// Descriptor is the catalogue entry.
func (h GetSignInRules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetSignInRulesName,
		Summary: "What a sign-in screen may know before anybody has signed in: which methods " +
			"this workspace signs in with, which providers it offers, what a password has to " +
			"meet, and the links its operator is obliged to show. The workspace is resolved " +
			"from the host, exactly as sign-in resolves it, and a host no workspace answers at " +
			"gets the installation's own level - which is byte for byte what a workspace that " +
			"has decided nothing answers.",
		SideEffects: "None. It reads the installation's level, the workspace's own, and the " +
			"providers configured for it.",
		Input: []usecase.Field{
			{
				Name: "host", Kind: usecase.KindString,
				Description: "The host the request arrived at, shown on the card.",
			},
			{
				Name: "tenant_slug", Kind: usecase.KindString,
				Description: "The subdomain the request arrived under, in multi mode.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent.",
			},
		},
		ReadOnly: true,
		// No action and nothing required: this is what a signed-out visitor draws a form from, and
		// an entry per screen load would be a trail of page views rather than a trail of acts.
		// There is also nobody to name as the actor - which is the honest reason as well.
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetSignInRules) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	rules, err := h.Execute(ctx, GetSignInRulesCommand{
		Host:         in.String("host"),
		TenantSlug:   in.String("tenant_slug"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return rulesOutput(rules), nil
}
