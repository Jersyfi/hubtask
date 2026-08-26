// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	// SearchItemsName is the catalogue name, as domain-model.md §5 writes it under "Search".
	SearchItemsName = "SearchItems"
	// searchTarget is what an audit entry about a search names. Its own target type rather than
	// the entry's, for the reason the trash has one: a search is one view over everything, and a
	// refused search is a refusal of the view rather than of any entry in it - there is no single
	// entry to name, which is what the batch question is (ReadTrash, C-04).
	searchTarget = "search"
)

// SearchItems answers "where is this" (C-08).
//
// The one read of this product that is not anchored to a place in the workspace. A query is
// anchored because an unanchored one is a question authorisation cannot answer in one step
// (view.Scope); a search is the question "anywhere I can see", so it is answered the way the trash
// and the hub level are - read, then narrowed to what the actor may see rather than refused
// (ListTrash, ReadContainers.Execute).
//
// Which makes the narrowing the security-critical half of this use case rather than a detail of
// it. A search that returned a title from a collection the actor cannot open is T-04 with a
// different verb, and no scope check upstream can prevent it: the rows come from everywhere at
// once. It is asked here, in the application layer, like every other permission (rule 2, ADR-0005).
type SearchItems struct {
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	Anchored   Anchored
	Reader     Reader
	UnitOfWork persistence.UnitOfWork
}

// SearchItemsQuery is the input, typed.
type SearchItemsQuery struct {
	Words string
	// ContainerID narrows the search to one hub or collection. Empty searches everywhere.
	ContainerID shared.ID
	// Language is the tag the words are read under. Empty takes the actor's locale.
	Language        string
	IncludeArchived bool
	IncludeTrashed  bool
	Cursor          string
	Size            int
}

// Execute answers one page of hits, in the order the database ranked them.
func (h SearchItems) Execute(
	ctx context.Context, actor appshared.ActorContext, query SearchItemsQuery,
) (repository.ItemHitPage, error) {
	words, err := view.ParseSearch(query.Words, "/q")
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	request := view.Search{
		Words:           words,
		ContainerID:     query.ContainerID,
		Language:        languageOr(query.Language, actor.Locale),
		IncludeArchived: query.IncludeArchived,
		IncludeTrashed:  query.IncludeTrashed,
		Cursor:          query.Cursor,
		Size:            PageSize(query.Size),
	}
	if err := request.Validate(""); err != nil {
		return repository.ItemHitPage{}, err
	}

	reach, err := h.reach(ctx, actor, request.ContainerID)
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	var page repository.ItemHitPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Items.Search(ctx, repository.TextSearch{
			Anchor: reach.anchor, Request: request, RestrictTo: reach.restrictTo,
		})
		return err
	})
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	if reach.narrow {
		if page.Hits, err = h.visible(ctx, actor, page.Hits); err != nil {
			return repository.ItemHitPage{}, err
		}
	}
	return page, nil
}

// scopeReach is what the permission question answered: where to look, which entries to keep, and
// whether the answer still has to be narrowed row by row.
type scopeReach struct {
	anchor     repository.Anchor
	restrictTo []shared.ID
	narrow     bool
}

// reach asks the permission question in the shape the scope makes it.
//
// Three shapes, and each one is the shape an existing read already has - deliberately, because a
// fourth answer to "may this actor see these entries" is a fourth place for it to be wrong.
//
//   - No scope: nothing to ask about. The rows span hubs, so the page is read and then narrowed,
//     exactly as the trash is (ListTrash.Execute).
//   - A collection: how far the actor reaches into it, exactly as the plain level list asks
//     (ListWorkItems.Execute). A role on the collection answers for every entry in it; somebody who
//     holds none may still hold memberships on entries inside it, and their search is those
//     entries (C-04).
//   - A hub: the client named it, so a refusal is a refusal rather than an empty page, exactly as
//     the query language answers (QueryItems.Execute). A role at the hub or above covers
//     everything in it, so nothing is left to narrow.
func (h SearchItems) reach(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID,
) (scopeReach, error) {
	if containerID.IsZero() {
		return scopeReach{anchor: repository.Anchor{Kind: repository.AnchorTenant}, narrow: true}, nil
	}

	var container domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Containers.Find(ctx, containerID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return scopeNotFound("/scope/container_id", containerID)
			}
			return err
		}
		container = found
		return nil
	})
	if err != nil {
		return scopeReach{}, err
	}

	request := access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(container),
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: containerTarget,
		TargetID:   container.ID,
	}

	if container.Type == domain.ContainerHub {
		if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
			return scopeReach{}, err
		}
		return scopeReach{anchor: repository.Anchor{
			Kind: repository.AnchorHub, HubID: container.ID, IncludeDescendants: true,
		}}, nil
	}

	reached, err := h.Anchored.ReachInto(ctx, actor, request, container.ID)
	if err != nil {
		return scopeReach{}, err
	}
	return scopeReach{
		anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: container.ID, IncludeDescendants: true,
		},
		restrictTo: reached.Shared,
	}, nil
}

// visible drops the hits the actor may not read.
//
// One question for the whole page, asked of each row's own path. The path ends on the entry rather
// than on its collection, which is what makes an entry shared with the actor individually visible
// here: a membership held anywhere along the path counts, and "shared items only" is a membership
// at ITEM scope (domain-model.md §3.2, C-04).
//
// The page's own cursor is left alone, for the reason the trash leaves it alone: it is a boundary
// in the *scanned* set, not in the filtered one. A client sees shorter pages and walks on until
// has_more is false; narrowing the cursor to the last visible row would skip everything between it
// and the last row actually read.
func (h SearchItems) visible(
	ctx context.Context, actor appshared.ActorContext, hits []repository.ItemHit,
) ([]repository.ItemHit, error) {
	if len(hits) == 0 {
		return hits, nil
	}

	paths := make([][]identity.Scope, 0, len(hits))
	for _, hit := range hits {
		paths = append(paths, hitPath(hit))
	}

	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: searchTarget,
	}, paths)
	if err != nil {
		return nil, err
	}

	visible := make([]repository.ItemHit, 0, len(hits))
	for i, hit := range hits {
		if i < len(allowed) && allowed[i] {
			visible = append(visible, hit)
		}
	}
	return visible, nil
}

// hitPath is the scope chain one hit is judged against: the tenant, the hub the entry's collection
// sits in, the collection, and the entry itself.
//
// Built from the projection rather than from a re-read, which is what the hub on the hit is for: a
// membership held at a hub applies downwards, so a path that named only the collection could not
// show an entry to somebody whose right sits above it.
func hitPath(hit repository.ItemHit) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !hit.HubID.IsZero() {
		path = append(path, identity.HubScope(hit.HubID))
	}
	return append(path,
		identity.CollectionScope(hit.Item.CollectionID), identity.ItemScope(hit.Item.ID))
}

// Descriptor is the catalogue entry.
func (h SearchItems) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SearchItemsName,
		Summary: "Searches the text of tasks, work packages and activities - their titles and " +
			"their notes - and answers them in the order they match, best first. The words are " +
			"read in a language: an entry is indexed under the language it was written in, and " +
			"the query under the caller's, so a German search finds an inflected German word and " +
			"an English one finds a stemmed English word. A language whose script has no word " +
			"boundaries, such as Japanese or Thai, additionally matches on substrings. " +
			"Unanchored by default - it searches everything the caller may see - and narrowed to " +
			"that rather than refused, so a page can be shorter than the size asked for; walk on " +
			"until has_more is false.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "q", Kind: usecase.KindString, Required: true,
				Description: "What to look for. Quoted phrases, `or` between words and a leading " +
					"minus for exclusion work as they do in a web search box. At most 200 characters.",
			},
			{
				Name: "container_id", Kind: usecase.KindID,
				Description: "The hub or collection to search in. Omitted searches everywhere the " +
					"caller may see.",
			},
			{
				Name: "language", Kind: usecase.KindString,
				Description: "The language the words are in, as a BCP 47 tag. Omitted takes the " +
					"caller's locale. It decides how the words are read, not which entries are " +
					"searched - an entry is always found by a query in the language it was " +
					"written in.",
			},
			{
				Name: "include_archived", Kind: usecase.KindBool,
				Description: "Keeps archived entries in the result.",
			},
			{
				Name: "include_trashed", Kind: usecase.KindBool,
				Description: "Keeps entries that are in the trash in the result.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The next_cursor of the previous page. Opaque: it is produced by this " +
					"server and is not to be constructed or parsed.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "Hits per page, 1 to 200. Defaults to 50.",
			},
		},
		// Declared and not required, as every other read declares it: an ordinary search writes no
		// audit entry, and a refused one does - recorded by the authorisation service against the
		// action that was refused (audit.md §4).
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: searchTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SearchItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, SearchItemsQuery{
		Words:           in.String("q"),
		ContainerID:     containerID,
		Language:        in.String("language"),
		IncludeArchived: in.Bool("include_archived"),
		IncludeTrashed:  in.Bool("include_trashed"),
		Cursor:          in.String("cursor"),
		Size:            in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Hits))
	for _, hit := range page.Hits {
		rows = append(rows, ItemOutput(hit.Item))
	}
	return pageOutput(rows, page.Info), nil
}
