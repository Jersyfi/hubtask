// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	CreatePageName       = "CreatePage"
	ReadPageName         = "ReadPage"
	ListPagesName        = "ListPages"
	UpdatePageName       = "UpdatePage"
	MovePageName         = "MovePage"
	TrashPageName        = "TrashPage"
	RestorePageName      = "RestorePage"
	ListTrashedPagesName = "ListTrashedPages"
	VerifyPageName       = "VerifyPage"
)

// CreatePageCommand is the input, typed.
type CreatePageCommand struct {
	// ID is an identifier the client minted, for a page made offline; empty mints one.
	ID              shared.ID
	HubID           shared.ID
	ParentID        shared.ID
	TemplateID      shared.ID
	Title           string
	Body            *string
	Icon            string
	Template        bool
	ContentLanguage string
}

// CreatePage writes a new page in a hub's space, at the end of its parent's pages (UC-DOC-01), or
// from one of the space's templates (UC-DOC-05).
type CreatePage struct{ Space Space }

// Execute creates the page.
func (h CreatePage) Execute(ctx context.Context, actor appshared.ActorContext, cmd CreatePageCommand) (domain.Page, error) {
	if err := h.Space.authorizeWrite(ctx, actor, cmd.HubID, cmd.HubID, PageCreatedAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	id := cmd.ID
	if id.IsZero() {
		id = h.Space.IDs.NewID()
	}
	now := h.Space.Clock.Now()
	var page domain.Page
	err := h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, cmd.HubID); err != nil {
			return err
		}
		count, err := h.Space.Pages.Count(ctx, cmd.HubID)
		if err != nil {
			return err
		}
		if count >= domain.MaxPagesPerHub {
			return shared.ErrConflict.WithDetail("pages.space_full").WithParams(map[string]string{"maximum": "5000"})
		}
		if !cmd.ParentID.IsZero() {
			parent, err := h.Space.find(ctx, cmd.ParentID, false)
			if err != nil {
				return err
			}
			ancestors, err := h.Space.Pages.Ancestors(ctx, parent.ID)
			if err != nil {
				return err
			}
			if err := domain.CheckParent(id, parent, ancestorIDs(ancestors), cmd.HubID); err != nil {
				return err
			}
		}
		body := ""
		if cmd.Body != nil {
			body = *cmd.Body
		}
		if !cmd.TemplateID.IsZero() {
			template, err := h.Space.find(ctx, cmd.TemplateID, false)
			if err != nil {
				return err
			}
			// A space starts pages from its own templates: another space's would carry its words
			// past that space's readers.
			if template.HubID != cmd.HubID {
				return shared.ErrNotFound.WithDetail("pages.template_not_found")
			}
			if !template.Template {
				return shared.ErrValidation.WithDetail("pages.template_not_template").
					WithFields(shared.FieldError{Path: "/template_id", Code: "pages.template_not_template"})
			}
			if cmd.Body == nil {
				body = template.Body
			}
		}
		last, err := h.Space.Pages.LastOrderKey(ctx, cmd.HubID, cmd.ParentID)
		if err != nil {
			return err
		}
		key, err := service.OrderKeyAfter(last)
		if err != nil {
			return err
		}
		page, err = domain.NewPage(domain.NewPageInput{
			ID: id, TenantID: actor.TenantID, HubID: cmd.HubID, ParentID: cmd.ParentID, Title: cmd.Title,
			Body: body, Icon: cmd.Icon, OrderKey: key, Template: cmd.Template,
			ContentLanguage: cmd.ContentLanguage, CreatedBy: actor.AccountID, Now: now, Text: h.Space.Text,
		})
		if err != nil {
			return err
		}
		if err := h.Space.Pages.Insert(ctx, page); err != nil {
			return err
		}
		if err := h.Space.snapshot(ctx, page, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Space.announce(ctx, actor, page); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageCreatedAction, pageTarget, page.ID,
			changed([]string{domain.FieldTitle, domain.FieldBody}, page), now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h CreatePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePageName, Permission: service.PermissionWriteItems,
		Summary: "Writes a new page in a hub's space - at the top or under another page, at the end of " +
			"its siblings - with a title and a Markdown body whose `hubtask:` links are mentions, cards " +
			"and live views. Started from one of the space's templates, it takes the template's body " +
			"unless one is given.",
		SideEffects: "Writes the page, its first version and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose space the page joins."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "The page it sits under; empty for the top of the space."},
			{Name: "title", Kind: usecase.KindString, Required: true, Description: "The page's title, one line."},
			{Name: "body", Kind: usecase.KindString, Description: "The body in Markdown."},
			{Name: "icon", Kind: usecase.KindString, Description: "An emoji or short symbol shown beside the title."},
			{Name: "template", Kind: usecase.KindBool, Description: "Offer the page as a template of the space."},
			{Name: "template_id", Kind: usecase.KindID, Description: "A template of the same space to start from."},
			{Name: "content_language", Kind: usecase.KindString, Description: "The BCP-47 tag search stems the page by."},
			{Name: "id", Kind: usecase.KindID, CallerOnly: true, Description: "An identifier the client minted for a page made offline."},
		},
		Audit: usecase.AuditDeclaration{Action: PageCreatedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids := map[string]shared.ID{}
			for _, field := range []string{"hub_id", "parent_id", "template_id", "id"} {
				id, err := optionalID(in, field)
				if err != nil {
					return nil, err
				}
				ids[field] = id
			}
			page, err := h.Execute(ctx, actor, CreatePageCommand{
				ID: ids["id"], HubID: ids["hub_id"], ParentID: ids["parent_id"], TemplateID: ids["template_id"],
				Title: in.String("title"), Body: rawText(in, "body"), Icon: in.String("icon"),
				Template: in.Bool("template"), ContentLanguage: in.String("content_language"),
			})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, true, h.Space.Clock.Now()), nil
		}),
	})
}

// ReadPage reads one page with its body, the pages above it and whether the reader may write it.
type ReadPage struct{ Space Space }

// Read is a page as ReadPage answers it.
type Read struct {
	Page      domain.Page
	Ancestors []usecase.Output
	Writable  bool
}

// Execute reads the page.
func (h ReadPage) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (Read, error) {
	page, err := h.Space.readable(ctx, actor, id, PageReadAction)
	if err != nil {
		return Read{}, err
	}
	out := Read{Page: page, Ancestors: []usecase.Output{}}
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		ancestors, err := h.Space.Pages.Ancestors(ctx, page.ID)
		for i := len(ancestors) - 1; i >= 0; i-- {
			out.Ancestors = append(out.Ancestors, usecase.Output{"id": ancestors[i].ID.String(), "title": ancestors[i].Title})
		}
		return err
	})
	if err != nil {
		return Read{}, err
	}
	out.Writable, err = h.Space.mayWrite(ctx, actor, page.HubID)
	return out, err
}

// Descriptor is the catalogue entry.
func (h ReadPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPageName, Permission: service.PermissionRead,
		Summary: "Reads one page with its Markdown body, its breadcrumb (the pages above it, from the " +
			"top of the space down) and whether the reader may write it. A page in the trash is not found.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			read, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			out := PageOutput(read.Page, true, h.Space.Clock.Now())
			out["ancestors"], out["writable"] = read.Ancestors, read.Writable
			return out, nil
		}),
	})
}

// ListPages reads a hub's space: its pages without their bodies, in tree order.
type ListPages struct{ Space Space }

// ListPagesQuery is the input, typed.
type ListPagesQuery struct {
	HubID           shared.ID
	IncludeArchived bool
	TemplatesOnly   bool
}

// Execute reads the tree.
func (h ListPages) Execute(ctx context.Context, actor appshared.ActorContext, query ListPagesQuery) ([]domain.Page, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(query.HubID, service.PermissionRead,
		PageReadAction, pagesRead, pageTarget, query.HubID)); err != nil {
		return nil, err
	}
	var pages []domain.Page
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, query.HubID); err != nil {
			return err
		}
		var err error
		pages, err = h.Space.Pages.List(ctx, query.HubID, query.IncludeArchived, query.TemplatesOnly, domain.MaxPagesPerHub)
		return err
	})
	return pages, err
}

// Descriptor is the catalogue entry.
func (h ListPages) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPagesName, Permission: service.PermissionRead,
		Summary: "Reads a hub's space: every page without its body, each with its parent and position, " +
			"parents before their children. Archived pages only when asked; templates alone when asked.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose space is read."},
			{Name: "include_archived", Kind: usecase.KindBool, Description: "Keep the archived pages in the answer."},
			{Name: "templates", Kind: usecase.KindBool, Description: "Answer only the space's templates."},
		},
		Audit: usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			pages, err := h.Execute(ctx, actor, ListPagesQuery{
				HubID: hubID, IncludeArchived: in.Bool("include_archived"), TemplatesOnly: in.Bool("templates"),
			})
			if err != nil {
				return nil, err
			}
			return pageList(pages, h.Space.Clock.Now()), nil
		}),
	})
}

func pageList(pages []domain.Page, now time.Time) usecase.Output {
	data := make([]usecase.Output, 0, len(pages))
	for _, page := range pages {
		data = append(data, PageOutput(page, false, now))
	}
	return usecase.Output{"data": data}
}

// UpdatePageCommand is the input, typed. A nil member is left alone.
type UpdatePageCommand struct {
	PageID          shared.ID
	Change          domain.Change
	ExpectedVersion int
}

// UpdatePage writes a page's title, body, icon, template mark, language or archive state. A write
// of the title or body is kept in the page's history, one version per editing session
// (UC-DOC-06).
type UpdatePage struct{ Space Space }

// Execute writes the page.
func (h UpdatePage) Execute(ctx context.Context, actor appshared.ActorContext, cmd UpdatePageCommand) (domain.Page, error) {
	place, err := h.Space.placeOf(ctx, actor, cmd.PageID)
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageChangedAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, cmd.PageID, true)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
		}
		now := h.Space.Clock.Now()
		next, fields, err := current.Changed(cmd.Change, actor.AccountID, now, h.Space.Text)
		if err != nil {
			return err
		}
		if len(fields) == 0 {
			page = current
			return nil
		}
		if err := h.Space.Pages.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		page = next
		if next.Title != current.Title || next.Body != current.Body {
			if err := h.Space.snapshot(ctx, next, actor.AccountID, now); err != nil {
				return err
			}
		}
		if err := h.Space.announce(ctx, actor, next); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageChangedAction, pageTarget, next.ID, changed(fields, next), now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h UpdatePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UpdatePageName, Permission: service.PermissionWriteItems,
		Summary: "Writes a page's title, Markdown body, icon, template mark or content language, or " +
			"archives or restores it. A write of the title or body is kept in the page's history: the " +
			"writes of one person within ten minutes are one version. Without expected_version the " +
			"write lands on whatever stands - what live co-editing saves.",
		SideEffects: "Writes the page, its version and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "title", Kind: usecase.KindString, Description: "The title, one line."},
			{Name: "body", Kind: usecase.KindString, Description: "The whole body in Markdown."},
			{Name: "icon", Kind: usecase.KindString, Description: "An emoji or short symbol; empty takes it away."},
			{Name: "template", Kind: usecase.KindBool, Description: "Offer the page as a template of the space."},
			{Name: "content_language", Kind: usecase.KindString, Description: "The BCP-47 tag search stems the page by; empty for the workspace's."},
			{Name: "archived", Kind: usecase.KindBool, Description: "Archive (true) or bring back (false)."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a page changed since is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: PageChangedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, UpdatePageCommand{
				PageID: id, ExpectedVersion: in.Int("expected_version"),
				Change: domain.Change{
					Title: in.OptionalString("title"), Body: rawText(in, "body"), Icon: in.OptionalString("icon"),
					Template: optionalBool(in, "template"), ContentLanguage: in.OptionalString("content_language"),
					Archived: optionalBool(in, "archived"),
				},
			})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, true, h.Space.Clock.Now()), nil
		}),
	})
}

// MovePageCommand is the input, typed.
type MovePageCommand struct {
	PageID   shared.ID
	ParentID shared.ID
	// AfterID is the sibling to place the page after; empty places it first.
	AfterID         shared.ID
	ExpectedVersion int
}

// MovePage places a page under another of its space, or at the top, between two siblings.
type MovePage struct{ Space Space }

// Execute moves the page.
func (h MovePage) Execute(ctx context.Context, actor appshared.ActorContext, cmd MovePageCommand) (domain.Page, error) {
	place, err := h.Space.placeOf(ctx, actor, cmd.PageID)
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageMovedAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, cmd.PageID, true)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
		}
		if !cmd.ParentID.IsZero() {
			parent, err := h.Space.find(ctx, cmd.ParentID, false)
			if err != nil {
				return err
			}
			ancestors, err := h.Space.Pages.Ancestors(ctx, parent.ID)
			if err != nil {
				return err
			}
			if err := domain.CheckParent(current.ID, parent, ancestorIDs(ancestors), current.HubID); err != nil {
				return err
			}
		}
		siblings, err := h.Space.Pages.List(ctx, current.HubID, true, false, domain.MaxPagesPerHub)
		if err != nil {
			return err
		}
		key, err := placeAfter(siblings, current.ID, cmd.ParentID, cmd.AfterID)
		if err != nil {
			return err
		}
		next := current
		next.ParentID, next.OrderKey = cmd.ParentID, key
		now := h.Space.Clock.Now()
		next.UpdatedBy, next.UpdatedAt = actor.AccountID, now
		if err := h.Space.Pages.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		page = next
		if err := h.Space.announce(ctx, actor, next); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageMovedAction, pageTarget, next.ID, []audit.Change{
			{Field: "parent_id", Classification: audit.Open, From: idOrNil(current.ParentID), To: idOrNil(next.ParentID)},
		}, now)
	})
	return page, err
}

// placeAfter answers the order key between the sibling named and the one after it, under a parent.
func placeAfter(pages []domain.Page, moving, parent, after shared.ID) (string, error) {
	previous, next := "", ""
	found := after.IsZero()
	for _, page := range pages {
		if page.ParentID != parent || page.ID == moving {
			continue
		}
		switch {
		case after.IsZero():
			if next == "" {
				next = page.OrderKey
			}
		case page.ID == after:
			previous, found = page.OrderKey, true
		case found && next == "":
			next = page.OrderKey
		}
	}
	if !found {
		return "", shared.ErrValidation.WithDetail("pages.after_not_sibling").
			WithFields(shared.FieldError{Path: "/after_id", Code: "pages.after_not_sibling"})
	}
	return service.OrderKeyBetween(previous, next)
}

// Descriptor is the catalogue entry.
func (h MovePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: MovePageName, Permission: service.PermissionWriteItems,
		Summary: "Places a page, with the pages under it, under another page of the same space or at its " +
			"top, after a sibling or first. A page cannot move under itself or one of its own " +
			"(pages.parent_cycle), nor into another space, nor deeper than twelve levels.",
		SideEffects: "Writes the page's place and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page to move."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "The page it moves under; empty for the top of the space."},
			{Name: "after_id", Kind: usecase.KindID, Description: "The sibling it follows; empty to place it first."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: PageMovedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids := map[string]shared.ID{}
			for _, field := range []string{"page_id", "parent_id", "after_id"} {
				id, err := optionalID(in, field)
				if err != nil {
					return nil, err
				}
				ids[field] = id
			}
			page, err := h.Execute(ctx, actor, MovePageCommand{
				PageID: ids["page_id"], ParentID: ids["parent_id"], AfterID: ids["after_id"],
				ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, false, h.Space.Clock.Now()), nil
		}),
	})
}

// TrashPage puts a page and the pages under it in the trash, from where RestorePage brings them
// back (P-03).
type TrashPage struct{ Space Space }

// Execute trashes the page.
func (h TrashPage) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (int, error) {
	place, err := h.Space.placeOf(ctx, actor, id)
	if err != nil {
		return 0, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageTrashedAction, pageTarget); err != nil {
		return 0, err
	}
	var count int
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Space.Clock.Now()
		var err error
		if count, err = h.Space.Pages.Trash(ctx, id, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Space.announceSubtree(ctx, actor, id, true); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageTrashedAction, pageTarget, id, []audit.Change{
			{Field: "pages", Classification: audit.Open, To: count},
		}, now)
	})
	return count, err
}

// Descriptor is the catalogue entry.
func (h TrashPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: TrashPageName, Permission: service.PermissionWriteItems,
		Summary: "Puts a page and every page under it in the space's trash. Nothing is removed: " +
			"RestorePage brings them back together.",
		SideEffects: "Marks the pages trashed and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PageTrashedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			count, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"trashed": count}, nil
		}),
	})
}

// RestorePage brings a trashed page back, with the pages that went into the trash with it.
type RestorePage struct{ Space Space }

// Execute restores the page.
func (h RestorePage) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Page, error) {
	var trashed domain.Page
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		trashed, err = h.Space.Pages.Find(ctx, id)
		return err
	})
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, trashed.HubID, trashed.ID, PageRestoredAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	if !trashed.IsTrashed() {
		return domain.Page{}, shared.ErrConflict.WithDetail("pages.not_trashed")
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !trashed.ParentID.IsZero() {
			// A page whose parent is still in the trash comes back at the top of the space.
			if _, err := h.Space.find(ctx, trashed.ParentID, false); err != nil {
				current, err := h.Space.Pages.FindForUpdate(ctx, trashed.ID)
				if err != nil {
					return err
				}
				current.ParentID = ""
				if err := h.Space.Pages.Update(ctx, current, current.Version); err != nil {
					return err
				}
			}
		}
		now := h.Space.Clock.Now()
		count, err := h.Space.Pages.Restore(ctx, trashed.ID, actor.AccountID, *trashed.DeletedAt, now)
		if err != nil {
			return err
		}
		if page, err = h.Space.Pages.Find(ctx, trashed.ID); err != nil {
			return err
		}
		if err := h.Space.announceSubtree(ctx, actor, page.ID, false); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageRestoredAction, pageTarget, page.ID, []audit.Change{
			{Field: "pages", Classification: audit.Open, To: count},
		}, now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h RestorePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RestorePageName, Permission: service.PermissionWriteItems,
		Summary: "Brings a trashed page back with the pages that went into the trash with it. A page " +
			"whose parent is still in the trash comes back at the top of the space.",
		SideEffects: "Restores the pages and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The trashed page."}},
		Audit:       usecase.AuditDeclaration{Action: PageRestoredAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return PageOutput(page, false, h.Space.Clock.Now()), nil
		}),
	})
}

// ListTrashedPages reads a space's trash.
type ListTrashedPages struct{ Space Space }

// Execute reads the trash.
func (h ListTrashedPages) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) ([]domain.Page, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(hubID, service.PermissionRead,
		PageReadAction, pagesRead, pageTarget, hubID)); err != nil {
		return nil, err
	}
	var pages []domain.Page
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		pages, err = h.Space.Pages.Trashed(ctx, hubID, domain.MaxPagesPerHub)
		return err
	})
	return pages, err
}

// Descriptor is the catalogue entry.
func (h ListTrashedPages) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListTrashedPagesName, Permission: service.PermissionRead,
		Summary:     "Reads a space's trash: the trashed pages without their bodies, the latest first.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose trash is read."}},
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			pages, err := h.Execute(ctx, actor, hubID)
			if err != nil {
				return nil, err
			}
			return pageList(pages, h.Space.Clock.Now()), nil
		}),
	})
}

// VerifyPageCommand is the input, typed.
type VerifyPageCommand struct {
	PageID shared.ID
	// OwnerID is who answers for the page; empty leaves the owner as it is.
	OwnerID shared.ID
	// Until is when the page is due for review again; nil takes the verification away.
	Until *time.Time
}

// VerifyPage marks a page as known to be right until a date, by the person who checked it, with
// the owner who answers for it (verification dates, M3).
type VerifyPage struct{ Space Space }

// Execute verifies the page.
func (h VerifyPage) Execute(ctx context.Context, actor appshared.ActorContext, cmd VerifyPageCommand) (domain.Page, error) {
	place, err := h.Space.placeOf(ctx, actor, cmd.PageID)
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageVerifiedAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	now := h.Space.Clock.Now()
	if err := domain.CheckVerification(cmd.Until, now); err != nil {
		return domain.Page{}, err
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, cmd.PageID, true)
		if err != nil {
			return err
		}
		next := current
		if !cmd.OwnerID.IsZero() {
			next.OwnerID = cmd.OwnerID
		}
		next.VerifiedUntil, next.VerifiedAt, next.VerifiedBy = cmd.Until, nil, ""
		if cmd.Until != nil {
			at := now
			next.VerifiedAt, next.VerifiedBy = &at, actor.AccountID
		}
		next.UpdatedBy, next.UpdatedAt = actor.AccountID, now
		if err := h.Space.Pages.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		page = next
		if err := armReview(ctx, h.Space.Reviews, actor.TenantID, next, now); err != nil {
			return err
		}
		if err := h.Space.announce(ctx, actor, next); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageVerifiedAction, pageTarget, next.ID, []audit.Change{
			{Field: "verified_until", Classification: audit.Open, To: timeOrNil(next.VerifiedUntil)},
			{Field: "owner_id", Classification: audit.Open, To: idOrNil(next.OwnerID)},
		}, now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h VerifyPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: VerifyPageName, Permission: service.PermissionWriteItems,
		Summary: "Marks a page as checked and right until a date, by the caller, and names who answers " +
			"for it. From that date on the page is due for review. Without a date the verification is " +
			"taken away.",
		SideEffects: "Writes the page's verification and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "verified_until", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "When the page is due for review again; empty takes the verification away."},
			{Name: "owner_id", Kind: usecase.KindID, Description: "The account that answers for the page; empty leaves it."},
		},
		Audit: usecase.AuditDeclaration{Action: PageVerifiedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			owner, err := optionalID(in, "owner_id")
			if err != nil {
				return nil, err
			}
			var until *time.Time
			if raw := in.String("verified_until"); raw != "" {
				at, err := time.Parse(time.RFC3339, raw)
				if err != nil {
					return nil, shared.ErrValidation.WithDetail("pages.verified_until_malformed").
						WithFields(shared.FieldError{Path: "/verified_until", Code: "pages.verified_until_malformed"})
				}
				at = at.UTC()
				until = &at
			}
			page, err := h.Execute(ctx, actor, VerifyPageCommand{PageID: id, OwnerID: owner, Until: until})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, false, h.Space.Clock.Now()), nil
		}),
	})
}

func ancestorIDs(ancestors []repositoryAncestor) []shared.ID {
	ids := make([]shared.ID, 0, len(ancestors))
	for _, ancestor := range ancestors {
		ids = append(ids, ancestor.ID)
	}
	return ids
}
