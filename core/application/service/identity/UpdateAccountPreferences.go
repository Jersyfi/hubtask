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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	UpdateAccountPreferencesName = "UpdateAccountPreferences"
	accountRead                  = "accounts:write"

	// AccountPreferencesChangedAction is the audit code.
	AccountPreferencesChangedAction audit.Action = "account.preferences_changed"
)

// UpdateAccountPreferencesCommand is the input, typed.
//
// The three preferences are pointers, and that is the whole design of this command: a nil means
// "leave it", an empty string means "clear it, the workspace default applies again". Without the
// distinction a client that wanted to change only the locale would have to send the other two
// back, and a client that got that wrong would silently reset somebody's time zone.
type UpdateAccountPreferencesCommand struct {
	AccountID shared.ID
	Locale    *string
	TimeZone  *string
	WeekStart *string
}

// UpdateAccountPreferences sets how the product speaks to one account: its locale, its time zone
// and which day its weeks start on (i18n-l10n.md §2).
//
// Two kinds of caller reach it, and the authorisation is what tells them apart: a person changing
// their own preferences needs no permission beyond being themselves, and an administrator changing
// somebody else's needs the one that manages members. That check is here rather than in the
// adapter, like every other (ADR-0005).
type UpdateAccountPreferences struct {
	Accounts   repository.Accounts
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute applies the preferences and returns the account.
func (h UpdateAccountPreferences) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateAccountPreferencesCommand,
) (domain.Account, error) {
	target := cmd.AccountID
	if target.IsZero() {
		// No identifier means the caller means themselves, which is the common case and the one
		// worth not making a client spell out.
		target = actor.AccountID
	}

	// Changing somebody else's preferences is administering them. Changing one's own is not, and
	// requiring the permission for it would mean a viewer could not pick their own time zone.
	if target != actor.AccountID {
		if err := h.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionManageMembers,
			Path:       []domain.Scope{domain.TenantScope()},
			Action:     AccountPreferencesChangedAction,
			TokenScope: accountRead,
			TargetType: accountTarget,
			TargetID:   target,
		}); err != nil {
			return domain.Account{}, err
		}
	}

	var updated domain.Account
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		account, err := h.Accounts.Find(ctx, target)
		if err != nil {
			return err
		}

		before := account
		account, err = account.WithPreferences(domain.Preferences{
			Locale:    valueOr(cmd.Locale, before.Locale),
			TimeZone:  valueOr(cmd.TimeZone, before.TimeZone),
			WeekStart: valueOr(cmd.WeekStart, before.WeekStart),
		})
		if err != nil {
			return err
		}

		now := h.Clock.Now()
		if err := h.Accounts.UpdatePreferences(ctx, account, now); err != nil {
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

// valueOr resolves the pointer: absent leaves what was there, present replaces it - including with
// an empty string, which is how a preference is cleared.
func valueOr(given *string, current string) string {
	if given == nil {
		return current
	}
	return *given
}

func (h UpdateAccountPreferences) recordAudit(
	ctx context.Context, before, after domain.Account, actor appshared.ActorContext, now time.Time,
) error {
	// The preferences are not user content and not personal in the sense the trail cares about: a
	// locale is a choice from a closed set, and recording it openly is what lets an auditor see
	// that somebody's settings were changed by somebody else.
	changes := audit.Changes(
		audit.Change{Field: "locale", Classification: audit.Open, From: before.Locale, To: after.Locale},
		audit.Change{Field: "time_zone", Classification: audit.Open, From: before.TimeZone, To: after.TimeZone},
		audit.Change{Field: "week_start", Classification: audit.Open, From: before.WeekStart, To: after.WeekStart},
	)

	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     AccountPreferencesChangedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    changes,
	})
}

// Descriptor registers the use case in all three channels.
func (h UpdateAccountPreferences) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateAccountPreferencesName,
		Summary: "Sets the locale, the time zone and the first day of the week for an account. " +
			"Omitting a field leaves it; sending it empty clears it, so the workspace default " +
			"applies again.",
		SideEffects: "Writes the account's preferences and an audit entry.",
		TokenScope:  accountRead,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Whose preferences. Omitted means the caller's own; anybody else's needs the member management permission.",
			},
			{
				Name: "locale", Kind: usecase.KindString,
				Description: "A BCP 47 tag such as de, de-AT or pt-BR. Empty clears it.",
			},
			{
				Name: "time_zone", Kind: usecase.KindString,
				Description: "An IANA name such as Europe/Berlin, never a fixed offset - an offset cannot represent daylight saving. Empty clears it.",
			},
			{
				Name: "week_start", Kind: usecase.KindString,
				Enum:        []string{"MONDAY", "SUNDAY", "SATURDAY"},
				Description: "Which day a calendar week starts on. Empty clears it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountPreferencesChangedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateAccountPreferences) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}

	account, err := h.Execute(ctx, actor, UpdateAccountPreferencesCommand{
		AccountID: accountID,
		Locale:    in.OptionalString("locale"),
		TimeZone:  in.OptionalString("time_zone"),
		WeekStart: in.OptionalString("week_start"),
	})
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
}
