// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const CopyPageName = "CopyPage"

// CopyPageCommand is the input, typed.
type CopyPageCommand struct {
	PageID shared.ID
	// ParentID is where the copy sits, meaningful only with ParentGiven: zero is both "the top of
	// the space" and "beside the page", and those are different requests.
	ParentID    shared.ID
	ParentGiven bool
	// Title is the copy's; empty keeps the page's - the server invents no "Copy of".
	Title string
}

// CopyPage writes a new page in the same space from a page (UC-DOC-01 check 8): its body, icon and
// language. It is a page written by the copier, so it is CreatePage's write - its permission, its
// bounds, its first version and its trail - and nothing of the page's history, comments,
// restrictions, shares or public links comes with it. A picture the body shows stays readable to
// whoever may read a page holding it, so a copy of a restricted page shows its pictures to the
// people the restriction admits alone.
type CopyPage struct{ Space Space }

// Execute copies the page.
func (h CopyPage) Execute(ctx context.Context, actor appshared.ActorContext, cmd CopyPageCommand) (domain.Page, error) {
	read, err := (ReadPage{Space: h.Space}).Execute(ctx, actor, cmd.PageID)
	if err != nil {
		return domain.Page{}, err
	}
	// A page shared on its own is read without its space; copying it would write into a space
	// the reader cannot see.
	if read.Shared {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.not_found")
	}
	source := read.Page
	parent := source.ParentID
	if cmd.ParentGiven {
		parent = cmd.ParentID
	}
	title := cmd.Title
	if title == "" {
		title = source.Title
	}
	body := source.Body
	return (CreatePage{Space: h.Space}).Execute(ctx, actor, CreatePageCommand{
		HubID: source.HubID, ParentID: parent, Title: title, Body: &body, Icon: source.Icon,
		ContentLanguage: source.ContentLanguage,
	})
}

// Descriptor is the catalogue entry.
func (h CopyPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CopyPageName, Permission: service.PermissionWriteItems,
		Summary: "Copies a page within its space: a new page with its body, icon and language, beside " +
			"it or under the page named, titled as given or as the page. Versions, comments, " +
			"restrictions, shares and public links are not copied, and the copy is no template.",
		SideEffects: "Writes the new page, its first version and an audit entry, as writing a page does.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page to copy."},
			{Name: "title", Kind: usecase.KindString, Description: "The copy's title; omitted keeps the page's."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "The page the copy sits under; null for the top of the space, omitted beside the page."},
		},
		Audit: usecase.AuditDeclaration{Action: PageCreatedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			parent, err := optionalID(in, "parent_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, CopyPageCommand{
				PageID: id, ParentID: parent, ParentGiven: in.Present("parent_id"), Title: in.String("title"),
			})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, true, h.Space.Clock.Now()), nil
		}),
	})
}
