// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// AttachPageMediaName is the use case's name (UC-DOC-14).
const AttachPageMediaName = "AttachPageMedia"

// PageMediaAttachedAction is the audit code of a file a page came to hold.
const PageMediaAttachedAction audit.Action = "page.media_attached"

// MaxMediaPerPage bounds a page's files; the bytes are bounded by the workspace's storage ceiling
// at the upload (multi-tenancy.md §4).
const MaxMediaPerPage = 500

// MediaObjects reads the uploads a page's pictures and files are.
type MediaObjects interface {
	Find(ctx context.Context, id shared.ID) (media.Object, error)
	// ReferencingPages answers the live pages that already hold an object.
	ReferencingPages(ctx context.Context, mediaID shared.ID) ([]shared.ID, error)
}

func mediaNotFound() error { return shared.ErrNotFound.WithDetail("pages.media_not_found") }

// AttachPageMedia makes a confirmed upload one of a page's pictures and files, so that the page's
// body draws it and every reader of the page reads it (UC-DOC-14).
//
// Holding is what grants reading, so it is judged here and not by the body: the object is one the
// writer uploaded, or one a page they may read already holds - a picture copied from one page to
// another. Anything else is not found, as GetMedia answers it (T-04).
type AttachPageMedia struct{ Space Space }

// Execute attaches the object.
func (h AttachPageMedia) Execute(ctx context.Context, actor appshared.ActorContext, pageID, mediaID shared.ID) error {
	if h.Space.Media == nil {
		return mediaNotFound()
	}
	if err := actor.RequireScope(pagesWrite); err != nil {
		return err
	}
	page, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return err
	}
	if err := h.Space.authorizeWrite(ctx, actor, page.HubID, page.ID, PageMediaAttachedAction, pageTarget); err != nil {
		return err
	}
	var object media.Object
	var holders []shared.ID
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if object, err = h.Space.Media.Find(ctx, mediaID); err != nil {
			return err
		}
		holders, err = h.Space.Media.ReferencingPages(ctx, mediaID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (object.Status != media.StatusReady || object.DeletedAt != nil)) {
		return mediaNotFound()
	}
	if err != nil {
		return err
	}
	if object.CreatedBy != actor.AccountID {
		mayRead, err := h.Space.readsAnyPage(ctx, actor, holders)
		if err != nil {
			return err
		}
		if !mayRead {
			return mediaNotFound()
		}
	}
	now := h.Space.Clock.Now()
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.find(ctx, page.ID, true); err != nil {
			return err
		}
		count, err := h.Space.Pages.CountMedia(ctx, page.ID)
		if err != nil {
			return err
		}
		if count >= MaxMediaPerPage {
			return shared.ErrConflict.WithDetail("pages.media_full").
				WithParams(map[string]string{"maximum": strconv.Itoa(MaxMediaPerPage)})
		}
		if err := h.Space.Pages.AttachMedia(ctx, page.ID, mediaID); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageMediaAttachedAction, pageTarget, page.ID, []audit.Change{
			{Field: "media_id", Classification: audit.Open, To: mediaID.String()},
		}, now)
	})
}

// Descriptor is the catalogue entry.
func (h AttachPageMedia) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AttachPageMediaName, Permission: service.PermissionWriteItems,
		Summary: "Makes a confirmed upload one of a page's pictures and files, so that the page's body draws it " +
			"and every reader of the page may read it: one the caller uploaded, or one a page they may read holds.",
		SideEffects: "Writes the holding and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "media_id", Kind: usecase.KindID, Required: true, Description: "A confirmed upload."},
		},
		Audit: usecase.AuditDeclaration{Action: PageMediaAttachedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			page, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			object, err := in.ID("media_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, page, object); err != nil {
				return nil, err
			}
			return usecase.Output{"media_id": object.String()}, nil
		}),
	})
}

// readsAnyPage answers, without a record, whether the actor reads one of the pages: admitted by
// its restrictions, and shared with them or in a hub they read. A page that is gone reads nothing.
func (s Space) readsAnyPage(ctx context.Context, actor appshared.ActorContext, ids []shared.ID) (bool, error) {
	if len(ids) == 0 || actor.RequireScope(pagesRead) != nil {
		return false, nil
	}
	var hubs []shared.ID
	byShare := false
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		readers, err := s.Pages.Readers(ctx, ids)
		if err != nil {
			return err
		}
		seen := map[shared.ID]bool{}
		for _, id := range ids {
			page, err := s.find(ctx, id, false)
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if !readers[id].Admits(actor.AccountID) {
				continue
			}
			if byShare, err = s.Pages.SharedWith(ctx, id, actor.AccountID); err != nil || byShare {
				return err
			}
			if !seen[page.HubID] {
				seen[page.HubID] = true
				hubs = append(hubs, page.HubID)
			}
		}
		return nil
	})
	if err != nil || byShare {
		return byShare, err
	}
	readable, err := s.readableHubs(ctx, actor, hubs)
	return len(readable) > 0, err
}

// PageMediaReach answers the media module whether a reader may read an object some pages hold
// (GetMedia, UC-DOC-14): whoever reads one of the pages reads its pictures and files.
type PageMediaReach struct{ Space Space }

// ReadsAny answers whether the actor reads one of the pages.
func (r PageMediaReach) ReadsAny(ctx context.Context, actor appshared.ActorContext, pageIDs []shared.ID) (bool, error) {
	return r.Space.readsAnyPage(ctx, actor, pageIDs)
}
