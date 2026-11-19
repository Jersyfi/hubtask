// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"encoding/base64"
	"strings"
	"unicode/utf8"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/coverimages"
)

// The use case names and trail codes of the image library a cover may take a picture from
// (E-18, UC-DOC-16 check 5).
const (
	ReadCoverImagesPolicyName = "ReadCoverImagesPolicy"
	SetCoverImagesPolicyName  = "SetCoverImagesPolicy"
	SearchCoverImagesName     = "SearchCoverImages"
	FetchCoverImageName       = "FetchCoverImage"

	CoverImagesPolicyReadAction audit.Action = "page_cover_images.policy_read"
	CoverImagesPolicySetAction  audit.Action = "page_cover_images.policy_set"
	CoverImagesSearchedAction   audit.Action = "page.cover_images_searched"
	CoverImageFetchedAction     audit.Action = "page.cover_image_fetched"

	coverImagesTarget = "page_cover_images"
	maxCoverQuery     = 100
)

// CoverLibrary is what an image library for covers needs beside the space. A nil Library offers
// none, whatever the workspace chose.
type CoverLibrary struct {
	Library  coverimages.Library
	Settings settingrepo.Settings
}

// coverImages resolves the workspace's choice inside the caller's transaction.
func (s Space) coverImages(ctx context.Context, tenant shared.ID) (setting.Resolution, error) {
	definition := domain.CoverImagesDefinition()
	if s.Covers.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, plan, err := s.Covers.Settings.Above(ctx, knowledgePrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	workspace := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: tenant}
	stored, err := s.Covers.Settings.At(ctx, []settingrepo.Place{workspace}, knowledgePrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	layers := append([]setting.Layer{instance, plan}, settingrepo.LayersOf([]settingrepo.Place{workspace}, stored)...)
	return setting.Resolve(definition, layers...), nil
}

func coverImagesOutput(r setting.Resolution) usecase.Output {
	out := usecase.Output{"library": domain.CoverImagesOf(r), "source": string(r.Source)}
	if r.Locked() {
		out["locked_by"] = string(r.LockedBy)
	}
	return out
}

// ReadCoverImagesPolicy answers which image library the workspace offers for covers.
type ReadCoverImagesPolicy struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ReadCoverImagesPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCoverImagesPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Answers which image library the workspace offers as a page cover's fourth " +
			"source, the level that decided it and the level that locked it.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: CoverImagesPolicyReadAction, TargetType: coverImagesTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Space.administer(ctx, actor, CoverImagesPolicyReadAction, pagesRead); err != nil {
				return nil, err
			}
			var r setting.Resolution
			err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				r, err = h.Space.coverImages(ctx, actor.TenantID)
				return err
			})
			if err != nil {
				return nil, err
			}
			return coverImagesOutput(r), nil
		}),
	})
}

// SetCoverImagesPolicy chooses the image library offered for covers, or none, inside what the
// installation and the plan leave.
type SetCoverImagesPolicy struct{ Space Space }

// Execute chooses.
func (h SetCoverImagesPolicy) Execute(ctx context.Context, actor appshared.ActorContext, library string) (setting.Resolution, error) {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		return setting.Resolution{}, shared.ErrForbidden.WithDetail("pages.agent_decides_policy_not")
	}
	if err := h.Space.administer(ctx, actor, CoverImagesPolicySetAction, pagesWrite); err != nil {
		return setting.Resolution{}, err
	}
	if h.Space.Covers.Settings == nil {
		return setting.Resolution{}, shared.ErrUnavailable.WithDetail("pages.cover_images_unavailable")
	}
	definition := domain.CoverImagesDefinition()
	now := h.Space.Clock.Now()
	var r setting.Resolution
	err := h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		instance, plan, err := h.Space.Covers.Settings.Above(ctx, knowledgePrefix)
		if err != nil {
			return err
		}
		entry := setting.Entry{Value: library, Hold: setting.HoldSet, By: actor.AccountID}
		if err := setting.Check(definition, setting.LevelWorkspace, setting.Resolve(definition, instance, plan), entry); err != nil {
			return err
		}
		place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
		if err := h.Space.Covers.Settings.Put(ctx, h.Space.IDs.NewID(), place, domain.CoverImagesKey, entry, now); err != nil {
			return err
		}
		if r, err = h.Space.coverImages(ctx, actor.TenantID); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, CoverImagesPolicySetAction, coverImagesTarget, actor.TenantID, []audit.Change{
			{Field: string(domain.CoverImagesKey), Classification: audit.Open, To: library},
		}, now)
	})
	return r, err
}

// Descriptor is the catalogue entry.
func (h SetCoverImagesPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetCoverImagesPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Chooses the image library a page's cover may take a picture from, or none. Off " +
			"by default, since every search leaves the installation; the installation or the plan " +
			"may lock it.",
		SideEffects: "Stores the setting and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "library", Kind: usecase.KindString, Required: true, Enum: []string{domain.CoverImagesOff, domain.CoverImagesOpenLibrary}, Description: "The library offered, or OFF."},
		},
		Audit: usecase.AuditDeclaration{Action: CoverImagesPolicySetAction, TargetType: coverImagesTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			r, err := h.Execute(ctx, actor, in.String("library"))
			if err != nil {
				return nil, err
			}
			return coverImagesOutput(r), nil
		}),
	})
}

// libraryFor answers whether a writer of the page may ask the image library: WRITE_ITEMS at the
// page's hub, and the workspace offering one.
func (s Space) libraryFor(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, action audit.Action) (bool, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return false, err
	}
	page, err := s.placeOf(ctx, actor, pageID)
	if err != nil {
		return false, err
	}
	if err := s.authorizeWrite(ctx, actor, page.HubID, page.ID, action, pageTarget); err != nil {
		return false, err
	}
	if s.Covers.Library == nil {
		return false, nil
	}
	var r setting.Resolution
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		r, err = s.coverImages(ctx, actor.TenantID)
		return err
	})
	return err == nil && domain.CoverImagesOf(r) != domain.CoverImagesOff, err
}

// SearchCoverImages finds pictures for a page's cover in the library the workspace offers. The
// query leaves the installation and is never written anywhere here (rule 10).
type SearchCoverImages struct{ Space Space }

// Descriptor is the catalogue entry.
func (h SearchCoverImages) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SearchCoverImagesName, Permission: service.PermissionWriteItems,
		Summary: "Answers whether the workspace offers an image library for covers and, with a " +
			"query, up to twelve of its openly licensed pictures with who made each and a preview " +
			"the server fetched.",
		SideEffects: "With a query and a library offered, asks the library; stores nothing.",
		TokenScope:  pagesWrite, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page whose cover it is for."},
			{Name: "query", Kind: usecase.KindString, Description: "What to look for; without it nothing is asked outside."},
		},
		Audit: usecase.AuditDeclaration{Action: CoverImagesSearchedAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			offered, err := h.Space.libraryFor(ctx, actor, id, CoverImagesSearchedAction)
			if err != nil {
				return nil, err
			}
			query := strings.TrimSpace(in.String("query"))
			if utf8.RuneCountInString(query) > maxCoverQuery {
				return nil, shared.ErrValidation.WithDetail("pages.cover_query_too_long").
					WithFields(shared.FieldError{Path: "/query", Code: "pages.cover_query_too_long"})
			}
			pictures := []any{}
			if offered && query != "" {
				found, err := h.Space.Covers.Library.Search(ctx, query)
				if err != nil {
					return nil, err
				}
				for _, picture := range found {
					pictures = append(pictures, usecase.Output{
						"id": picture.ID, "creator": picture.Creator,
						"preview": "data:" + picture.PreviewType + ";base64," + base64.StdEncoding.EncodeToString(picture.Preview),
					})
				}
			}
			return usecase.Output{"offered": offered, "pictures": pictures}, nil
		}),
	})
}

// FetchCoverImage fetches a library's picture for a page's cover, with who made it; the client
// uploads it as the cover's image, so it passes the uploads' checks like any other.
type FetchCoverImage struct{ Space Space }

// Descriptor is the catalogue entry.
func (h FetchCoverImage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: FetchCoverImageName, Permission: service.PermissionWriteItems,
		Summary: "Fetches an image library's picture for a page's cover, through the server, with " +
			"who made it for the cover's credit. Refused while the workspace offers no library.",
		SideEffects: "Asks the library; stores nothing.",
		TokenScope:  pagesWrite, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page whose cover it is for."},
			{Name: "picture_id", Kind: usecase.KindString, Required: true, Description: "The picture's id at the library, as the search gave it."},
		},
		Audit: usecase.AuditDeclaration{Action: CoverImageFetchedAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			offered, err := h.Space.libraryFor(ctx, actor, id, CoverImageFetchedAction)
			if err != nil {
				return nil, err
			}
			if !offered {
				return nil, shared.ErrConflict.WithDetail("pages.cover_images_off")
			}
			image, err := h.Space.Covers.Library.Fetch(ctx, in.String("picture_id"))
			if err != nil {
				return nil, err
			}
			credit := image.Creator
			if runes := []rune(credit); len(runes) > domain.MaxCreditLength {
				credit = string(runes[:domain.MaxCreditLength])
			}
			return usecase.Output{
				"content_base64": base64.StdEncoding.EncodeToString(image.Content),
				"content_type":   image.ContentType, "credit": credit,
			}, nil
		}),
	})
}
