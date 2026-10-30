// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	SearchPagesName       = "SearchPages"
	ListPageBacklinksName = "ListPageBacklinks"
	ExportPageName        = "ExportPage"

	maxSearchText   = 200
	maxSearchHits   = 50
	maxSpacesSearch = 500
	maxBacklinks    = 200
	maxExportPages  = 500
)

// readableHubs answers, among the hubs, those the reader may read, without a record.
func (s Space) readableHubs(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) ([]shared.ID, error) {
	if len(hubs) == 0 {
		return nil, nil
	}
	paths := make([][]identity.Scope, 0, len(hubs))
	for _, hub := range hubs {
		paths = append(paths, hubPath(hub))
	}
	allowed, err := s.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: PageReadAction, TokenScope: pagesRead, TargetType: pageTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	out := make([]shared.ID, 0, len(hubs))
	for i, hub := range hubs {
		if i < len(allowed) && allowed[i] {
			out = append(out, hub)
		}
	}
	return out, nil
}

// SearchPages finds pages by their words in the spaces the reader may read (UC-DOC-01).
type SearchPages struct{ Space Space }

// Execute searches.
func (h SearchPages) Execute(ctx context.Context, actor appshared.ActorContext, text string, hubID shared.ID, limit int) ([]repository.Hit, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return nil, shared.ErrValidation.WithDetail("pages.search_empty").
			WithFields(shared.FieldError{Path: "/text", Code: "pages.search_empty"})
	case utf8.RuneCountInString(text) > maxSearchText:
		return nil, shared.ErrValidation.WithDetail("pages.search_too_long").
			WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/text", Code: "pages.search_too_long"})
	}
	if limit <= 0 || limit > maxSearchHits {
		limit = maxSearchHits
	}
	var hubs []shared.ID
	if !hubID.IsZero() {
		hubs = []shared.ID{hubID}
	} else {
		err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			listed, err := h.Space.Containers.List(ctx, workrepo.ContainerQuery{
				Type: work.ContainerHub, Page: workrepo.Page{Size: maxSpacesSearch},
			})
			for _, hub := range listed.Containers {
				hubs = append(hubs, hub.ID)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	readable, err := h.Space.readableHubs(ctx, actor, hubs)
	if err != nil || len(readable) == 0 {
		return []repository.Hit{}, err
	}
	var hits []repository.Hit
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		hits, err = h.Space.Pages.Search(ctx, text, readable, limit)
		return err
	})
	return hits, err
}

// Descriptor is the catalogue entry.
func (h SearchPages) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SearchPagesName, Permission: service.PermissionRead,
		Summary: "Finds pages by their title and body, stemmed in each page's language, in one hub's space " +
			"or in every space the reader may read, best first, each with the passage that matched. " +
			"Archived and trashed pages are left out.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "text", Kind: usecase.KindString, Required: true, Description: "What to look for, in web search syntax."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "One hub's space; empty for every readable space."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 50."},
		},
		Audit: usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := optionalID(in, "hub_id")
			if err != nil {
				return nil, err
			}
			hits, err := h.Execute(ctx, actor, in.String("text"), hubID, in.Int("limit"))
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(hits))
			for _, hit := range hits {
				data = append(data, usecase.Output{
					"id": hit.ID.String(), "hub_id": hit.HubID.String(), "parent_id": idOrNil(hit.ParentID),
					"title": hit.Title, "icon": nilIfEmpty(hit.Icon), "excerpt": hit.Excerpt,
					"rank": hit.Rank, "updated_at": hit.UpdatedAt,
				})
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// ListPageBacklinks answers what links to a page: other pages and entries, each only where the
// reader may see it (UC-DOC-04).
type ListPageBacklinks struct{ Space Space }

// Execute reads the backlinks.
func (h ListPageBacklinks) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) ([]usecase.Output, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageReadAction)
	if err != nil {
		return nil, err
	}
	var backlinks []repository.Backlink
	var pages []repository.Summary
	var entries []workrepo.EntrySummary
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if backlinks, err = h.Space.Pages.Backlinks(ctx, string(reference.KindPage), page.ID, maxBacklinks); err != nil {
			return err
		}
		var pageIDs, entryIDs []shared.ID
		for _, backlink := range backlinks {
			if backlink.SourceKind == string(reference.KindPage) {
				pageIDs = append(pageIDs, backlink.SourceID)
			} else {
				entryIDs = append(entryIDs, backlink.EntryID)
			}
		}
		if len(pageIDs) > 0 {
			if pages, err = h.Space.Pages.Summaries(ctx, pageIDs); err != nil {
				return err
			}
		}
		if len(entryIDs) > 0 {
			entries, err = h.Space.Entries.Entries(ctx, entryIDs)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	hubs := make([]shared.ID, 0, len(pages))
	for _, summary := range pages {
		hubs = append(hubs, summary.HubID)
	}
	readable, err := h.Space.readableHubs(ctx, actor, hubs)
	if err != nil {
		return nil, err
	}
	seenHub := map[shared.ID]bool{}
	for _, hub := range readable {
		seenHub[hub] = true
	}
	visiblePages := map[shared.ID]repository.Summary{}
	for _, summary := range pages {
		if seenHub[summary.HubID] {
			visiblePages[summary.ID] = summary
		}
	}
	visibleEntries, err := h.visibleEntries(ctx, actor, entries)
	if err != nil {
		return nil, err
	}
	out := []usecase.Output{}
	for _, backlink := range backlinks {
		switch {
		case backlink.SourceKind == string(reference.KindPage):
			if summary, ok := visiblePages[backlink.SourceID]; ok {
				out = append(out, usecase.Output{
					"source_kind": "PAGE", "source_id": summary.ID.String(), "page_id": summary.ID.String(),
					"entry_id": nil, "title": summary.Title, "strength": backlink.Strength,
				})
			}
		default:
			if summary, ok := visibleEntries[backlink.EntryID]; ok {
				out = append(out, usecase.Output{
					"source_kind": backlink.SourceKind, "source_id": backlink.SourceID.String(), "page_id": nil,
					"entry_id": summary.ID.String(), "title": summary.Title, "strength": backlink.Strength,
				})
			}
		}
	}
	return out, nil
}

func (h ListPageBacklinks) visibleEntries(
	ctx context.Context, actor appshared.ActorContext, entries []workrepo.EntrySummary,
) (map[shared.ID]workrepo.EntrySummary, error) {
	visible := map[shared.ID]workrepo.EntrySummary{}
	if len(entries) == 0 {
		return visible, nil
	}
	paths := make([][]identity.Scope, 0, len(entries))
	for _, s := range entries {
		path := []identity.Scope{identity.TenantScope()}
		if !s.HubID.IsZero() {
			path = append(path, identity.HubScope(s.HubID))
		}
		paths = append(paths, append(path, identity.CollectionScope(s.CollectionID), identity.ItemScope(s.ID)))
	}
	allowed, err := h.Space.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: PageReadAction, TokenScope: pagesRead, TargetType: "work_item",
		On: access.ItemSubject{Does: service.ItemRead},
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, s := range entries {
		if i < len(allowed) && allowed[i] {
			visible[s.ID] = s
		}
	}
	return visible, nil
}

// Descriptor is the catalogue entry.
func (h ListPageBacklinks) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPageBacklinksName, Permission: service.PermissionRead,
		Summary: "Answers what links to a page: the pages, entries' notes and comments that mention it, " +
			"show it as a card or embed it, each with its title - only those the reader may see.",
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
			data, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// ExportPage writes a page, or a page and every page under it, as Markdown files with front matter
// (UC-DOC-09): what a person takes elsewhere, and what an import rebuilds the tree from.
type ExportPage struct{ Space Space }

// Execute exports.
func (h ExportPage) Execute(
	ctx context.Context, actor appshared.ActorContext, pageID shared.ID, subtree bool, format domain.Format,
) ([]domain.File, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageExportedAction)
	if err != nil {
		return nil, err
	}
	pages := []domain.Page{page}
	if subtree {
		err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			pages, err = h.Space.Pages.Subtree(ctx, page.ID, false, maxExportPages)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	files, err := domain.TreeIn(pages, page.ID.String(), format)
	if err != nil {
		return nil, err
	}
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Space.record(ctx, actor, PageExportedAction, pageTarget, page.ID, []audit.Change{
			{Field: "pages", Classification: audit.Open, To: len(files)},
			{Field: "format", Classification: audit.Open, To: string(format)},
		}, h.Space.Clock.Now())
	})
	return files, err
}

// Descriptor is the catalogue entry.
func (h ExportPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ExportPageName, Permission: service.PermissionRead,
		Summary: "Exports a page - or a page and every page under it, at most 500 - as Markdown files with " +
			"front matter (id, title, parent), standalone HTML documents or DOCX documents (base64), each " +
			"under a folder named after its parent. References stay `hubtask:` addresses.",
		SideEffects: "Writes an audit entry: an export takes words out of the workspace.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "subtree", Kind: usecase.KindBool, Description: "Export the pages under it as well."},
			{Name: "format", Kind: usecase.KindString, Description: "MARKDOWN (the default), HTML or DOCX."},
		},
		Audit: usecase.AuditDeclaration{Action: PageExportedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			format, err := domain.ParseFormat(in.String("format"))
			if err != nil {
				return nil, err
			}
			files, err := h.Execute(ctx, actor, id, in.Bool("subtree"), format)
			if err != nil {
				return nil, err
			}
			encoding := "TEXT"
			if format.Binary() {
				encoding = "BASE64"
			}
			data := make([]usecase.Output, 0, len(files))
			for _, file := range files {
				data = append(data, usecase.Output{"page_id": file.PageID, "path": file.Path, "content": file.Content, "encoding": encoding})
			}
			return usecase.Output{"format": string(format), "files": data}, nil
		}),
	})
}

// ModulesOn answers the modules on at a hub, or at the workspace for the zero hub.
type ModulesOn interface {
	On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (module.Set, error)
}

// PageReach answers a query's pages summary which spaces it counts (F-9): those the reader may
// read pages in, the question SearchPages asks. A hub the request named alone is asked with its
// refusal recorded; with the knowledge module off at the workspace, nothing is counted but refused.
type PageReach struct {
	Space Space
	// Modules is the module switch; unwired, the module counts as on.
	Modules ModulesOn
}

// Readable answers the readable spaces among the hubs.
func (r PageReach) Readable(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID, named bool) ([]shared.ID, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	if r.Modules != nil {
		on, err := r.Modules.On(ctx, actor, shared.ID(""))
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		if err := on.Require(module.Knowledge); err != nil {
			return nil, err
		}
	}
	if named && len(hubs) == 1 {
		if err := r.Space.Authorizer.Authorize(ctx, actor,
			r.Space.request(hubs[0], service.PermissionRead, PageReadAction, pagesRead, pageTarget, hubs[0])); err != nil {
			return nil, err
		}
		return hubs, nil
	}
	return r.Space.readableHubs(ctx, actor, hubs)
}
