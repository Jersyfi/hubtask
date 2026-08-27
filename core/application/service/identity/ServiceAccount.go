// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateServiceAccountName = "CreateServiceAccount"
	ListServiceAccountsName  = "ListServiceAccounts"

	// ServiceAccountCreatedAction is the audit code. Creating one is granting a way in that
	// outlives everybody, which is exactly the kind of act a trail exists for (audit.md §2).
	ServiceAccountCreatedAction audit.Action = "account.service_account_created"
	// ServiceAccountsReadAction is what the listing performs.
	ServiceAccountsReadAction audit.Action = "account.service_accounts_read"
)

// ServiceAccounts is what the two use cases share.
type ServiceAccounts struct {
	Accounts   repository.Accounts
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// CreateServiceAccount creates an account that exists only to be acted through (G-01).
type CreateServiceAccount struct{ Accounts ServiceAccounts }

// Execute creates it.
//
// The permission is the one that manages members, and for the same reason it governs a service
// account's tokens: an account is a way into the workspace, and one that is nothing but a way in
// is administered by whoever answers for who has access.
//
// It is what a rule's run_as points at (G-05), which is why this task comes before the rule
// engine: a rule engine whose rules run as people is a rule engine whose rules die with their
// author's departure.
func (h CreateServiceAccount) Execute(
	ctx context.Context, actor appshared.ActorContext, displayName string,
) (domain.Account, error) {
	s := h.Accounts
	if err := s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     ServiceAccountCreatedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
	}); err != nil {
		return domain.Account{}, err
	}

	var created domain.Account
	err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		account, err := domain.NewServiceAccount(s.IDs.NewID(), actor.TenantID, displayName)
		if err != nil {
			return err
		}
		if err := s.Accounts.Insert(ctx, account); err != nil {
			return err
		}
		created = account
		return s.recordCreation(ctx, actor, account, s.Clock.Now())
	})
	if err != nil {
		return domain.Account{}, err
	}
	return created, nil
}

// ListServiceAccounts answers the workspace's machines.
type ListServiceAccounts struct{ Accounts ServiceAccounts }

// Execute reads them, under the same permission that creates one: whoever answers for access sees
// what holds it.
func (h ListServiceAccounts) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.Account, error) {
	s := h.Accounts
	if err := s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     ServiceAccountsReadAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
	}); err != nil {
		return nil, err
	}

	var accounts []domain.Account
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := s.Accounts.ListOfKind(ctx, domain.AccountServiceAccount)
		accounts = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

// recordCreation writes the evidence. The display name is in it, unlike a token's: a service
// account's name is not one person's free text about their own credential but the label the trail
// will carry beside every action the machine takes, and an entry that omitted it would say a way
// in was created without saying which.
func (s ServiceAccounts) recordCreation(
	ctx context.Context, actor appshared.ActorContext, account domain.Account, at time.Time,
) error {
	return s.Audit.Append(ctx, audit.Entry{
		TenantID:   account.TenantID,
		OccurredAt: at,
		Action:     ServiceAccountCreatedAction,
		Outcome:    audit.OutcomeSuccess,
		// Notice rather than info, for the reason an invitation is: somebody - something - now
		// has a way into this workspace, and one that outlives every person in it.
		Severity:    audit.SeverityNotice,
		ActorKind:   actor.Kind,
		ActorID:     actor.AccountID,
		ActorLabel:  actor.AccountName,
		TargetType:  accountTarget,
		TargetID:    account.ID,
		TargetLabel: account.DisplayName,
		Context:     audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "kind", Classification: audit.Open, To: string(account.Kind)},
			audit.Change{Field: "status", Classification: audit.Open, To: string(account.Status)},
		),
	})
}

// Descriptor is the catalogue entry.
func (h CreateServiceAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateServiceAccountName,
		Summary: "Creates an account that exists only to be acted through: no mail address, no " +
			"sign-in anywhere, and personal access tokens as its only credential. It holds " +
			"rights the way a person does, through memberships granted to it, and it is what an " +
			"automation rule runs as - so that a rule does not stop working when its author " +
			"leaves. Needs the permission that manages members.",
		SideEffects: "Writes the account and an audit entry.",
		TokenScope:  membersWrite,
		Input: []usecase.Field{
			{
				Name: "display_name", Kind: usecase.KindString, Required: true,
				Description: "What the audit trail records beside every action it takes. Name " +
					"it after what it does - \"the nightly export\", not \"svc1\".",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ServiceAccountCreatedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateServiceAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	account, err := h.Execute(ctx, actor, in.String("display_name"))
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
}

// Descriptor is the catalogue entry.
func (h ListServiceAccounts) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListServiceAccountsName,
		Summary: "The workspace's service accounts: the accounts that exist only to be acted " +
			"through by an integration, a script or a rule. Under the same permission that " +
			"creates one - whoever answers for access sees what holds it.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersWrite,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: ServiceAccountsReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListServiceAccounts) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	accounts, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(accounts))
	for _, account := range accounts {
		rows = append(rows, accountOutput(account))
	}
	return usecase.Output{"data": rows}, nil
}
