// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// GetAccountName is the read that turns an identifier into a name.
const GetAccountName = "GetAccount"

// GetAccount answers "who is this", which is what every record that says *who* leaves open.
//
// `ActivityEntry.actor`, an assignee, a member row: each carries `{type, id}` and no label, and the
// contract gives the reason - "the account is one request away and this record is deleted with its
// entry, so there is nothing for a copy of somebody's name to outlive". The request it names did
// not exist, so a client could name exactly one person: the one reading. F2-15 shipped a history in
// which everybody else is "Somebody", which is honest and unusable in a shared workspace.
//
// What it answers is less than `GetOwnAccount` does, and the difference is the point. A caller sees
// the name, the kind and the status - not the email, the locale, the time zone or the first day of
// the week. `data-protection.md` §9 draws that line: the visibility of profile data to other tenant
// members is *minimal - display name, avatar*. This is that minimum, and `AccountSummary` in the
// contract is the shape rather than `Account` with fields blanked, because a schema that omits a
// field cannot leak it by accident later.
//
// Read-only throughout, for the reason `GetOwnAccount` is: a feed resolving a dozen actors must not
// pin a dozen read transactions to the primary (`multi-tenancy.md` §7).
type GetAccount struct {
	Accounts   repository.Accounts
	UnitOfWork persistence.UnitOfWork
}

// Execute returns the account behind an identifier, as another member of the tenant may see it.
//
// There is no permission check, and that is the decision rather than an omission - the same
// decision `GetOwnAccount` records for its own reason, reached differently. A name is what makes a
// shared workspace readable at all: a viewer who may see an entry's history but not learn who wrote
// it is being shown a document with the subject removed from every sentence. And there is nothing
// narrower to authorise against, because the identifiers a client holds arrived in records it was
// already allowed to read.
//
// The tenant boundary is not checked here either, and for the reason it is not checked in
// `GetOwnAccount`: the row is found through the transaction wrapper that sets `app.tenant_id`
// (ADR-0010), so an identifier from another tenant does not resolve rather than being refused -
// which the cross-tenant test asserts rather than assumes.
//
// The token scope *is* checked, because a credential bounds its holder rather than granting a
// second identity (ADR-0005).
//
// An erased account answers with the marker its erasure wrote where the name was. That is not a
// special case here: it is what the row holds, and there is nothing left in it to withhold.
func (h GetAccount) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) (domain.Account, error) {
	if err := actor.RequireScope(accountsRead); err != nil {
		return domain.Account{}, err
	}
	if accountID.IsZero() {
		return domain.Account{}, shared.ErrValidation.
			WithDetail("accounts.account_required").
			WithFields(shared.FieldError{Path: "/account_id", Code: "accounts.account_required"})
	}

	var account domain.Account
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Accounts.Find(ctx, accountID)
		if err != nil {
			return err
		}
		account = found
		return nil
	})
	if err != nil {
		return domain.Account{}, err
	}
	return account, nil
}

// Descriptor registers the use case in all three channels.
func (h GetAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetAccountName,
		Summary: "Reads the name behind an account identifier, as another member of the same " +
			"tenant may see it: the display name, the kind and the status, and deliberately not " +
			"the email or the preferences. This is what lets a history, an assignment or a " +
			"member list say who rather than showing an identifier.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "Whose name. The caller's own is answered here too, and " +
					"`GetOwnAccount` is the read that needs no identifier.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	account, err := h.Execute(ctx, actor, accountID)
	if err != nil {
		return nil, err
	}
	return accountSummaryOutput(account), nil
}

// accountSummaryOutput is `accountOutput` without the four fields that are the account holder's own
// business. Written out rather than derived by deleting keys: a projection that starts from
// everything and removes what must not travel leaks a field the day somebody adds one.
func accountSummaryOutput(account domain.Account) usecase.Output {
	return usecase.Output{
		"id":           account.ID.String(),
		"kind":         string(account.Kind),
		"display_name": account.DisplayName,
		"status":       string(account.Status),
	}
}
