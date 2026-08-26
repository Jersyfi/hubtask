// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// QueryItemsName is the catalogue name, as domain-model.md §5 writes it under "Views & query".
const QueryItemsName = "QueryItems"

// QueryItems answers the query language: one filter over one anchored scope, sorted, paged, and
// optionally grouped into the columns a board draws (api-guidelines.md §3, B-12).
//
// It lives in this package rather than in a `view` one of its own, although the catalogue groups it
// with the saved views. What it reads is work items, and it answers them through this package's
// projection, its authorisation path, its page size and its label expansion - four things a second
// package would have to import or copy. `view` is for SavedView, which stores a query rather than
// running one; the *grammar* is already in core/domain/model/view, where project-structure.md §1
// puts it.
type QueryItems struct {
	Items      repository.Items
	ItemLabels repository.ItemLabels
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute answers the query.
//
// Three steps, in this order. The placeholders are resolved first because it costs nothing and a
// query that cannot be resolved is one nobody should read a row for. The anchor is then read and
// the permission asked about it - once, for the whole result, exactly as ListWorkItems asks about
// the collection it was given: the client named the scope, so a refusal is a refusal rather than an
// empty page - and somebody who holds only individual shares inside it queries those (C-04). Only
// then does the compiled statement run.
func (h QueryItems) Execute(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec,
) (repository.ItemQueryResult, error) {
	resolved, err := h.resolvePlaceholders(actor, spec)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	anchor, container, err := h.anchor(ctx, actor, resolved.Scope)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(container),
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: containerTarget,
		TargetID:   container.ID,
	}); err != nil {
		return repository.ItemQueryResult{}, err
	}

	var result repository.ItemQueryResult
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		result, err = h.Items.Query(ctx, repository.ItemSearch{
			Anchor: anchor, Spec: resolved,
			// The searcher's language, for MATCHES to read the words under. The entry's own is
			// what its document was built under, and the two are deliberately different questions
			// (ADR-0034, ItemSearch.Language).
			Language: actor.Locale,
		})
		return err
	})
	if err != nil {
		return repository.ItemQueryResult{}, err
	}
	return result, nil
}

// resolvePlaceholders replaces `@me` and the date anchors with the values only the server knows.
//
// The zone is the actor's, which is what api-guidelines.md §3 means by "resolved server-side in the
// actor's time zone": somebody in Auckland asking for what is due today is asking about their day.
// A zone the installation cannot resolve falls back to UTC rather than refusing - the preference
// was accepted when it was stored, and a query is not where to discover that the zone database has
// moved on.
func (h QueryItems) resolvePlaceholders(
	actor appshared.ActorContext, spec view.Spec,
) (view.Spec, error) {
	if spec.Filter == nil {
		return spec, nil
	}

	location := time.UTC
	if actor.TimeZone != "" {
		if loaded, err := time.LoadLocation(actor.TimeZone); err == nil {
			location = loaded
		}
	}

	resolved, err := spec.Filter.Resolve(view.Resolution{
		Now:      h.Clock.Now(),
		Location: location,
		ActorID:  actor.AccountID,
	}, "/filter")
	if err != nil {
		return view.Spec{}, err
	}
	spec.Filter = &resolved
	return spec, nil
}

// anchor reads what the scope names and reports both the resolved anchor and the container the
// permission is asked about.
//
// A hub, a collection and an entry are three different reads and one question. The container comes
// back as well as the anchor because it is what the path is built from: a membership held at the
// hub applies downwards, so the check needs the container's place in the tree rather than its
// identifier (domain-model.md §3.2).
func (h QueryItems) anchor(
	ctx context.Context, actor appshared.ActorContext, scope view.Scope,
) (repository.Anchor, domain.Container, error) {
	var (
		anchor    repository.Anchor
		container domain.Container
	)

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !scope.ItemID.IsZero() {
			item, err := h.Items.Find(ctx, scope.ItemID)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return scopeNotFound("/scope/item_id", scope.ItemID)
				}
				return err
			}
			collection, err := findCollection(ctx, h.Containers, item.CollectionID)
			if err != nil {
				return err
			}

			container = collection
			anchor = repository.Anchor{
				Kind:               repository.AnchorItem,
				CollectionID:       item.CollectionID,
				ItemID:             item.ID,
				PathPrefix:         item.Path,
				IncludeDescendants: scope.IncludeDescendants,
			}
			return nil
		}

		found, err := h.Containers.Find(ctx, scope.ContainerID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return scopeNotFound("/scope/container_id", scope.ContainerID)
			}
			return err
		}

		container = found
		if found.Type == domain.ContainerHub {
			// A hub holds no items of its own; what it scopes is the items of its collections. The
			// flag is therefore not passed on - "one level below a hub" would be a level of
			// collections, and this query answers entries.
			anchor = repository.Anchor{
				Kind: repository.AnchorHub, HubID: found.ID, IncludeDescendants: true,
			}
			return nil
		}
		anchor = repository.Anchor{
			Kind:               repository.AnchorCollection,
			CollectionID:       found.ID,
			IncludeDescendants: scope.IncludeDescendants,
		}
		return nil
	})
	if err != nil {
		return repository.Anchor{}, domain.Container{}, err
	}
	return anchor, container, nil
}

// scopeNotFound is the one answer for a scope that is not there and for one that belongs to another
// tenant. Row level security has already made the second look like the first, and telling them
// apart would confirm that an identifier exists somewhere (multi-tenancy.md §2).
func scopeNotFound(path string, id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("query.scope_not_found").
		WithParams(map[string]string{"scope": id.String()}).
		WithFields(shared.FieldError{Path: path, Code: "query.scope_not_found"})
}

// Descriptor is the catalogue entry.
//
// The filter, the ordering and the grouping are declared as documents rather than as fields of
// their own, because they are a grammar: what may go in them is answered by
// `GET /meta/capabilities` under `query_fields`, which is a list this installation computes and not
// one a schema could pin. An agent reads that manifest for the same reason a client does
// (ai-first.md §1.1).
func (h QueryItems) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: QueryItemsName,
		Summary: "Searches tasks, work packages and activities inside one hub, collection or " +
			"entry: a filter of field, operator and value combined with AND, OR and NOT, an " +
			"ordering, cursor paging, and an optional grouping into the columns a board draws. " +
			"The fields and operators this installation serves are listed by /meta/capabilities " +
			"under query_fields; anything else is refused by name rather than ignored.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "scope_container_id", Kind: usecase.KindID,
				Description: "The hub or collection to search in. Exactly one scope is required.",
			},
			{
				Name: "scope_item_id", Kind: usecase.KindID,
				Description: "The entry whose subtree is searched. Exactly one scope is required.",
			},
			{
				Name: "include_descendants", Kind: usecase.KindBool,
				Description: "Searches the whole subtree, which is the default. False narrows the " +
					"scope to one level: the entries directly in the collection, or the direct " +
					"children of the entry.",
			},
			{
				Name: "filter", Kind: usecase.KindObject,
				Description: "The filter tree: a leaf of field, op and value, or a combination of " +
					"op AND, OR or NOT with nodes. At most five levels and fifty nodes. A value " +
					"beginning with @ is resolved on the server: @me, @today, @end_of_month, each " +
					"optionally with an ISO 8601 offset such as @today+P3D.",
			},
			{
				Name: "sort", Kind: usecase.KindList,
				Description: "The ordering, most significant first: objects of field, dir (ASC or " +
					"DESC) and nulls (FIRST or LAST). Defaults to the manual order, and always " +
					"ends on the identifier so that a cursor is unambiguous.",
			},
			{
				Name: "group_by", Kind: usecase.KindObject,
				Description: "The board projection: field, and limit_per_group. Each group " +
					"carries its own rows and its own cursor. A grouped query takes no cursor of " +
					"its own - a column is paged by asking for that column.",
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
				Description: "Rows per page, 1 to 200. Defaults to 50.",
			},
			{
				Name: "count", Kind: usecase.KindString,
				Enum: []string{string(view.CountNone), string(view.CountEstimated), string(view.CountExact)},
				Description: "`exact` answers the size of the whole result in a second query. " +
					"`estimated` is not served.",
			},
			{
				Name: "expand_labels", Kind: usecase.KindBool,
				Description: "Includes the labels each entry carries, read for the whole result in " +
					"one query. Omitted leaves the field out of the answer entirely.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h QueryItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	spec, err := specOf(in)
	if err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, spec)
	if err != nil {
		return nil, err
	}

	expand := in.Bool("expand_labels")
	carried, err := labelsOf(ctx, h.UnitOfWork, h.ItemLabels, actor, expand, everyItem(result))
	if err != nil {
		return nil, err
	}
	return queryOutput(result, spec, carried), nil
}

// specOf turns what a channel sent into the specification the domain validated.
//
// Every part goes through core/domain/model/view, and none of them is read twice: the grammar
// decides what a field is, what an operator may compare, and what a value has to look like, so this
// function contains no rule of its own beyond the page size the whole product shares.
func specOf(in usecase.Input) (view.Spec, error) {
	containerID, err := in.ID("scope_container_id")
	if err != nil {
		return view.Spec{}, err
	}
	itemID, err := in.ID("scope_item_id")
	if err != nil {
		return view.Spec{}, err
	}

	// Absent means the whole subtree, so the flag is read through Present rather than through Bool:
	// a missing boolean is false, and false is the other instruction.
	includeDescendants := true
	if in.Present("include_descendants") {
		includeDescendants = in.Bool("include_descendants")
	}

	scope, err := view.ParseScope(containerID, itemID, includeDescendants, "/scope")
	if err != nil {
		return view.Spec{}, err
	}
	filter, err := view.ParseFilter(in["filter"], "/filter")
	if err != nil {
		return view.Spec{}, err
	}
	sort, err := view.ParseSort(in["sort"], "/sort")
	if err != nil {
		return view.Spec{}, err
	}
	groupBy, err := view.ParseGroupBy(in["group_by"], "/group_by")
	if err != nil {
		return view.Spec{}, err
	}
	count, err := view.ParseCount(in.String("count"), "/count")
	if err != nil {
		return view.Spec{}, err
	}

	spec := view.Spec{
		Scope:           scope,
		Filter:          filter,
		Sort:            sort,
		GroupBy:         groupBy,
		IncludeArchived: in.Bool("include_archived"),
		IncludeTrashed:  in.Bool("include_trashed"),
		Cursor:          in.String("cursor"),
		Size:            PageSize(in.Int("size")),
		Count:           count,
	}
	if err := spec.Validate(""); err != nil {
		return view.Spec{}, err
	}
	return spec, nil
}

// everyItem is the whole result as one slice: what the label expansion is asked about, whichever
// shape the answer took.
func everyItem(result repository.ItemQueryResult) []domain.WorkItem {
	items := result.Items
	for _, group := range result.Groups {
		items = append(items, group.Items...)
	}
	return items
}

// queryOutput is the answer's shape: `{ data, groups, page, total }` (api-guidelines.md §3).
//
// `data` and `groups` are both always present and the one that does not apply is empty, so that a
// client reads either unconditionally - the same reasoning that puts `parent_id: null` in an item
// rather than leaving the field out.
func queryOutput(
	result repository.ItemQueryResult, spec view.Spec, carried map[shared.ID][]shared.ID,
) usecase.Output {
	out := pageOutput(rowsOf(result.Items, carried), result.Info)
	out["groups"] = groupsOf(result, spec, carried)
	out["total"] = nil
	if spec.Count == view.CountExact {
		out["total"] = result.Total
	}
	return out
}

func groupsOf(
	result repository.ItemQueryResult, spec view.Spec, carried map[shared.ID][]shared.ID,
) []usecase.Output {
	groups := make([]usecase.Output, 0, len(result.Groups))

	for _, group := range result.Groups {
		out := pageOutput(rowsOf(group.Items, carried), group.Info)
		out["key"] = nil
		if !group.Absent {
			out["key"] = group.Key
		}
		out["count"] = nil
		if spec.Count == view.CountExact {
			out["count"] = group.Total
		}
		groups = append(groups, out)
	}
	return groups
}

func rowsOf(items []domain.WorkItem, carried map[shared.ID][]shared.ID) []usecase.Output {
	rows := make([]usecase.Output, 0, len(items))
	for _, item := range items {
		rows = append(rows, withLabels(ItemOutput(item), item, carried))
	}
	return rows
}
