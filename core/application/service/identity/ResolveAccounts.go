// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ResolveAccountsName is GetAccount for many identifiers at once.
const ResolveAccountsName = "ResolveAccounts"

// MaxResolvedAccounts bounds one request: a screen's worth of people, not a directory.
const MaxResolvedAccounts = 200

// ResolveAccounts answers "who are these" in one read, so that a member list or a feed naming a
// hundred people costs one request and not a hundred - which a client's rate limit would refuse in
// part, leaving people shown as "Somebody". GetAccount's rules, unchanged: no permission beyond
// the token's scope, the tenant bounded by the transaction wrapper, the same narrow projection.
// An identifier that does not resolve is left out rather than refused.
type ResolveAccounts struct {
	Accounts   repository.Accounts
	UnitOfWork persistence.UnitOfWork
}

// Execute answers the accounts behind the identifiers that resolve, each once.
func (h ResolveAccounts) Execute(
	ctx context.Context, actor appshared.ActorContext, accountIDs []shared.ID,
) ([]domain.Account, error) {
	if err := actor.RequireScope(accountsRead); err != nil {
		return nil, err
	}
	if len(accountIDs) == 0 {
		return nil, shared.ErrValidation.
			WithDetail("accounts.account_required").
			WithFields(shared.FieldError{Path: "/ids", Code: "accounts.account_required"})
	}
	if len(accountIDs) > MaxResolvedAccounts {
		return nil, shared.ErrValidation.
			WithDetail("accounts.too_many").WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/ids", Code: "accounts.too_many"})
	}

	var found []domain.Account
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		seen := make(map[shared.ID]bool, len(accountIDs))
		for _, id := range accountIDs {
			if id.IsZero() || seen[id] {
				continue
			}
			seen[id] = true
			account, err := h.Accounts.Find(ctx, id)
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			found = append(found, account)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// Descriptor registers the use case in all three channels.
func (h ResolveAccounts) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ResolveAccountsName,
		Summary: "Reads the names behind up to 200 account identifiers at once, as GetAccount " +
			"does for one: the display name, the kind and the status. An identifier that does " +
			"not resolve in this workspace is left out of the answer.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "ids", Kind: usecase.KindIDList, Required: true,
				Description: "Whose names, 1 to 200 identifiers.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ResolveAccounts) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	ids, err := in.IDList("ids")
	if err != nil {
		return nil, err
	}
	accounts, err := h.Execute(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(accounts))
	for _, account := range accounts {
		rows = append(rows, accountSummaryOutput(account))
	}
	return usecase.Output{"data": rows}, nil
}
