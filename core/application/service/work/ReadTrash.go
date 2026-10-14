// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
	// Policies and TombstoneWindow are the two numbers the retention pass removes by - the trash
	// period it reads and the installation's offline window the Purger is given - so that the date
	// each entry carries is the pass's own.
	Policies        lifecyclerepo.Policies
	TombstoneWindow time.Duration
	// Holds and Contributors are what says whether a legal hold keeps a deletion: its rows are
	// judged as the retention pass judges them (lifecycle.Holds.Blocking), so `held` is the pass's
	// own answer rather than a second rule.
	Holds        lifecyclerepo.HoldList
	Contributors lifecyclerepo.ContributorReader
}

// TrashListing is one page of the trash, when the automatic pass may take each entry, and which a
// legal hold keeps.
type TrashListing struct {
	repository.TrashPage
	policy lifecycle.Policy
	window time.Duration
	held   map[shared.ID]bool
}

// Held reports whether a legal hold keeps the entry's deletion: a hold blocks one of its rows, and
// the pass keeps a root while anything below it stays (data-retention.md §4 item 6).
func (l TrashListing) Held(entry domain.TrashEntry) bool {
	return l.held[entry.BatchID]
}

// PurgeAfter is the moment after which the automatic pass may remove the entry, unless a legal hold
// keeps it.
func (l TrashListing) PurgeAfter(entry domain.TrashEntry) time.Time {
	return l.policy.RemovableAfter(entry.DeletedAt, l.window)
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
) (TrashListing, error) {
	listing := TrashListing{window: h.TombstoneWindow}
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		listing.TrashPage, err = h.Trash.List(ctx, repository.Page{
			Cursor: query.Cursor, Size: PageSize(query.Size),
		})
		if err != nil {
			return err
		}
		listing.policy, err = h.trashPolicy(ctx)
		return err
	})
	if err != nil {
		return TrashListing{}, err
	}

	if listing.Entries, err = h.visible(ctx, actor, listing.Entries); err != nil {
		return TrashListing{}, err
	}
	// After the narrowing, so nothing of a deletion the actor may not read is read on their behalf.
	if listing.held, err = h.heldBatches(ctx, actor, listing.Entries); err != nil {
		return TrashListing{}, err
	}
	return listing, nil
}

// heldBatches answers which of these deletions a legal hold keeps.
//
// Every row of each deletion is judged, as the retention pass judges it: an entry by its hub, its
// collection, its path and whose data it carries, a container by itself and its hub. One row held
// keeps the whole deletion, because the pass removes a row only with everything below it and the
// root is above every other row of its batch. No statement beyond the hold list while no hold is
// in force, and none for the contributors while no hold names a person.
func (h ListTrash) heldBatches(
	ctx context.Context, actor appshared.ActorContext, entries []domain.TrashEntry,
) (map[shared.ID]bool, error) {
	held := map[shared.ID]bool{}
	if len(entries) == 0 {
		return held, nil
	}
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		listed, err := h.Holds.List(ctx, false)
		if err != nil {
			return err
		}
		holds := lifecycle.Holds(listed)
		if len(holds) == 0 {
			return nil
		}

		batches := make([]shared.ID, 0, len(entries))
		for _, entry := range entries {
			batches = append(batches, entry.BatchID)
		}
		members, err := h.Trash.BatchMembers(ctx, batches)
		if err != nil {
			return err
		}

		contributors := map[shared.ID][]shared.ID{}
		if holds.AnyOnAccounts() {
			items := make([]shared.ID, 0, len(members))
			for _, member := range members {
				if member.Kind == domain.TrashItemKind {
					items = append(items, member.ID)
				}
			}
			if contributors, err = h.Contributors.Contributors(ctx, items); err != nil {
				return err
			}
		}

		for _, member := range members {
			if held[member.BatchID] {
				continue
			}
			target := lifecycle.Target{ContainerIDs: presentIDs(member.HubID, member.ID)}
			if member.Kind == domain.TrashItemKind {
				target = lifecycle.Target{
					ItemID:          member.ID,
					ContainerIDs:    presentIDs(member.HubID, member.CollectionID),
					AncestorItemIDs: domain.PathIDs(member.Path),
					Contributors:    contributors[member.ID],
				}
			}
			if _, blocked := holds.Blocking(target); blocked {
				held[member.BatchID] = true
			}
		}
		return nil
	})
	return held, err
}

// presentIDs leaves out the empty ones: a hub has no hub above it.
func presentIDs(ids ...shared.ID) []shared.ID {
	out := make([]shared.ID, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() {
			out = append(out, id)
		}
	}
	return out
}

// trashPolicy is the period the retention pass reads. A tenant the pass has not reached yet has no
// row, and the documented default is what the pass is about to write for it; writing it here is not
// an option, because this read is read-only.
func (h ListTrash) trashPolicy(ctx context.Context) (lifecycle.Policy, error) {
	policy, err := h.Policies.Find(ctx, lifecycle.KindTrash)
	if errors.Is(err, shared.ErrNotFound) {
		for _, fallback := range lifecycle.DefaultPolicies() {
			if fallback.DataKind == lifecycle.KindTrash {
				return fallback, nil
			}
		}
	}
	return policy, err
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
		output := trashEntryOutput(entry)
		output["purge_after"] = page.PurgeAfter(entry)
		output["held"] = page.Held(entry)
		entries = append(entries, output)
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
