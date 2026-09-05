// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListTrashName = "ListTrash"
	trashRead     = "trash:read"
	// trashTarget is what an audit entry about the trash names. Its own target type rather than the
	// container's or the entry's, because the trash is one view over both and a refused read of it
	// is a refusal of the view.
	trashTarget = "trash"

	// TrashReadAction is the audit code of an attempted read. Declared even though an ordinary read
	// writes no entry: a refused one does, recorded against the action that was refused (audit.md §4).
	TrashReadAction audit.Action = "trash.read"
)

// ListTrash reads what is in the trash, newest deletion first.
//
// One entry per deletion rather than per deleted row. A hub with two hundred entries under it went
// in as one act and comes back as one act (I-C2), so what is listed is the root of each deletion -
// the thing somebody deleted - and the batch beside it is what took the rest.
//
// Read-only throughout, which is not a detail: the transaction may be served by a read replica
// (multi-tenancy.md §7), and a read that opened a write transaction would pin it to the primary.
type ListTrash struct {
	Trash      repository.Trash
	Reader     Reader
	UnitOfWork persistence.UnitOfWork
}

// ListTrashQuery is the input, typed.
type ListTrashQuery struct {
	Cursor string
	Size   int
}

// Execute returns one page of the trash.
//
// The permission check has the shape the hub level has, and for the same reason: the trash is
// anchored to nothing. It spans hubs, so there is no single path to check - and checking the tenant
// scope would refuse everybody whose membership sits on a hub rather than on the workspace, which is
// most of what hub-scoped memberships are for. So the page is read and then narrowed to what the
// actor may see: "what did I delete" is answered with what they may see, not with a 403.
func (h ListTrash) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListTrashQuery,
) (repository.TrashPage, error) {
	var page repository.TrashPage
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Trash.List(ctx, repository.Page{
			Cursor: query.Cursor, Size: PageSize(query.Size),
		})
		return err
	})
	if err != nil {
		return repository.TrashPage{}, err
	}

	if page.Entries, err = h.visible(ctx, actor, page.Entries); err != nil {
		return repository.TrashPage{}, err
	}
	return page, nil
}

// visible drops the entries the actor may not read.
//
// The page's own cursor is left alone deliberately. It is a boundary in the *scanned* set, not in
// the filtered one, which is what keeps the walk correct: a client sees shorter pages and pages on
// until has_more is false. Narrowing the cursor to the last visible row would skip everything
// between it and the last row actually read.
func (h ListTrash) visible(
	ctx context.Context, actor appshared.ActorContext, entries []domain.TrashEntry,
) ([]domain.TrashEntry, error) {
	if len(entries) == 0 {
		return entries, nil
	}

	paths := make([][]identity.Scope, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, trashEntryPath(entry))
	}

	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     TrashReadAction,
		TokenScope: trashRead,
		TargetType: trashTarget,
	}, paths)
	if err != nil {
		return nil, err
	}

	visible := make([]domain.TrashEntry, 0, len(entries))
	for i, entry := range entries {
		if i < len(allowed) && allowed[i] {
			visible = append(visible, entry)
		}
	}
	return visible, nil
}

// trashEntryPath is the scope chain a deleted thing is judged against.
//
// It is built from the projection rather than from a re-read, which is what the hub on the entry is
// for: a membership held at a hub applies downwards, so an entry that named only its collection
// could not be shown to somebody whose right sits above it (domain-model.md §3.2). A hub is its own
// level and names no parent.
func trashEntryPath(entry domain.TrashEntry) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !entry.HubID.IsZero() {
		path = append(path, identity.HubScope(entry.HubID))
	}

	switch {
	case entry.Kind == domain.TrashContainerKind && entry.HubID.IsZero():
		// A hub. It is the level itself rather than something inside one.
		path = append(path, identity.HubScope(entry.ID))
	case entry.Kind == domain.TrashContainerKind:
		path = append(path, identity.CollectionScope(entry.ID))
	case !entry.CollectionID.IsZero():
		path = append(path, identity.CollectionScope(entry.CollectionID))
	}
	return path
}

// Descriptor is the catalogue entry.
func (h ListTrash) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListTrashName,
		Summary: "Lists what is in the trash, newest deletion first: one entry per deletion, not " +
			"one per deleted row. What is listed is the root of each deletion - the thing somebody " +
			"deleted - with the batch that took the rest beside it. The list spans hubs and is " +
			"narrowed to what the caller may see rather than refused, so a page can be shorter than " +
			"the size asked for; walk on until has_more is false.",
		SideEffects: "None. Reads only.",
		TokenScope:  trashRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the newest " +
					"deletion.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many deletions to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TrashReadAction, TargetType: trashTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListTrash) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	page, err := h.Execute(ctx, actor, ListTrashQuery{
		Cursor: in.String("cursor"), Size: in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	entries := make([]usecase.Output, 0, len(page.Entries))
	for _, entry := range page.Entries {
		entries = append(entries, trashEntryOutput(entry))
	}
	return pageOutput(entries, page.Info), nil
}

// trashEntryOutput is the projection as every channel renders it.
//
// The optional identifiers travel as explicit nulls rather than being omitted: a field that appeared
// only for some kinds of entry is one a client cannot read unconditionally, and this list mixes the
// two kinds by design.
func trashEntryOutput(entry domain.TrashEntry) usecase.Output {
	return usecase.Output{
		"kind":           string(entry.Kind),
		"id":             entry.ID.String(),
		"trash_batch_id": entry.BatchID.String(),
		"deleted_at":     entry.DeletedAt,
		// Nil where nothing was recorded, so the contract's null travels rather than an actor of
		// kind "". A row deleted before migration 0070 was never asked who.
		"deleted_by": func() any {
			if !entry.DeletedBy.IsKnown() {
				return nil
			}
			return usecase.Output{
				"type": string(entry.DeletedBy.Kind),
				"id":   idOrNil(entry.DeletedBy.ID),
			}
		}(),
		"title":         entry.Title,
		"subtype":       entry.Subtype,
		"hub_id":        idOrNil(entry.HubID),
		"collection_id": idOrNil(entry.CollectionID),
		"parent_id":     idOrNil(entry.ParentID),
		"version":       entry.Version,
	}
}
