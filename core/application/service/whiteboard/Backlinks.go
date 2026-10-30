// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
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

// ListWhiteboardBacklinksName is the use case's name (UC-CNV-07).
const ListWhiteboardBacklinksName = "ListWhiteboardBacklinks"

const maxBacklinks = 200

// Texts reads the pages and entries' texts that point at an object (page_reference and
// entry_reference), and the pages' titles.
type Texts interface {
	Backlinks(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]knowledgerepo.Backlink, error)
	Summaries(ctx context.Context, ids []shared.ID) ([]knowledgerepo.Summary, error)
}

// Entries resolves the entries a backlink names.
type Entries interface {
	Entries(ctx context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error)
}

// Reader answers many permission questions at once, without a record: a backlink out of reach is
// left out, not refused (P-05).
type Reader interface {
	Permitted(ctx context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error)
}

// ListWhiteboardBacklinks answers what points at a board: pages, entries' notes and comments, and
// other boards, each only where the reader may see it (UC-CNV-07).
type ListWhiteboardBacklinks struct{ Studio Studio }

// Backlink is one text or board pointing at a board, resolved for the reader.
type Backlink struct {
	SourceKind string
	SourceID   shared.ID
	PageID     shared.ID
	EntryID    shared.ID
	BoardID    shared.ID
	Title      string
	Strength   string
}

// Execute reads the backlinks.
func (h ListWhiteboardBacklinks) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) ([]Backlink, error) {
	board, err := h.Studio.readable(ctx, actor, id, BoardReadAction)
	if err != nil {
		return nil, err
	}
	var texts []knowledgerepo.Backlink
	var pages []knowledgerepo.Summary
	var entries []workrepo.EntrySummary
	var boards []repository.Pointing
	err = h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if boards, err = h.Studio.Boards.Referencing(ctx, string(reference.KindWhiteboard), board.ID, maxBacklinks); err != nil {
			return err
		}
		if h.Studio.Texts == nil {
			return nil
		}
		if texts, err = h.Studio.Texts.Backlinks(ctx, string(reference.KindWhiteboard), board.ID, maxBacklinks); err != nil {
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
			if pages, err = h.Studio.Texts.Summaries(ctx, pageIDs); err != nil {
				return err
			}
		}
		if len(entryIDs) > 0 && h.Studio.Entries != nil {
			entries, err = h.Studio.Entries.Entries(ctx, entryIDs)
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
	for _, other := range boards {
		hubs = append(hubs, other.HubID)
	}
	readableHub, err := h.Studio.readableHubs(ctx, actor, hubs)
	if err != nil {
		return nil, err
	}
	visibleEntry, err := h.visibleEntries(ctx, actor, entries)
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
	for _, other := range boards {
		if other.ID != board.ID && readableHub[other.HubID] {
			out = append(out, Backlink{SourceKind: "WHITEBOARD", SourceID: other.ID, BoardID: other.ID, Title: other.Title, Strength: other.Strength})
		}
	}
	return out, nil
}

// readableHubs answers which of the hubs the reader may read, asked once each.
func (s Studio) readableHubs(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) (map[shared.ID]bool, error) {
	readable := map[shared.ID]bool{}
	var unique []shared.ID
	for _, hub := range hubs {
		if _, seen := readable[hub]; !seen {
			readable[hub] = false
			unique = append(unique, hub)
		}
	}
	if len(unique) == 0 || s.Reader == nil {
		return readable, nil
	}
	paths := make([][]identity.Scope, 0, len(unique))
	for _, hub := range unique {
		paths = append(paths, hubPath(hub))
	}
	allowed, err := s.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: BoardReadAction, TokenScope: boardsRead, TargetType: boardTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, hub := range unique {
		readable[hub] = i < len(allowed) && allowed[i]
	}
	return readable, nil
}

func (h ListWhiteboardBacklinks) visibleEntries(
	ctx context.Context, actor appshared.ActorContext, entries []workrepo.EntrySummary,
) (map[shared.ID]workrepo.EntrySummary, error) {
	visible := map[shared.ID]workrepo.EntrySummary{}
	if len(entries) == 0 || h.Studio.Reader == nil {
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
	allowed, err := h.Studio.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: BoardReadAction, TokenScope: boardsRead, TargetType: "work_item",
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

// BacklinkOutput is a backlink as the contract carries it (schema WhiteboardBacklinkList).
func BacklinkOutput(b Backlink) usecase.Output {
	id := func(value shared.ID) any {
		if value.IsZero() {
			return nil
		}
		return value.String()
	}
	return usecase.Output{
		"source_kind": b.SourceKind, "source_id": b.SourceID.String(), "page_id": id(b.PageID),
		"entry_id": id(b.EntryID), "whiteboard_id": id(b.BoardID), "title": b.Title, "strength": b.Strength,
	}
}

// Descriptor is the catalogue entry.
func (h ListWhiteboardBacklinks) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListWhiteboardBacklinksName, Permission: service.PermissionRead,
		Summary: "Answers what points at a whiteboard: the pages, entries' notes and comments and other boards " +
			"that mention it, show it as a card or embed it, each with its title - only those the reader may see.",
		SideEffects: "None. Reads only.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."}},
		Audit:       usecase.AuditDeclaration{Action: BoardReadAction, TargetType: boardTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			backlinks, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(backlinks))
			for _, backlink := range backlinks {
				data = append(data, BacklinkOutput(backlink))
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}
