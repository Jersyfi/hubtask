// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ListJumbleDestinationsName is the catalogue name.
const ListJumbleDestinationsName = "ListJumbleDestinations"

// destinationsPageSize is a picker's page: a whole workspace's collections in one read for nearly
// every workspace, and a page boundary for the rest.
const destinationsPageSize = 100

// ListJumbleDestinations answers where an arrival may become work for the person asking: the
// collections that are neither trashed nor archived and where creating an entry is permitted to
// them, private hubs they are in included, in the order of the tree.
//
// It asks what converting asks first - WRITE_ITEMS at the workspace (domain-model.md §5) - so that
// a person who could not convert anything is refused rather than handed an empty list. Each
// collection is then decided as a create there would be, through the authoriser's filter: a
// collection left out is nobody's refusal, so nothing is recorded per row (audit.md §4). A page can
// therefore be shorter than asked; the cursor is the scanned boundary, and a client pages on until
// there is no next page (ListContainers' reasoning).
type ListJumbleDestinations struct {
	Collections repository.Collections
	Authorizer  Authorizer
	Reader      Reader
	UnitOfWork  persistence.UnitOfWork
}

// Execute answers one page.
func (h ListJumbleDestinations) Execute(
	ctx context.Context, actor appshared.ActorContext, page repository.Page,
) (repository.CollectionPage, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ContainerReadAction,
		TokenScope: containersRead,
		TargetType: containerTarget,
	}); err != nil {
		return repository.CollectionPage{}, err
	}
	if page.Size <= 0 {
		page.Size = destinationsPageSize
	}

	var read repository.CollectionPage
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		read, err = h.Collections.InOrder(ctx, repository.CollectionQuery{IncludePrivate: true, Page: page})
		return err
	}); err != nil {
		return repository.CollectionPage{}, err
	}
	if len(read.Collections) == 0 {
		return read, nil
	}

	paths := make([][]identity.Scope, 0, len(read.Collections))
	for _, row := range read.Collections {
		paths = append(paths, containerPath(row.Collection))
	}
	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		// What CreateWorkItem asks at the collection's path.
		Permission: service.PermissionWriteItems,
		Action:     ItemCreatedAction,
		TokenScope: containersRead,
		TargetType: itemTarget,
		On:         access.ItemSubject{Does: service.ItemCreate},
	}, paths)
	if err != nil {
		return repository.CollectionPage{}, err
	}
	kept := read.Collections[:0:0]
	for i, row := range read.Collections {
		if i < len(allowed) && allowed[i] {
			kept = append(kept, row)
		}
	}
	read.Collections = kept
	return read, nil
}

// Descriptor is the catalogue entry.
func (h ListJumbleDestinations) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListJumbleDestinationsName,
		Permission: service.PermissionWriteItems,
		Summary: "Where a jumble entry may become work for the caller: the collections that are " +
			"neither trashed nor archived and where the caller may create an entry, private hubs " +
			"they are in included, each with its hub, in the order of the tree. Paged; a page may " +
			"be shorter than asked, and the cursor continues until there is no next page.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The next_cursor of the previous page.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "Collections scanned per page, 1 to 200. Defaults to 100.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerReadAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListJumbleDestinations) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	size := in.Int("size")
	if size > 0 {
		size = PageSize(size)
	}
	page, err := h.Execute(ctx, actor, repository.Page{Cursor: in.String("cursor"), Size: size})
	if err != nil {
		return nil, err
	}
	data := make([]usecase.Output, 0, len(page.Collections))
	for _, row := range page.Collections {
		data = append(data, usecase.Output{
			"id":          row.Collection.ID.String(),
			"name":        row.Collection.Name,
			"hub_id":      row.Collection.ParentID.String(),
			"hub_name":    row.HubName,
			"hub_private": row.HubPrivate,
		})
	}
	return pageOutput(data, page.Info), nil
}
