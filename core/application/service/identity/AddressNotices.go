// SPDX-License-Identifier: Apache-2.0
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

// A member without an email address is told of a change of the workspace's address at the next
// sign-in (multi-tenancy.md §3.1, UC-INS-19 check 8): the notice the change wrote, read and
// acknowledged by the member alone.

const (
	ListAddressNoticesName        = "ListAddressNotices"
	AcknowledgeAddressNoticesName = "AcknowledgeAddressNotices"

	AddressNoticesReadAction         audit.Action = "account.address_notices_read"
	AddressNoticesAcknowledgedAction audit.Action = "account.address_notices_acknowledged"
)

// ListAddressNotices answers the caller's notice, none once acknowledged.
type ListAddressNotices struct {
	Notices    repository.AddressChangeNotices
	UnitOfWork persistence.UnitOfWork
}

// Execute reads it. A service account is told nothing: only people sign in again.
func (h ListAddressNotices) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.AddressChangeNotice, error) {
	if err := ownNoticeActor(actor, accountsRead); err != nil {
		return nil, err
	}
	notices := []domain.AddressChangeNotice{}
	if actor.Kind != appshared.ActorUser {
		return notices, nil
	}
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Notices.Of(ctx, actor.AccountID)
		if read != nil {
			notices = read
		}
		return err
	})
	return notices, err
}

// AcknowledgeAddressNotices records that the caller read it. Twice is once.
type AcknowledgeAddressNotices struct {
	Notices    repository.AddressChangeNotices
	UnitOfWork persistence.UnitOfWork
}

// Execute removes it.
func (h AcknowledgeAddressNotices) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if err := ownNoticeActor(actor, accountsWrite); err != nil {
		return err
	}
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Notices.Acknowledge(ctx, actor.AccountID)
	})
}

func ownNoticeActor(actor appshared.ActorContext, scope string) error {
	if !actor.IsAuthenticated() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if err := actor.RequireScope(scope); err != nil {
		return err
	}
	if actor.AccountID.IsZero() {
		return shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	return nil
}

// Descriptor is the catalogue entry.
func (h ListAddressNotices) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListAddressNoticesName,
		Summary: "Lists the change of the workspace's address the caller, who has no email address, " +
			"has still to be told of: the old and the new address and when it changed. At most one.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: AddressNoticesReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			notices, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(notices))
			for _, notice := range notices {
				data = append(data, usecase.Output{
					"from": notice.From, "to": notice.To,
					"changed_at": notice.ChangedAt.UTC(),
				})
			}
			return usecase.Output{"data": data}, nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h AcknowledgeAddressNotices) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        AcknowledgeAddressNoticesName,
		Summary:     "Records that the caller read the change of the workspace's address.",
		SideEffects: "Removes the caller's notice of the change.",
		TokenScope:  accountsWrite,
		Audit: usecase.AuditDeclaration{
			Action: AddressNoticesAcknowledgedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read receipt of one's own notice."},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			return usecase.Output{}, h.Execute(ctx, actor)
		}),
	}
}
