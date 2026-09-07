// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	notificationrepo "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListNotificationPreferencesName = "ListNotificationPreferences"

	// NotificationPreferencesReadAction is the audit code of an attempted read of somebody
	// else's preferences. Declared even though a permitted read writes no entry: a refused one
	// is recorded against the action that was refused (audit.md §4).
	NotificationPreferencesReadAction audit.Action = "account.notification_preferences_read"
)

// EffectivePreference is one row of the settings form: what applies to a category and a channel,
// and whether anybody wrote it. "Not stored" and "off" are different facts, and a form that
// showed the default as a choice somebody made would be lying about who made it.
type EffectivePreference struct {
	notification.Preference
	IsDefault bool
}

// ListNotificationPreferences answers what an account wants to be told about (F3-02).
//
// One row per category and channel the installation knows, in the order the domain lists them,
// with the default filled in where nothing is stored. The categories are a closed set in a check
// constraint and a client must not compile them in: this read, and `notification_categories` in
// the manifest, are what it renders the form from.
//
// Who may read is the rule `UpdateAccountPreferences` set for a person's own settings: the
// caller's own with the token scope alone, and somebody else's with the member management
// permission at the workspace. Read-only throughout (multi-tenancy.md §7).
type ListNotificationPreferences struct {
	Accounts    repository.Accounts
	Preferences notificationrepo.Preferences
	Authorizer  Authorizer
	UnitOfWork  persistence.UnitOfWork
}

// Execute returns the effective preferences, defaults included and marked.
func (h ListNotificationPreferences) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) ([]EffectivePreference, error) {
	target, err := preferenceSubject(ctx, actor, accountID, h.Authorizer, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     NotificationPreferencesReadAction,
		TokenScope: accountsRead,
		TargetType: accountTarget,
	})
	if err != nil {
		return nil, err
	}

	var stored []notification.Preference
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := findAccount(ctx, h.Accounts, target); err != nil {
			return err
		}
		var err error
		stored, err = h.Preferences.ListForAccount(ctx, target)
		return err
	})
	if err != nil {
		return nil, err
	}
	return effectivePreferences(actor.TenantID, target, stored), nil
}

// preferenceSubject decides whose settings are meant and whether the actor may touch them: the
// caller's own need the token scope and nothing more - reading one's own settings is not
// administering anybody - and anybody else's ask the member management permission, with the
// refusal recorded. Written once for the read and the write, so that the two cannot disagree.
func preferenceSubject(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
	authorizer Authorizer, request access.Request,
) (shared.ID, error) {
	target := accountID
	if target.IsZero() {
		target = actor.AccountID
	}
	if target == actor.AccountID {
		return target, actor.RequireScope(request.TokenScope)
	}
	request.TargetID = target
	return target, authorizer.Authorize(ctx, actor, request)
}

// findAccount reads the subject so that a preference of somebody who is not here is "not found"
// rather than a foreign key violation from three layers down.
func findAccount(ctx context.Context, accounts repository.Accounts, id shared.ID) (domain.Account, error) {
	account, err := accounts.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Account{}, shared.ErrNotFound.
				WithDetail("accounts.not_found").
				WithParams(map[string]string{"account_id": id.String()})
		}
		return domain.Account{}, err
	}
	return account, nil
}

// effectivePreferences is the form: every pair the installation knows, the stored row where
// there is one and the default where there is not.
func effectivePreferences(
	tenantID, accountID shared.ID, stored []notification.Preference,
) []EffectivePreference {
	type pair struct {
		category notification.Category
		channel  notification.Channel
	}
	written := make(map[pair]notification.Preference, len(stored))
	for _, preference := range stored {
		written[pair{preference.Category, preference.Channel}] = preference
	}

	categories, channels := notification.Categories(), notification.Channels()
	effective := make([]EffectivePreference, 0, len(categories)*len(channels))
	for _, category := range categories {
		for _, channel := range channels {
			if preference, found := written[pair{category, channel}]; found {
				effective = append(effective, EffectivePreference{Preference: preference})
				continue
			}
			effective = append(effective, EffectivePreference{
				Preference: notification.DefaultPreference(tenantID, accountID, category, channel),
				IsDefault:  true,
			})
		}
	}
	return effective
}

// Descriptor registers the use case in all three channels.
func (h ListNotificationPreferences) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListNotificationPreferencesName,
		Summary: "Reads what an account wants to be told about: one row per notification category " +
			"and channel the installation knows, with the effective value. A pair nobody has " +
			"written is answered with the default and marked as such, because \"not stored\" and " +
			"\"off\" are different facts. The caller's own need no permission; anybody else's " +
			"need the member management permission.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Whose preferences. Omitted means the caller's own.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: NotificationPreferencesReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListNotificationPreferences) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	effective, err := h.Execute(ctx, actor, accountID)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(effective))
	for _, preference := range effective {
		rows = append(rows, preferenceOutput(preference))
	}
	return usecase.Output{"data": rows}, nil
}

// preferenceOutput is one row as every channel renders it. The timestamp is an explicit null for
// a default, because nobody wrote it and a client must be able to read the field unconditionally.
func preferenceOutput(preference EffectivePreference) usecase.Output {
	out := usecase.Output{
		"category":      string(preference.Category),
		"channel":       string(preference.Channel),
		"enabled":       preference.Enabled,
		"include_title": preference.IncludeTitle,
		"is_default":    preference.IsDefault,
		"updated_at":    nil,
	}
	if !preference.IsDefault {
		out["updated_at"] = preference.UpdatedAt
	}
	return out
}
