// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListCiAttachmentsName = "ListCiAttachments"

	// ciTarget is what the trail calls an inventory object, as the inventory module does.
	ciTarget = "cmdb_ci"

	// CiFilesReadAction is the audit code of a refused read of an object's files.
	CiFilesReadAction audit.Action = "cmdb.ci_read"
)

// CiPlaces finds the inventory object whose hub decides the read.
type CiPlaces interface {
	FindCI(ctx context.Context, id shared.ID, lock bool) (cmdb.CI, error)
}

// ListCiAttachments reads an inventory object's files as media objects (UC-AST-01; F-1, IN-2).
//
// Here rather than beside AttachCiMedia for the reason ListAttachments is: the answer is a page of
// media records, and its projection is GetMedia's. The permission question is the object's: READ
// on its hub.
type ListCiAttachments struct {
	CIs        CiPlaces
	Files      repository.CiFiles
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// Execute returns one page of the object's files, oldest upload first, without download targets.
func (h ListCiAttachments) Execute(ctx context.Context, actor appshared.ActorContext, ciID shared.ID, cursor string, size int) (repository.ObjectPage, error) {
	var ci cmdb.CI
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		ci, err = h.CIs.FindCI(ctx, ciID, false)
		return err
	})
	if err != nil {
		return repository.ObjectPage{}, err
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(ci.HubID)},
		Action:     CiFilesReadAction,
		TokenScope: mediaRead,
		TargetType: ciTarget,
		TargetID:   ciID,
	}); err != nil {
		return repository.ObjectPage{}, err
	}
	var page repository.ObjectPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Files.ListForCi(ctx, ciID, workrepo.Page{Cursor: cursor, Size: pageSize(size)})
		return err
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h ListCiAttachments) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListCiAttachmentsName,
		Module:     module.Assets,
		Permission: service.PermissionRead,
		Summary: "Reads a configuration item's files as media objects, oldest upload first. Readable by whoever " +
			"reads the item's hub. No download target per row - one is read for one file through GetMedia.",
		SideEffects: "None. Reads only.",
		TokenScope:  mediaRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "cursor", Kind: usecase.KindString, Description: "The opaque cursor of the previous page."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many files to return. Clamped to the contract's maximum."},
		},
		Audit: usecase.AuditDeclaration{Action: CiFilesReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a configuration item is not an entry, and a read writes no history.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, id, in.String("cursor"), in.Int("size"))
			if err != nil {
				return nil, err
			}
			objects := make([]usecase.Output, 0, len(page.Objects))
			for _, object := range page.Objects {
				objects = append(objects, mediaOutput(object))
			}
			return pageOutput(objects, page.Info), nil
		}),
	}
}
