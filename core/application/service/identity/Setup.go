// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"strings"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const SetUpInstallationName = "SetUpInstallation"

// InstallationSetUpAction is the first start's setup, written into the new workspace's own trail
// beside its provisioning: its members are entitled to see how the installation began.
const InstallationSetUpAction audit.Action = "instance.set_up"

// journalSetUp is the instance journal's entry for the same moment.
const journalSetUp = "instance.set_up"

// WorkspaceSeeder writes a new workspace as provisioning does, inside the caller's transaction
// (admin.ProvisionTenant.SeedWorkspace). It answers the default hub and the example collection.
type WorkspaceSeeder interface {
	SeedWorkspace(
		ctx context.Context, actor appshared.ActorContext, tenant domain.Tenant, owner domain.Account,
		now time.Time,
	) (shared.ID, shared.ID, error)
}

// FirstOwners activates the owner the setup just wrote, with its password and without a redemption
// token: the setup code was the proof.
type FirstOwners interface {
	ActivateFirstOwner(ctx context.Context, accountID shared.ID, passwordHash string, now time.Time) (bool, error)
}

// SetUpInstallationCommand is what the setup card sends.
type SetUpInstallationCommand struct {
	Code             string
	DisplayName      string
	Slug             string
	OwnerEmail       string
	OwnerDisplayName string
	Password         secret.Secret
	DefaultLocale    string
	DefaultTimeZone  string
	UserAgent        string
	RemoteAddr       string
}

// SetUpInstallation is the first start (UC-INS-01): with the code the server printed at start, one
// step creates the first workspace with its defaults, its owner with the password, and the owner's
// entry in the operator register - recorded in the new trail and in the instance journal - and
// signs the owner in.
//
// Authorisation is the code: the route is public because there is nobody yet who could hold a
// credential, and the code is printed only where the server's own output is readable. Once any
// workspace exists every code is spent and nothing here can run again - the way back into an
// installation after that is the machine's (`hubtask operator add`, UC-INS-03), never a public page.
type SetUpInstallation struct {
	Writer    SessionWriter
	Passwords *PasswordWriter
	Codes     repository.SetupCodes
	Seeder    WorkspaceSeeder
	Owners    FirstOwners
	Operators repository.Operators
	Journal   adminrepo.Journal
	Domains   text.DomainEncoder
	Text      text.Normalizer
	// Tenancy decides whether a short name is asked for: in single mode it routes nothing and is
	// derived from the name; in multi mode it is the workspace's subdomain.
	Tenancy env.TenancyMode
	// Instance is the installation's level: a file may have seeded its language and zone before
	// the first workspace exists, and that workspace starts with them. Nil reads nothing.
	Instance repository.InstanceSettings
}

// Execute sets the installation up and answers the owner's session.
func (h SetUpInstallation) Execute(
	ctx context.Context, cmd SetUpInstallationCommand,
) (SessionPair, error) {
	w := h.Writer

	slug := strings.TrimSpace(cmd.Slug)
	switch {
	case h.Tenancy == env.TenancyMulti && slug == "":
		return SessionPair{}, shared.ErrValidation.WithDetail("setup.short_name_required").
			WithFields(shared.FieldError{Path: "/slug", Code: "setup.short_name_required"})
	case h.Tenancy != env.TenancyMulti && slug != "":
		return SessionPair{}, shared.ErrValidation.WithDetail("setup.short_name_not_asked").
			WithFields(shared.FieldError{Path: "/slug", Code: "setup.short_name_not_asked"})
	case h.Tenancy != env.TenancyMulti:
		slug = domain.DeriveTenantSlug(cmd.DisplayName)
	}

	locale, zone, err := startingLocalisation(ctx, w.UnitOfWork, h.Instance, cmd.DefaultLocale, cmd.DefaultTimeZone)
	if err != nil {
		return SessionPair{}, err
	}

	now := w.Clock.Now()
	tenant, err := domain.NewTenant(domain.NewTenantInput{
		ID: w.IDs.NewID(), Slug: slug, DisplayName: cmd.DisplayName,
		DefaultLocale: locale, DefaultTimeZone: zone, Now: now,
		Text: h.Text,
	})
	if err != nil {
		return SessionPair{}, err
	}
	ownerName := strings.TrimSpace(cmd.OwnerDisplayName)
	if ownerName == "" {
		ownerName = cmd.OwnerEmail
	}
	owner, err := domain.Invite(w.IDs.NewID(), tenant.ID, cmd.OwnerEmail, ownerName, h.Domains, h.Text)
	if err != nil {
		return SessionPair{}, err
	}

	// The rule is the installation's: the workspace it would tighten does not exist yet. Judged
	// before the code is claimed, so a refused password spends nothing.
	rules, err := h.rules(ctx)
	if err != nil {
		return SessionPair{}, err
	}
	if err := h.judge(ctx, tenant, owner, cmd.Password, rules); err != nil {
		return SessionPair{}, err
	}
	// Outside the transaction: Argon2id is deliberately slow.
	passwordHash, err := w.Passwords.Hash(
		secret.New(domain.NormalisePassword(h.form(), cmd.Password.Reveal())))
	if err != nil {
		return SessionPair{}, err
	}

	// A code that is not even shaped like one is claimed as the empty code, so that it meets the
	// same answers a wrong one does - "already set up" first.
	code, _ := domain.ParseSetupCode(cmd.Code)

	// The new tenant's own scope, as provisioning's: the first write it sees is bounded to it.
	scope := persistence.Scope{TenantID: tenant.ID, ActorID: owner.ID}
	account := owner
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		claim, err := h.Codes.Claim(ctx, code, now)
		if err != nil {
			return err
		}
		if err := claimRefusal(claim); err != nil {
			return err
		}

		// The owner acts: the workspace is theirs from its first row, and its trail says so.
		actor := appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: tenant.ID, TenantSlug: tenant.Slug,
			AccountID: owner.ID, AccountName: owner.DisplayName,
		}
		hubID, _, err := h.Seeder.SeedWorkspace(ctx, actor, tenant, owner, now)
		if err != nil {
			return err
		}

		activated, err := h.Owners.ActivateFirstOwner(ctx, owner.ID, passwordHash, now)
		if err != nil {
			return err
		}
		if !activated {
			// The account was written a moment ago in this very transaction.
			return shared.ErrInternal.WithDetail("setup.owner_not_activated")
		}
		account.Status = domain.AccountActive

		// No adder: the entry was made by the installation's own first start, which the
		// contract's `Operator.added_by` describes as absent.
		enlisted, err := h.Operators.Enlist(ctx, owner.ID, shared.ID(""))
		if err != nil {
			return err
		}
		if enlisted != repository.EnlistAdded && enlisted != repository.EnlistPresent {
			// The owner was activated a moment ago in this very transaction.
			return shared.ErrInternal.WithDetail("setup.owner_not_activated")
		}

		if err := w.Audit.Append(ctx, audit.Entry{
			TenantID: tenant.ID, OccurredAt: now, Action: InstallationSetUpAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: appshared.ActorUser, ActorID: owner.ID, ActorLabel: owner.DisplayName,
			TargetType: "tenant", TargetID: tenant.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "operator_account_id", Classification: audit.Open, To: owner.ID.String()},
			),
		}); err != nil {
			return err
		}
		return h.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: now, Action: journalSetUp,
			TenantID: tenant.ID, TenantSlug: tenant.Slug, ActorLabel: owner.DisplayName,
			Details: map[string]any{
				"owner_account_id": owner.ID.String(),
				"default_hub_id":   hubID.String(),
				"operator_added":   true,
			},
		})
	})
	if err != nil {
		return SessionPair{}, err
	}

	return w.openSessionWith(ctx, persistence.Scope{TenantID: tenant.ID}, tenant.ID, account,
		cmd.UserAgent, cmd.RemoteAddr, SignedInAction, nil,
		rules.Effective.Policy.Sessions, domain.SignedInWithPassword)
}

// claimRefusal turns the claim's answer into the card's sentence.
func claimRefusal(claim repository.SetupClaim) error {
	switch claim {
	case repository.SetupClaimed:
		return nil
	case repository.SetupAlreadyDone:
		return shared.ErrConflict.WithDetail("setup.already_done")
	case repository.SetupExpired:
		return shared.ErrForbidden.WithDetail("setup.code_expired").
			WithFields(shared.FieldError{Path: "/code", Code: "setup.code_expired"})
	default:
		return domain.SetupCodeWrong()
	}
}

func (h SetUpInstallation) rules(ctx context.Context) (ResolvedPolicy, error) {
	if h.Passwords == nil {
		return ResolvedPolicy{
			Effective: domain.Effective(domain.PolicyLayer{}, domain.PolicyLayer{}, domain.PolicyLayer{}),
		}, nil
	}
	return h.Passwords.ResolveFor(ctx, shared.ID(""))
}

func (h SetUpInstallation) judge(
	ctx context.Context, tenant domain.Tenant, owner domain.Account, password secret.Secret,
	rules ResolvedPolicy,
) error {
	words := domain.PasswordContext{WorkspaceName: tenant.DisplayName, WorkspaceHost: tenant.Slug}
	if err := domain.CheckPasswordAgainst(
		rules.Effective.Policy.Password, h.form(), password.Reveal(), words,
	); err != nil {
		return err
	}
	if h.Passwords == nil {
		return nil
	}
	return h.Passwords.Judge(ctx, PasswordCandidate{
		TenantID: tenant.ID,
		Account:  repository.PasswordAccount{Account: owner},
		Password: password,
		Rules:    rules,
	})
}

func (h SetUpInstallation) form() text.Normalizer {
	if h.Passwords != nil {
		return h.Passwords.Text
	}
	return h.Text
}

// Descriptor is the catalogue entry.
func (h SetUpInstallation) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetUpInstallationName,
		Summary: "Sets up a fresh installation with the one-time code its first start printed: " +
			"the first workspace with its defaults, its owner with the password, and the " +
			"owner's entry in the operator register, in one step - and signs the owner in. " +
			"Refused once any workspace exists.",
		SideEffects: "Spends every setup code, writes the workspace and its seeded structure, " +
			"activates the owner, adds them to the operator register, records the setup in the " +
			"new workspace's trail and in the instance journal, and opens a session.",
		Input: []usecase.Field{
			{
				Name: "code", Kind: usecase.KindString, Required: true,
				Description: "The setup code the server printed at start; case and dashes do not matter.",
			},
			{Name: "display_name", Kind: usecase.KindString, Required: true},
			{
				Name: "slug", Kind: usecase.KindString,
				Description: "Multi mode only, and required there: the workspace's subdomain.",
			},
			{Name: "owner_email", Kind: usecase.KindString, Required: true},
			{Name: "owner_display_name", Kind: usecase.KindString},
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The owner's password, under the installation's password rule.",
			},
			{Name: "default_locale", Kind: usecase.KindString},
			{Name: "default_time_zone", Kind: usecase.KindString},
			{
				Name: "user_agent", Kind: usecase.KindString,
				Description: "The client as it introduced itself; recorded on the session.",
			},
			{
				Name: "remote_addr", Kind: usecase.KindString,
				Description: "The peer's address. Only its network class is ever recorded.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: InstallationSetUpAction, TargetType: "tenant",
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An installation is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetUpInstallation) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, SetUpInstallationCommand{
		Code:             in.String("code"),
		DisplayName:      in.String("display_name"),
		Slug:             in.String("slug"),
		OwnerEmail:       in.String("owner_email"),
		OwnerDisplayName: in.String("owner_display_name"),
		Password:         secret.New(in.String("password")),
		DefaultLocale:    in.String("default_locale"),
		DefaultTimeZone:  in.String("default_time_zone"),
		UserAgent:        in.String("user_agent"),
		RemoteAddr:       in.String("remote_addr"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}

// startingLocalisation answers the language and zone a new workspace starts with where the caller
// named none: the installation's defaults (UC-INS-09, "what every workspace starts with"), and
// otherwise nothing, which the tenant's own constructor reads as `en` and `UTC`. Read before the
// write's transaction and under the installation's scope - `instance_setting` carries no policy,
// and the write that follows runs in a scope of its own. Nil reads nothing.
func startingLocalisation(
	ctx context.Context, unitOfWork persistence.UnitOfWork, instance repository.InstanceSettings,
	locale, zone string,
) (string, string, error) {
	if instance == nil || (strings.TrimSpace(locale) != "" && strings.TrimSpace(zone) != "") {
		return locale, zone, nil
	}
	var defaults domain.LocalisationDefaults
	err := unitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		level, err := instance.Read(ctx)
		defaults = level.Localisation
		return err
	})
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(locale) == "" {
		locale = defaults.Locale
	}
	if strings.TrimSpace(zone) == "" {
		zone = defaults.TimeZone
	}
	return locale, zone, nil
}
