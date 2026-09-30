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
	// Installation and InstallationLegal are the same resolution with the workspace's own layer
	// left out: what the level above set, which is what a settings screen shows beside each
	// control so that a reader can tell their own tightening from the default they inherited.
	Installation      domain.SignInPolicy
	InstallationLegal domain.LegalLinks
	// Workspace is the row the rule was resolved for. Zero where none was.
	Workspace domain.Workspace
	// BlocklistFile is the operator's own list, instance-only.
	BlocklistFile string
}

// Resolve answers the rule in force, in **one** scope.
//
// Both rows are read under the workspace's own scope, and that is deliberate rather than
// convenient: `instance_setting` carries no row-level policy (ADR-0070 §2), so it is as readable
// under a tenant's scope as under none - and a second scope would be a *second* transaction. Which
// matters for one reason: this resolver is called from inside transactions other people opened -
// the workspace patch's, the credential read's - and a nested unit of work that changes tenant is
// refused outright (`postgres.tenant_switch_in_transaction`), silently turning the whole resolution
// into an error that the callers above read as a policy decision. It cost an afternoon once.
//
// With no workspace to resolve for - a host nobody answers at - there is no tenant to be in, and
// the installation scope is the honest one.
func (r SignInPolicyResolver) Resolve(ctx context.Context, tenantID shared.ID) (ResolvedPolicy, error) {
	scope := persistence.InstallationScope()
	if !tenantID.IsZero() {
		scope = persistence.Scope{TenantID: tenantID}
	}

	var (
		instance  repository.InstanceLevel
		workspace domain.Workspace
	)
	err := r.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		read, err := r.Instance.Read(ctx)
		if err != nil {
			return err
		}
		instance = read
		if tenantID.IsZero() {
			return nil
		}
		found, err := r.Workspaces.Find(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// A host that resolves to no row answers the installation's level, which is what
				// a workspace with nothing set answers too.
				return nil
			}
			return err
		}
		workspace = found
		return nil
	})
	if err != nil {
		return ResolvedPolicy{}, err
	}

	legal, legalLocks := domain.EffectiveLegal(instance.Legal, workspace.Settings.LegalLayer())
	above, _ := domain.EffectiveLegal(instance.Legal, domain.LegalLayer{})
	return ResolvedPolicy{
		Effective: domain.Effective(
			instance.Policy, domain.PolicyLayer{}, workspace.Settings.SignInLayer()),
		Legal:     legal,
		LegalLock: legalLocks,
		// The same two levels without the workspace's, which is the default it may tighten.
		Installation:      domain.Effective(instance.Policy, domain.PolicyLayer{}, domain.PolicyLayer{}).Policy,
		InstallationLegal: above,
		Workspace:         workspace,
		BlocklistFile:     instance.BlocklistFile,
	}, nil
}

// Legal answers the links in force for a workspace, or the installation's own where the identifier
// is zero (SI-12).
//
// The manifest's own question, answered by the one resolver rather than by a second read: a footer
// inside the application needs the same four links the signed-out card shows, and two resolutions of
// one rule is the drift ADR-0068 exists to prevent.
func (r SignInPolicyResolver) Legal(ctx context.Context, tenantID shared.ID) (domain.LegalLinks, error) {
	resolved, err := r.Resolve(ctx, tenantID)
	if err != nil {
		return domain.LegalLinks{}, err
	}
	return resolved.Legal, nil
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
	// HasAccount is whether the caller is somebody with a password already. It decides two lines
	// and nothing else: the history depth and "not the one you have now" are answered to a reader
	// who has one and withheld from a reader who does not - a line under a field that can never be
	// met is a line that only worries people, and a guesser must not learn the depth at all.
	HasAccount bool
}

// GetSignInRules answers the public route.
type GetSignInRules struct {
	Resolver  SignInPolicyResolver
	Tenants   repository.TenantDirectory
	Providers repository.IdentityProviders
	// Workspaces answers which of the installation's providers this workspace took.
	Workspaces repository.Workspaces

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
		Password:      passwordRulesView(resolved, cmd.HasAccount),
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

// providersOf answers the ways in that are configured here.
//
// Plural since SI-10, and both levels: what a workspace configured and what its installation offers
// every workspace, which is what the read policy admits together (migration 0103). A provider that
// is switched off is not a way in and is not in the answer - a button that leads to a refusal is
// worse than no button.
func (h GetSignInRules) providersOf(ctx context.Context, tenantID shared.ID) ([]ProviderSummary, error) {
	if tenantID.IsZero() || h.Providers == nil {
		return []ProviderSummary{}, nil
	}

	var (
		inForce  []domain.IdentityProvider
		settings domain.WorkspaceSettings
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			read, err := h.Providers.List(ctx)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return nil
				}
				return err
			}
			inForce = read
			if h.Workspaces == nil {
				return nil
			}
			// Which of the installation's rows this workspace took. One read for the whole card.
			workspace, err := h.Workspaces.Find(ctx)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			settings = workspace.Settings
			return nil
		})
	if err != nil {
		return nil, err
	}

	summaries := make([]ProviderSummary, 0, len(inForce))
	for _, configured := range inForce {
		// A provider the installation offers is not a button until this workspace took it: "für
		// alle Arbeitsbereiche angeboten, nirgends an" (SI-10). A button that led to a way in
		// nobody here chose would be the installation deciding for the workspace.
		if configured.Issuer == "" || !offeredHere(configured, settings) {
			continue
		}
		scope := ProviderScopeWorkspace
		if configured.Installation() {
			scope = ProviderScopeInstallation
		}
		summaries = append(summaries, ProviderSummary{
			ID:          configured.ID.String(),
			DisplayName: configured.DisplayName,
			Kind:        string(configured.Kind),
			Scope:       scope,
		})
	}
	return summaries, nil
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
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	rules, err := h.Execute(ctx, GetSignInRulesCommand{
		Host:         in.String("host"),
		TenantSlug:   in.String("tenant_slug"),
		TenantHeader: in.String("tenant_header"),
		// The route is public, and a caller who *is* signed in is still the caller: the profile's
		// password field reads this very answer, and it needs the two lines a signed-out visitor
		// must not be given.
		HasAccount: actor.IsAuthenticated() && !actor.AccountID.IsZero(),
	})
	if err != nil {
		return nil, err
	}
	return rulesOutput(rules), nil
}
