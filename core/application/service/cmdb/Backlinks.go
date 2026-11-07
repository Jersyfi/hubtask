// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	whiteboardrepo "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ListCiBacklinksName is the use case's name (UC-AST-01 check 9).
const ListCiBacklinksName = "ListCiBacklinks"

const maxBacklinks = 200

// Texts reads the pages and entries' texts that point at an object, and the pages' titles.
type Texts interface {
	Backlinks(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]knowledgerepo.Backlink, error)
	Summaries(ctx context.Context, ids []shared.ID) ([]knowledgerepo.Summary, error)
}

// Boards reads the whiteboards that point at an object.
type Boards interface {
	Referencing(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]whiteboardrepo.Pointing, error)
}

// Entries resolves the entries a backlink names.
type Entries interface {
	Entries(ctx context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error)
}

// Reader answers many permission questions at once without a record: a backlink out of reach is
// left out, not refused (P-05).
type Reader interface {
	Permitted(ctx context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error)
}

// Pointers is what finds the texts and boards pointing at a CI; a nil part finds none.
type Pointers struct {
	Texts   Texts
	Boards  Boards
	Entries Entries
	Reader  Reader
}

// Backlink is one text or board pointing at a CI, resolved for the reader.
type Backlink struct {
	SourceKind string
	SourceID   shared.ID
	PageID     shared.ID
	EntryID    shared.ID
	BoardID    shared.ID
	Title      string
	Strength   string
}

// ListCiBacklinks answers what points at a CI: pages, entries' notes and comments, and whiteboards,
// each only where the reader may see it.
type ListCiBacklinks struct {
	Inventory Inventory
	Pointers  Pointers
}

// Execute reads the backlinks of a CI the reader may read.
func (h ListCiBacklinks) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) ([]Backlink, error) {
	ci, err := h.Inventory.placeOf(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := h.Inventory.authorizeHub(ctx, actor, ci.HubID, false, CIReadAction, ciTarget, id); err != nil {
		return nil, err
	}
	p := h.Pointers
	var texts []knowledgerepo.Backlink
	var pages []knowledgerepo.Summary
	var entries []workrepo.EntrySummary
	var boards []whiteboardrepo.Pointing
	err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if p.Boards != nil {
			if boards, err = p.Boards.Referencing(ctx, string(reference.KindCI), id, maxBacklinks); err != nil {
				return err
			}
		}
		if p.Texts == nil {
			return nil
		}
		if texts, err = p.Texts.Backlinks(ctx, string(reference.KindCI), id, maxBacklinks); err != nil {
			return err
		}
		var pageIDs, entryIDs []shared.ID
		for _, text := range texts {
			if text.SourceKind == string(reference.KindPage) {
				pageIDs = append(pageIDs, text.SourceID)
			} else {
				entryIDs = append(entryIDs, text.EntryID)
			}
		}
		if len(pageIDs) > 0 {
			if pages, err = p.Texts.Summaries(ctx, pageIDs); err != nil {
				return err
			}
		}
		if len(entryIDs) > 0 && p.Entries != nil {
			entries, err = p.Entries.Entries(ctx, entryIDs)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	hubs := make([]shared.ID, 0, len(pages)+len(boards))
	for _, page := range pages {
		hubs = append(hubs, page.HubID)
	}
	for _, board := range boards {
		hubs = append(hubs, board.HubID)
	}
	readableHub, err := p.readableHubs(ctx, actor, hubs)
	if err != nil {
		return nil, err
	}
	visibleEntry, err := p.visibleEntries(ctx, actor, entries)
	if err != nil {
		return nil, err
	}
	pageOf := map[shared.ID]knowledgerepo.Summary{}
	for _, page := range pages {
		if readableHub[page.HubID] && page.Readers.Admits(actor.AccountID) {
			pageOf[page.ID] = page
		}
	}
	out := []Backlink{}
	for _, text := range texts {
		if text.SourceKind == string(reference.KindPage) {
			if page, ok := pageOf[text.SourceID]; ok {
				out = append(out, Backlink{SourceKind: "PAGE", SourceID: page.ID, PageID: page.ID, Title: page.Title, Strength: text.Strength})
			}
			continue
		}
		if entry, ok := visibleEntry[text.EntryID]; ok {
			out = append(out, Backlink{SourceKind: text.SourceKind, SourceID: text.SourceID, EntryID: entry.ID, Title: entry.Title, Strength: text.Strength})
		}
	}
	for _, board := range boards {
		if readableHub[board.HubID] {
			out = append(out, Backlink{SourceKind: "WHITEBOARD", SourceID: board.ID, BoardID: board.ID, Title: board.Title, Strength: board.Strength})
		}
	}
	return out, nil
}

func (p Pointers) readableHubs(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) (map[shared.ID]bool, error) {
	readable := map[shared.ID]bool{}
	var unique []shared.ID
	for _, hub := range hubs {
		if _, seen := readable[hub]; !seen {
			readable[hub] = false
			unique = append(unique, hub)
		}
	}
	if len(unique) == 0 || p.Reader == nil {
		return readable, nil
	}
	paths := make([][]identity.Scope, 0, len(unique))
	for _, hub := range unique {
		paths = append(paths, hubPath(hub))
	}
	allowed, err := p.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: CIReadAction, TokenScope: cisRead, TargetType: ciTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, hub := range unique {
		readable[hub] = i < len(allowed) && allowed[i]
	}
	return readable, nil
}

func (p Pointers) visibleEntries(ctx context.Context, actor appshared.ActorContext, entries []workrepo.EntrySummary) (map[shared.ID]workrepo.EntrySummary, error) {
	visible := map[shared.ID]workrepo.EntrySummary{}
	if len(entries) == 0 || p.Reader == nil {
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
	allowed, err := p.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: CIReadAction, TokenScope: cisRead, TargetType: "work_item",
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
func (h ListCiBacklinks) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiBacklinksName, Permission: service.PermissionRead,
		Summary: "Answers what points at a configuration item: the pages, entries' notes and comments and the " +
			"whiteboards that mention it (`hubtask:ci/<id>`), show it as a card or draw it, each with its title - " +
			"only those the reader may see.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			backlinks, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(backlinks))
			for _, b := range backlinks {
				data = append(data, usecase.Output{
					"source_kind": b.SourceKind, "source_id": b.SourceID.String(), "page_id": idOrNil(b.PageID),
					"entry_id": idOrNil(b.EntryID), "whiteboard_id": idOrNil(b.BoardID), "title": b.Title, "strength": b.Strength,
				})
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}
