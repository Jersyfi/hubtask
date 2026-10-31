// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadEntryRequesterName = "ReadEntryRequester"

	// EntryRequesterReadAction is what a refused read of a request's requester is recorded under.
	EntryRequesterReadAction audit.Action = "entry_requester.read"

	entryRequesterTarget = "entry_requester"
)

// EntryRequester is who raised a request and the portal organisation of their mail domain.
type EntryRequester struct {
	AccountID    shared.ID
	Organisation *domain.Organisation
}

// ReadEntryRequester answers who raised an entry and their organisation (UC-SVC-03 check 7), for
// the panel a handler reads beside a request. The address is read to find the organisation and
// never answered: a handler replies through the entry, and the requester's other requests are a
// query by creator, which answers only what the reader may see.
type ReadEntryRequester struct {
	Desk       Desk
	Items      workrepo.Items
	Containers workrepo.Containers
	Accounts   identityrepo.Accounts
}

// Execute answers the entry's requester.
func (h ReadEntryRequester) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (EntryRequester, error) {
	if err := actor.RequireScope(deskRead); err != nil {
		return EntryRequester{}, err
	}
	var (
		creator, assignee shared.ID
		path              []identity.Scope
	)
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Items.Find(ctx, itemID)
		if err != nil {
			return err
		}
		if found.IsTrashed() {
			return appshared.ItemNotFound(itemID)
		}
		creator, assignee = found.CreatedBy, found.AssigneeID
		container, err := h.Containers.Find(ctx, found.CollectionID)
		if err != nil {
			return err
		}
		path = service.ContainerScopes(container)
		return nil
	})
	if err != nil {
		return EntryRequester{}, err
	}
	if err := h.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: path, Action: EntryRequesterReadAction,
		TokenScope: deskRead, TargetType: entryRequesterTarget, TargetID: itemID,
		On: access.ItemSubject{Does: service.ItemRead, ID: itemID, Assignee: assignee},
	}); err != nil {
		return EntryRequester{}, err
	}
	out := EntryRequester{AccountID: creator}
	err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		organisation, found, err := organisationOf(ctx, h.Accounts, h.Desk.Organisations, creator)
		if found {
			out.Organisation = &organisation
		}
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ReadEntryRequester) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadEntryRequesterName, Permission: service.PermissionRead,
		Summary: "Who raised an entry - its creator - and the portal organisation their mail domain " +
			"belongs to, if any. The address is not answered; the requester's other requests are a " +
			"query by created_by.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit: usecase.AuditDeclaration{Action: EntryRequesterReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			found, err := h.Execute(ctx, actor, itemID)
			if err != nil {
				return nil, err
			}
			out := usecase.Output{"account_id": found.AccountID.String(), "organisation": nil}
			if found.Organisation != nil {
				out["organisation"] = usecase.Output{"key": found.Organisation.Key, "name": found.Organisation.Name}
			}
			return out, nil
		}),
	}, entryRequesterTarget)
}
