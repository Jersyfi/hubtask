// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	CreateManagedAccountName = "CreateManagedAccount"

	// ManagedAccountCreatedAction is the audit code: somebody now has a way into this workspace,
	// one an administrator handed them (audit.md §4). The start password is nowhere in the entry.
	ManagedAccountCreatedAction audit.Action = "account.managed_created"
)

// CreateManagedAccountCommand is the input, typed.
type CreateManagedAccountCommand struct {
	DisplayName string
	SignInName  string
	Role        domain.Role
	Scope       domain.Scope
	// StepUpToken is the fresh proof an OWNER role demands, as the grant does.
	StepUpToken string
}

// ManagedAccountCreated is the account, its place, and the start password - shown here and never
// again (ADR-0074 §2).
type ManagedAccountCreated struct {
	Account       domain.Account
	Membership    domain.Grant
	StartPassword secret.Secret
}

// ManagedAccountsRule answers the sign-in rule in force, for the one switch this use case reads.
type ManagedAccountsRule interface {
	Resolve(ctx context.Context, tenantID shared.ID) (ResolvedPolicy, error)
}

// CreateManagedAccount gives somebody without a mail address an account (UC-ID-20 checks 1, 2).
//
// One use case creates and places, in one transaction: a start password shown for an account whose
// grant then failed would be an account nobody can find. The grant is GrantMembership's own - its
// permission at the scope, its step-up for OWNER, its trail entry - not a copy of it.
type CreateManagedAccount struct {
	Managed    repository.ManagedAccounts
	Authorizer Authorizer
	Grant      GrantMembership
	Rule       ManagedAccountsRule
	Passwords  cryptoport.PasswordHasher
	Entropy    clock.Entropy
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Text folds the sign-in name and composes the display name (i18n-l10n.md §5).
	Text text.Normalizer
}

// Execute creates the account, places it, and answers its start password once.
func (h CreateManagedAccount) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateManagedAccountCommand,
) (ManagedAccountCreated, error) {
	// An invitation's permission: the workspace's people are the workspace's owners' and
	// administrators' to add to (check 1).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     ManagedAccountCreatedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
	}); err != nil {
		return ManagedAccountCreated{}, err
	}
	// A rule's run reads no answer, so the start password would reach nobody: an account nobody
	// can ever sign in to is refused rather than created.
	if !actor.RunsForRule.IsZero() {
		return ManagedAccountCreated{}, shared.ErrForbidden.WithDetail("accounts.managed_needs_a_person")
	}

	account, err := domain.NewManagedAccount(h.IDs.NewID(), actor.TenantID, cmd.SignInName, cmd.DisplayName, h.Text)
	if err != nil {
		return ManagedAccountCreated{}, err
	}
	grant, err := h.Grant.prepare(ctx, actor, GrantMembershipCommand{
		AccountID: account.ID, Scope: cmd.Scope, Role: cmd.Role, StepUpToken: cmd.StepUpToken,
	})
	if err != nil {
		return ManagedAccountCreated{}, err
	}

	start, hash, err := drawStartPassword(h.Entropy, h.Passwords)
	if err != nil {
		return ManagedAccountCreated{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Read in the creation's transaction: the switch governs creating, and a creation that
		// read "on" just before an administrator switched it off is the one race accepted.
		rule, err := h.Rule.Resolve(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if !rule.Effective.Policy.ManagedAccounts {
			return shared.ErrForbidden.WithDetail("accounts.managed_accounts_off")
		}
		// Asked before inserting, so the common case - a name already in use - is a clear answer;
		// the index still decides between two creations at once.
		switch _, err := h.Managed.FindBySignInName(ctx, account.SignInName); {
		case err == nil:
			return signInNameTaken()
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}
		now := h.Clock.Now()
		if err := h.Managed.InsertManaged(ctx, account, hash, now); err != nil {
			return err
		}
		if err := h.recordAudit(ctx, account, actor); err != nil {
			return err
		}
		return h.Grant.record(ctx, actor, grant)
	})
	if err != nil {
		return ManagedAccountCreated{}, err
	}
	return ManagedAccountCreated{Account: account, Membership: grant, StartPassword: start}, nil
}

// drawStartPassword draws a start password and its hash, for a creation and a renewal alike. Outside
// the transaction: Argon2id is deliberately slow, and a connection held through it would let a burst
// of requests drain the pool.
func drawStartPassword(entropy clock.Entropy, passwords cryptoport.PasswordHasher) (secret.Secret, string, error) {
	drawn, err := entropy.Bytes(domain.StartPasswordBytes)
	if err != nil {
		return secret.Secret{}, "", shared.ErrInternal.WithDetail("accounts.start_password_unmintable").WithCause(err)
	}
	spelt, err := domain.StartPassword(drawn)
	if err != nil {
		return secret.Secret{}, "", err
	}
	start := secret.New(spelt)
	hash, err := passwords.Hash(start)
	if err != nil {
		return secret.Secret{}, "", err
	}
	return start, hash, nil
}

func signInNameTaken() error {
	return shared.ErrConflict.WithDetail("accounts.sign_in_name_taken").
		WithFields(shared.FieldError{Path: "/sign_in_name", Code: "accounts.sign_in_name_taken"})
}

func (h CreateManagedAccount) recordAudit(
	ctx context.Context, account domain.Account, actor appshared.ActorContext,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   account.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     ManagedAccountCreatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   account.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			// Sensitive, as an invitation's address is: the trail can compare two entries without
			// holding the name readable in a table that outlives the account.
			audit.Change{Field: "sign_in_name", Classification: audit.Sensitive, To: account.SignInName},
			audit.Change{Field: "status", Classification: audit.Open, To: string(account.Status)},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h CreateManagedAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateManagedAccountName,
		Summary: "Gives somebody without a mail address an account in this workspace: a sign-in " +
			"name instead of an address, and a role at a scope. Hubtask draws a start password " +
			"and answers it once; the person's first sign-in with it leads into choosing their own. " +
			"Refused where the workspace does not allow managed accounts, and for an act an " +
			"automation rule runs, since nobody would receive the start password.",
		SideEffects: "Writes the account and its membership, answers the start password once, and " +
			"writes two audit entries. Nothing keeps the start password but its hash.",
		TokenScope: membersWrite,
		Input: []usecase.Field{
			{
				Name: "display_name", Kind: usecase.KindString, Required: true,
				Description: "What to show beside this person's actions.",
			},
			{
				Name: "sign_in_name", Kind: usecase.KindString, Required: true,
				Description: "What the person signs in with: 3 to 32 characters after folding, no @, " +
					"no spaces. Unique in the workspace, compared folded.",
			},
			{
				Name: "role", Kind: usecase.KindString, Required: true,
				Enum: roleNames(), Description: "The role the person holds at the scope.",
			},
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Enum: []string{string(domain.ScopeTenant), string(domain.ScopeHub),
					string(domain.ScopeCollection), string(domain.ScopeItem)},
				Description: "TENANT for the whole workspace; otherwise the level the scope identifier names.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "The hub, collection or item. Omitted for the whole workspace.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof the OWNER role demands.",
			},
		},
		StepUp: "granting the OWNER role",
		Audit: usecase.AuditDeclaration{
			Action: ManagedAccountCreatedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateManagedAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	scopeID, err := in.ID("scope_id")
	if err != nil {
		return nil, err
	}
	created, err := h.Execute(ctx, actor, CreateManagedAccountCommand{
		DisplayName: in.String("display_name"),
		SignInName:  in.String("sign_in_name"),
		Role:        domain.Role(in.String("role")),
		Scope:       domain.Scope{Type: domain.ScopeType(in.String("scope_type")), ID: scopeID},
		StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"account":        accountOutput(created.Account),
		"membership":     grantOutput(created.Membership),
		"start_password": created.StartPassword.Reveal(),
	}, nil
}
