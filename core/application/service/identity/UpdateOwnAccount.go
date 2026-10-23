// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	UpdateOwnAccountName = "UpdateOwnAccount"

	// AccountRenamedAction is the audit code of a person changing their own display name.
	AccountRenamedAction audit.Action = "account.renamed"
)

// UpdateOwnAccountCommand is the input. A nil DisplayName leaves the name as it is.
type UpdateOwnAccountCommand struct {
	DisplayName *string
}

// UpdateOwnAccount changes what the caller is called: the display name the other members see
// beside everything the person does, and the audit trail records next to their actions.
//
// Only one's own, which is the whole authorisation: there is no identifier to name anybody else,
// so being signed in as the person is enough - the token scope aside. A service account is refused,
// because it is named by whoever administers it, and a token acting as one renaming it would let a
// machine change what the trail calls it.
type UpdateOwnAccount struct {
	Accounts   repository.Accounts
	Names      repository.AccountNames
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Text       text.Normalizer
}

// Execute renames the caller's account and returns it.
func (h UpdateOwnAccount) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateOwnAccountCommand,
) (domain.Account, error) {
	if err := actor.RequireScope(accountsWrite); err != nil {
		return domain.Account{}, err
	}
	if actor.AccountID.IsZero() {
		return domain.Account{}, shared.ErrForbidden.WithDetail("accounts.self_required")
	}

	var updated domain.Account
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		account, err := h.Accounts.Find(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if account.Kind != domain.AccountUser {
			return shared.ErrForbidden.WithDetail("accounts.name_needs_a_person")
		}
		if cmd.DisplayName == nil {
			updated = account
			return nil
		}

		before := account
		account, err = account.WithDisplayName(*cmd.DisplayName, h.Text)
		if err != nil {
			return err
		}
		if account.DisplayName == before.DisplayName {
			// The same name again changes nothing, and an entry saying it did would be noise.
			updated = account
			return nil
		}

		now := h.Clock.Now()
		if err := h.Names.Rename(ctx, account, now); err != nil {
			return err
		}
		updated = account
		return h.recordAudit(ctx, before, account, actor, now)
	})
	if err != nil {
		return domain.Account{}, err
	}
	return updated, nil
}

func (h UpdateOwnAccount) recordAudit(
	ctx context.Context, before, after domain.Account, actor appshared.ActorContext, now time.Time,
) error {
	// A name is personal data: the trail says that it changed, never what to (rule 10).
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     AccountRenamedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "display_name", Classification: audit.Sensitive, From: before.DisplayName, To: after.DisplayName},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h UpdateOwnAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        UpdateOwnAccountName,
		Summary:     "Changes the caller's own display name - what the other members see beside everything they do.",
		SideEffects: "Writes the account's display name and an audit entry.",
		TokenScope:  accountsWrite,
		Input: []usecase.Field{
			{
				Name: "display_name", Kind: usecase.KindString,
				Description: "The new name, at most 200 characters on one line. Omitted leaves it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountRenamedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateOwnAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	account, err := h.Execute(ctx, actor, UpdateOwnAccountCommand{
		DisplayName: in.OptionalString("display_name"),
	})
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
}
