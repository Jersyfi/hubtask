// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
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

	// requesterAssetsShown bounds the requester's assets beside a request; requesterAssetsRead
	// is how many are read before the hubs the reader may not see are left out.
	requesterAssetsShown = 20
	requesterAssetsRead  = 100
)

// EntryRequester is who raised a request, the portal organisation of their mail domain and the
// configuration items they own that the reader may read.
type EntryRequester struct {
	AccountID    shared.ID
	Organisation *domain.Organisation
	Assets       []cmdb.CI
	// ByMail says the mail intake filed it for a sender without an account: AccountID is then
	// the intake's owner, not the requester.
	ByMail bool
}

// OwnedAssets answers the CIs an account owns across the workspace's hubs (repository/cmdb).
type OwnedAssets interface {
	CIsOwnedBy(ctx context.Context, accountID shared.ID, limit int) ([]cmdb.CI, error)
}

// ReadEntryRequester answers who raised an entry and their organisation (UC-SVC-03 check 7), for
// the panel a handler reads beside a request. The address is read to find the organisation and
// never answered: a handler replies through the entry, and the requester's other requests are a
// query by creator, which answers only what the reader may see. The requester's assets (check 9)
// are those of hubs where the reader may read CIs and assets are on; a CI left out is nobody
// refused, so nothing is audited (audit.md §4).
type ReadEntryRequester struct {
	Desk       Desk
	Items      workrepo.Items
	Containers workrepo.Containers
	Accounts   identityrepo.Accounts
	// Assets and Modules serve the requester's CIs; either nil answers none.
	Assets  OwnedAssets
	Modules ModuleStates
	// Requesters tells the entries the mail intake filed for a requester without an account, whose
	// creator is the administrator who opened the intake; nil takes every creator as the requester.
	Requesters RequesterAddresses
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
	out := EntryRequester{AccountID: creator, Assets: []cmdb.CI{}}
	var owned []cmdb.CI
	err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if h.Requesters != nil {
			// Filed by mail: the creator's organisation and CIs are the intake owner's, not the
			// requester's, who belongs to none (UC-SVC-11).
			filed, err := h.Requesters.AddressesOn(ctx, itemID)
			out.ByMail = len(filed) > 0
			if err != nil || out.ByMail {
				return err
			}
		}
		organisation, found, err := organisationOf(ctx, h.Accounts, h.Desk.Organisations, creator)
		if found {
			out.Organisation = &organisation
		}
		if err != nil || h.Assets == nil || creator.IsZero() {
			return err
		}
		owned, err = h.Assets.CIsOwnedBy(ctx, creator, requesterAssetsRead)
		return err
	})
	if err != nil {
		return EntryRequester{}, err
	}
	out.Assets, err = h.readableAssets(ctx, actor, owned)
	return out, err
}

// readableAssets keeps the CIs of the hubs where the reader may read CIs and assets are on.
func (h ReadEntryRequester) readableAssets(ctx context.Context, actor appshared.ActorContext, owned []cmdb.CI) ([]cmdb.CI, error) {
	out := []cmdb.CI{}
	if len(owned) == 0 || h.Desk.Visibility == nil || h.Modules == nil {
		return out, nil
	}
	var hubs []shared.ID
	seen := map[shared.ID]bool{}
	for _, ci := range owned {
		if !seen[ci.HubID] {
			seen[ci.HubID] = true
			hubs = append(hubs, ci.HubID)
		}
	}
	paths := make([][]identity.Scope, 0, len(hubs))
	for _, hub := range hubs {
		paths = append(paths, []identity.Scope{identity.TenantScope(), identity.HubScope(hub)})
	}
	allowed, err := h.Desk.Visibility.Permitted(ctx, actor, access.Request{
		// deskRead is the CMDB's own scope for reading a CI as well (items:read).
		Permission: service.PermissionRead, Action: EntryRequesterReadAction, TokenScope: deskRead, TargetType: entryRequesterTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	readable := map[shared.ID]bool{}
	for i, hub := range hubs {
		if i >= len(allowed) || !allowed[i] {
			continue
		}
		on, err := h.Modules.On(ctx, actor, hub)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		readable[hub] = err == nil && on.Has(module.Assets)
	}
	for _, ci := range owned {
		if readable[ci.HubID] && len(out) < requesterAssetsShown {
			out = append(out, ci)
		}
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ReadEntryRequester) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadEntryRequesterName, Permission: service.PermissionRead,
		Summary: "Who raised an entry - its creator - and the portal organisation their mail domain " +
			"belongs to, if any, and up to 20 configuration items they own in the hubs where the reader may " +
			"read them. The address is not answered; the requester's other requests are a query by created_by.",
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
			assets := make([]usecase.Output, 0, len(found.Assets))
			for _, ci := range found.Assets {
				assets = append(assets, usecase.Output{
					"id": ci.ID.String(), "hub_id": ci.HubID.String(), "class": ci.Class, "name": ci.Name, "stage": string(ci.Stage),
				})
			}
			out := usecase.Output{"account_id": found.AccountID.String(), "organisation": nil, "assets": assets, "by_mail": found.ByMail}
			if found.Organisation != nil {
				out["organisation"] = usecase.Output{"key": found.Organisation.Key, "name": found.Organisation.Name}
			}
			return out, nil
		}),
	}, entryRequesterTarget)
}
