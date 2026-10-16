// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// Renaming and removing one's own passkeys (identity.md §23.2, UC-ID-22 checks 2, 4, 5).

const (
	RenameOwnPasskeyName = "RenameOwnPasskey"
)

// PasskeyRenamedAction is a name changed: the act and its moment, never the old or the new name
// (rule 10). Info: a name opens nothing.
const PasskeyRenamedAction audit.Action = "auth.passkey_renamed" //nolint:gosec // G101: an audit code

// RenameOwnPasskey changes the name a person gave one of their passkeys.
type RenameOwnPasskey struct{ Writer PasskeyWriter }

// Execute renames. The session alone is enough, or a token that may write its holder's account: a
// name opens nothing, so no fresh proof is asked (identity.md §23.2).
func (h RenameOwnPasskey) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, raw string,
) (OwnPasskey, error) {
	if err := passkeyPerson(actor); err != nil {
		return OwnPasskey{}, err
	}
	if err := actor.RequireScope(accountRead); err != nil {
		return OwnPasskey{}, err
	}
	name, err := domain.PasskeyName(raw)
	if err != nil {
		return OwnPasskey{}, err
	}
	s := h.Writer.Session
	var renamed OwnPasskey
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		credential, err := h.Writer.Passkeys.Rename(ctx, actor.AccountID, id, name)
		if err != nil {
			return err
		}
		renamed = ownPasskeyOf(credential)
		return s.recordMfaAudit(ctx, PasskeyRenamedAction, audit.SeverityInfo, domain.Account{
			ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
		}, s.Clock.Now())
	})
	if err != nil {
		return OwnPasskey{}, err
	}
	return renamed, nil
}

// Descriptor is the catalogue entry.
func (h RenameOwnPasskey) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RenameOwnPasskeyName,
		Summary: "Changes the name the caller gave one of their own passkeys or security keys, and " +
			"nothing else. No fresh proof: a name opens nothing.",
		SideEffects: "Changes the name; writes an audit entry without it.",
		TokenScope:  accountRead,
		Input: []usecase.Field{
			{Name: "passkey_id", Kind: usecase.KindID, Required: true,
				Description: "The passkey, as the list names it."},
			{Name: "name", Kind: usecase.KindString, Required: true,
				Description: "What the person calls it: 1 to 64 characters."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PasskeyRenamedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			id, err := in.ID("passkey_id")
			if err != nil {
				return nil, err
			}
			renamed, err := h.Execute(ctx, actor, id, in.String("name"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"passkey": ownPasskeyOutput(renamed)}, nil
		}),
	}
}
