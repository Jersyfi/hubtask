// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"fmt"
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
	// WithSubpages brings the template's subpages along beneath the new page: a space's own space
	// template (UC-DOC-05 check 5).
	WithSubpages bool
	// Subpages are written beneath the new page in the same write: Hubtask's own space templates,
	// whose words the client writes in the reader's language (rule 8, UC-DOC-05 check 6).
	Subpages []Subpage
}

// Subpage is one page of a tree written at once, with the pages under it.
type Subpage struct {
	Title, Body, Icon string
	Subpages          []Subpage
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
	if cmd.WithSubpages && len(cmd.Subpages) > 0 {
		return domain.Page{}, shared.ErrValidation.WithDetail("pages.subpages_twice").
			WithFields(shared.FieldError{Path: "/subpages", Code: "pages.subpages_twice"})
	}
	if cmd.WithSubpages && cmd.TemplateID.IsZero() {
		return domain.Page{}, shared.ErrValidation.WithDetail("pages.subpages_without_template").
			WithFields(shared.FieldError{Path: "/with_subpages", Code: "pages.subpages_without_template"})
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
		level := 1
		if !cmd.ParentID.IsZero() {
			parent, err := h.Space.find(ctx, cmd.ParentID, false)
			if err != nil {
				return err
			}
			if err := h.Space.admitted(ctx, actor, parent.ID); err != nil {
				return err
			}
			ancestors, err := h.Space.Pages.Ancestors(ctx, parent.ID)
			if err != nil {
				return err
			}
			if err := domain.CheckParent(id, parent, ancestorIDs(ancestors), cmd.HubID); err != nil {
				return err
			}
			level = len(ancestors) + 2
		}
		plan := planSubpages(cmd.Subpages, -1, 1, nil)
		body := ""
		if cmd.Body != nil {
			body = *cmd.Body
		}
		if !cmd.TemplateID.IsZero() {
			template, err := h.Space.find(ctx, cmd.TemplateID, false)
			if err != nil {
				return err
			}
			if err := h.Space.admitted(ctx, actor, template.ID); err != nil {
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
			if cmd.WithSubpages {
				if plan, err = h.Space.templateSubpages(ctx, actor, template.ID); err != nil {
					return err
				}
			}
		}
		if err := checkPlan(plan, count, level); err != nil {
			return err
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
		// In a space with drafts a new page waits for its writer to publish it (UC-DOC-15); the
		// subpages written with it wait under it.
		unpublished, err := h.Space.drafting(ctx, actor.TenantID, cmd.HubID)
		if err != nil {
			return err
		}
		if page, err = h.Space.write(ctx, actor, page, unpublished, now); err != nil {
			return err
		}
		return h.Space.writeSubpages(ctx, actor, page, plan, now)
	})
	return page, err
}

// subpagesOf reads a tree of subpages as the channels deliver it: objects of strings with their own
// `subpages`, never deeper than a space holds.
func subpagesOf(list []any, path string, depth int) ([]Subpage, error) {
	if len(list) == 0 {
		return nil, nil
	}
	if depth > domain.MaxDepth || len(list) > domain.MaxPagesPerWrite {
		return nil, shared.ErrValidation.WithDetail("pages.too_many_at_once").
			WithParams(map[string]string{"maximum": "100"}).
			WithFields(shared.FieldError{Path: path, Code: "pages.too_many_at_once"})
	}
	out := make([]Subpage, 0, len(list))
	for i, item := range list {
		fields, ok := item.(map[string]any)
		at := fmt.Sprintf("%s/%d", path, i)
		if !ok {
			return nil, shared.ErrValidation.WithDetail("pages.subpage_invalid").
				WithFields(shared.FieldError{Path: at, Code: "pages.subpage_invalid"})
		}
		text := func(key string) string {
			value, _ := fields[key].(string)
			return value
		}
		children, _ := fields["subpages"].([]any)
		below, err := subpagesOf(children, at+"/subpages", depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, Subpage{Title: text("title"), Body: text("body"), Icon: text("icon"), Subpages: below})
	}
	return out, nil
}

// write stores a new page with its first version, its change and its trail; unpublished keeps it
// to its writer until it is published.
func (s Space) write(ctx context.Context, actor appshared.ActorContext, page domain.Page, unpublished bool, now time.Time) (domain.Page, error) {
	if err := s.Pages.Insert(ctx, page); err != nil {
		return domain.Page{}, err
	}
	if unpublished {
		var err error
		if page, err = s.markUnpublished(ctx, actor, page, now); err != nil {
			return domain.Page{}, err
		}
	}
	if err := s.snapshot(ctx, page, actor.AccountID, now); err != nil {
		return domain.Page{}, err
	}
	if err := s.announce(ctx, actor, page); err != nil {
		return domain.Page{}, err
	}
	return page, s.record(ctx, actor, PageCreatedAction, pageTarget, page.ID,
		changed([]string{domain.FieldTitle, domain.FieldBody}, page), now)
}

// planned is one subpage of a write, its parent an earlier entry of the plan or the new page (-1).
type planned struct {
	parent                      int
	depth                       int
	title, body, icon, language string
}

// planSubpages flattens a tree parents first, so that each parent is written before its pages.
func planSubpages(pages []Subpage, parent, depth int, plan []planned) []planned {
	for _, sub := range pages {
		plan = append(plan, planned{parent: parent, depth: depth, title: sub.Title, body: sub.Body, icon: sub.Icon})
		plan = planSubpages(sub.Subpages, len(plan)-1, depth+1, plan)
	}
	return plan
}

// checkPlan refuses a write past its bounds before anything of it is written.
func checkPlan(plan []planned, count, level int) error {
	if len(plan)+1 > domain.MaxPagesPerWrite {
		return shared.ErrValidation.WithDetail("pages.too_many_at_once").
			WithParams(map[string]string{"maximum": "100"}).
			WithFields(shared.FieldError{Path: "/subpages", Code: "pages.too_many_at_once"})
	}
	if count+len(plan)+1 > domain.MaxPagesPerHub {
		return shared.ErrConflict.WithDetail("pages.space_full").WithParams(map[string]string{"maximum": "5000"})
	}
	for _, sub := range plan {
		if level+sub.depth > domain.MaxDepth {
			return shared.ErrValidation.WithDetail("pages.too_deep").WithParams(map[string]string{"maximum": "12"}).
				WithFields(shared.FieldError{Path: "/subpages", Code: "pages.too_deep"})
		}
	}
	return nil
}

// templateSubpages plans a template's live subpages the writer may read: an archived page or one
// behind a restriction that does not admit them stays behind with every page under it.
func (s Space) templateSubpages(ctx context.Context, actor appshared.ActorContext, templateID shared.ID) ([]planned, error) {
	tree, err := s.Pages.Subtree(ctx, templateID, false, domain.MaxPagesPerWrite+1)
	if err != nil {
		return nil, err
	}
	admitted, err := s.keepAdmitted(ctx, actor, tree)
	if err != nil {
		return nil, err
	}
	readable := make(map[shared.ID]bool, len(admitted))
	for _, page := range admitted {
		readable[page.ID] = !page.IsArchived()
	}
	index := map[shared.ID]int{templateID: -1}
	depth := map[shared.ID]int{templateID: 0}
	var plan []planned
	for _, page := range tree {
		parent, ok := index[page.ParentID]
		if page.ID == templateID || !ok || !readable[page.ID] {
			continue
		}
		depth[page.ID] = depth[page.ParentID] + 1
		plan = append(plan, planned{
			parent: parent, depth: depth[page.ID], title: page.Title, body: page.Body, icon: page.Icon,
			language: page.ContentLanguage,
		})
		index[page.ID] = len(plan) - 1
	}
	return plan, nil
}

// writeSubpages writes a plan beneath a new page, each at the end of its parent's pages.
func (s Space) writeSubpages(ctx context.Context, actor appshared.ActorContext, root domain.Page, plan []planned, now time.Time) error {
	written := make([]shared.ID, len(plan))
	last := map[shared.ID]string{}
	for i, sub := range plan {
		parent := root.ID
		if sub.parent >= 0 {
			parent = written[sub.parent]
		}
		key, err := service.OrderKeyAfter(last[parent])
		if err != nil {
			return err
		}
		last[parent] = key
		page, err := domain.NewPage(domain.NewPageInput{
			ID: s.IDs.NewID(), TenantID: actor.TenantID, HubID: root.HubID, ParentID: parent, Title: sub.title,
			Body: sub.body, Icon: sub.icon, OrderKey: key, ContentLanguage: sub.language,
			CreatedBy: actor.AccountID, Now: now, Text: s.Text,
		})
		if err != nil {
			return err
		}
		if _, err := s.write(ctx, actor, page, false, now); err != nil {
			return err
		}
		written[i] = page.ID
	}
	return nil
}

// Descriptor is the catalogue entry.
func (h CreatePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePageName, Permission: service.PermissionWriteItems,
		Summary: "Writes a new page in a hub's space - at the top or under another page, at the end of " +
			"its siblings - with a title and a Markdown body whose `hubtask:` links are mentions, cards " +
			"and live views. Started from one of the space's templates, it takes the template's body " +
			"unless one is given, and with `with_subpages` the template's subpages beneath it; " +
			"`subpages` writes a tree of pages beneath it in the same write (a space template). At most " +
			"100 pages in one write.",
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
			{Name: "with_subpages", Kind: usecase.KindBool, Description: "Bring the template's subpages along beneath the new page."},
			{Name: "subpages", Kind: usecase.KindList, Description: "Pages written beneath the new page, each {title, body, icon, subpages}."},
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
			list, _ := in["subpages"].([]any)
			subpages, err := subpagesOf(list, "/subpages", 1)
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, CreatePageCommand{
				ID: ids["id"], HubID: ids["hub_id"], ParentID: ids["parent_id"], TemplateID: ids["template_id"],
				Title: in.String("title"), Body: rawText(in, "body"), Icon: in.String("icon"),
				Template: in.Bool("template"), ContentLanguage: in.String("content_language"),
				WithSubpages: in.Bool("with_subpages"), Subpages: subpages,
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
	// Shared marks a page read by a share alone: without the pages above it, never written.
	Shared bool
	// Drafts tells whether the page's space keeps drafts; Draft is what waits on the page, shown
	// to a writer alone (UC-DOC-15).
	Drafts bool
	Draft  *domain.Draft
}

// Execute reads the page.
func (h ReadPage) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (Read, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return Read{}, err
	}
	if page, shared, err := h.Space.sharedRead(ctx, actor, id); err != nil || shared {
		return Read{Page: page, Ancestors: []usecase.Output{}, Shared: shared}, err
	}
	page, err := h.Space.readable(ctx, actor, id, PageReadAction)
	if err != nil {
		return Read{}, err
	}
	out := Read{Page: page, Ancestors: []usecase.Output{}}
	if out.Writable, err = h.Space.mayWrite(ctx, actor, page.HubID); err != nil {
		return Read{}, err
	}
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		ancestors, err := h.Space.Pages.Ancestors(ctx, page.ID)
		if err != nil {
			return err
		}
		for i := len(ancestors) - 1; i >= 0; i-- {
			out.Ancestors = append(out.Ancestors, usecase.Output{"id": ancestors[i].ID.String(), "title": ancestors[i].Title})
		}
		if out.Drafts, err = h.Space.drafting(ctx, actor.TenantID, page.HubID); err != nil || !out.Writable {
			return err
		}
		out.Draft, err = h.Space.draftOf(ctx, page.ID)
		return err
	})
	if err != nil {
		return Read{}, err
	}
	return out, nil
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
			out["ancestors"], out["writable"], out["shared"] = read.Ancestors, read.Writable, read.Shared
			out["drafts"], out["draft"] = read.Drafts, DraftOutput(read.Draft)
			out["unpublished"] = read.Draft != nil && read.Draft.Unpublished
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
		if pages, err = h.Space.Pages.List(ctx, query.HubID, query.IncludeArchived, query.TemplatesOnly, domain.MaxPagesPerHub); err != nil {
			return err
		}
		pages, err = h.Space.keepAdmitted(ctx, actor, pages)
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
	written, err := h.write(ctx, actor, cmd)
	return written.Page, err
}

// Written is a page after a write, with the draft that waits on it in a space with drafts.
type Written struct {
	Page   domain.Page
	Draft  *domain.Draft
	Drafts bool
}

func (h UpdatePage) write(ctx context.Context, actor appshared.ActorContext, cmd UpdatePageCommand) (Written, error) {
	place, err := h.Space.placeOf(ctx, actor, cmd.PageID)
	if err != nil {
		return Written{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageChangedAction, pageTarget); err != nil {
		return Written{}, err
	}
	var out Written
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, cmd.PageID, true)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
		}
		change := cmd.Change
		// In a space with drafts a published page's title and body change in its draft; its readers
		// read what was published (UC-DOC-15). A page never published is its writer's alone and is
		// written as it is.
		if out.Drafts, err = h.Space.drafting(ctx, actor.TenantID, current.HubID); err != nil {
			return err
		}
		if out.Draft, err = h.Space.draftOf(ctx, current.ID); err != nil {
			return err
		}
		if out.Drafts && change.Touches() && (out.Draft == nil || out.Draft.Pending()) {
			var drafted domain.Change
			drafted, change = domain.DraftOnly(change)
			if out.Draft, err = h.Space.saveDraft(ctx, actor, current, out.Draft, drafted); err != nil {
				return err
			}
		}
		page, err := h.change(ctx, actor, current, change)
		out.Page = page
		return err
	})
	return out, err
}

// change writes what a change changes on the page itself.
func (h UpdatePage) change(ctx context.Context, actor appshared.ActorContext, current domain.Page, change domain.Change) (domain.Page, error) {
	var page domain.Page
	err := func() error {
		now := h.Space.Clock.Now()
		next, fields, err := current.Changed(change, actor.AccountID, now, h.Space.Text)
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
	}()
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
			written, err := h.write(ctx, actor, UpdatePageCommand{
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
			out := PageOutput(written.Page, true, h.Space.Clock.Now())
			out["drafts"], out["draft"] = written.Drafts, DraftOutput(written.Draft)
			out["unpublished"] = written.Draft != nil && written.Draft.Unpublished
			return out, nil
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
		level := 1
		if !cmd.ParentID.IsZero() {
			parent, err := h.Space.find(ctx, cmd.ParentID, false)
			if err != nil {
				return err
			}
			if err := h.Space.admitted(ctx, actor, parent.ID); err != nil {
				return err
			}
			ancestors, err := h.Space.Pages.Ancestors(ctx, parent.ID)
			if err != nil {
				return err
			}
			if err := domain.CheckParent(current.ID, parent, ancestorIDs(ancestors), current.HubID); err != nil {
				return err
			}
			level = len(ancestors) + 2
		}
		siblings, err := h.Space.Pages.List(ctx, current.HubID, true, false, domain.MaxPagesPerHub)
		if err != nil {
			return err
		}
		if err := domain.CheckSubtreeDepth(level, domain.SubtreeHeight(siblings, current.ID)); err != nil {
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
		if trashed, err = h.Space.Pages.Find(ctx, id); err != nil {
			return err
		}
		return h.Space.admitted(ctx, actor, trashed.ID)
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
		if pages, err = h.Space.Pages.Trashed(ctx, hubID, domain.MaxPagesPerHub); err != nil {
			return err
		}
		pages, err = h.Space.keepAdmitted(ctx, actor, pages)
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
