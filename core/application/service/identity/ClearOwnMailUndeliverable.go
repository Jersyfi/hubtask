// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ClearOwnMailUndeliverableName = "ClearOwnMailUndeliverable"

	// MailUndeliverableClearedAction is the audit code of a clear: where mail to a person goes is
	// part of how they are reached, as their name is.
	MailUndeliverableClearedAction audit.Action = "account.mail_undeliverable_cleared"
)

// UndeliverableClearer removes the mark an address carries once the mail server refused it.
type UndeliverableClearer interface {
	Clear(ctx context.Context, accountID shared.ID) error
}

// ClearOwnMailUndeliverable lets a person have their mail tried again once the mailbox works:
// the mark a refusal for good set (notification.DeliverNotification) is removed. Only one's own,
// so there is nothing to authorise beyond the credential and its scope.
type ClearOwnMailUndeliverable struct {
	Marks      UndeliverableClearer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute clears the caller's mark.
func (h ClearOwnMailUndeliverable) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if err := actor.RequireScope(accountsWrite); err != nil {
		return err
	}
	if actor.AccountID.IsZero() {
		return shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Marks.Clear(ctx, actor.AccountID); err != nil {
			return err
		}
		// No address in the entry: it is personal data (rule 10).
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: h.Clock.Now(),
			Action:     MailUndeliverableClearedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: accountTarget,
			TargetID:   actor.AccountID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
}

// Descriptor is the catalogue entry.
func (h ClearOwnMailUndeliverable) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ClearOwnMailUndeliverableName,
		Summary: "Clears the mark the caller's address carries once the mail server refused it for " +
			"good, so that messages to it are sent again.",
		SideEffects: "Messages to the caller's address are tried again.",
		TokenScope:  accountsWrite,
		Audit: usecase.AuditDeclaration{
			Action: MailUndeliverableClearedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			return nil, h.Execute(ctx, actor)
		}),
	}
}
