// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
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
)

// AggregateItemsName is the catalogue name (core/application/catalogue).
const AggregateItemsName = "AggregateItems"

// AggregateItems summarises what a query keeps: how many entries, and the sum, average, least or
// greatest of a custom number, money or duration or of a built-in quantity - per value of a field
// and per day, week or month (S-11). The scope, the filter, the permission and the shaping are the
// query's own (QueryItems), so a summary never counts an entry the same query would not list.
//
// The whole workspace is the one scope a query does not take: the collections the actor reads are
// decided here, each by its own path, and handed to the statement as one bound list - the
// narrowing inside the statement that a page of rows cannot have. An entry shared with the actor
// alone, in a collection they hold no role on, is counted as a level list shows it: each such
// entry judged at its own path, and bound as a second list.
type AggregateItems struct {
	Query      QueryItems
	Aggregates repository.ItemAggregates
	// Collections and Reader decide a workspace summary's collections; unwired, it is refused.
	Collections repository.Collections
	Reader      Reader
	// Shared answers the entries shared with the actor alone; unwired, none is counted.
	Shared identityrepo.SharedEntries
}

// MaxSummaryCollections bounds the collections a workspace summary reads: the bound list is one
// parameter, and past this size a summary belongs to a hub. MaxSummaryShared bounds the entries
// shared with the actor alone the same way.
const (
	MaxSummaryCollections = 2000
	MaxSummaryShared      = 5000
)

// Execute answers the summary.
func (h AggregateItems) Execute(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec, aggregation view.Aggregation,
) (repository.ItemAggregateResult, error) {
	q := h.Query
	resolved, err := q.resolvePlaceholders(actor, spec)
	if err != nil {
		return repository.ItemAggregateResult{}, err
	}

	var (
		anchor repository.Anchor
		path   = []identity.Scope{identity.TenantScope()}
		scope  shared.ID
	)
	if resolved.Scope.Workspace {
		if anchor, err = h.workspace(ctx, actor, resolved.IncludeArchived); err != nil {
			return repository.ItemAggregateResult{}, err
		}
	} else {
		var container domain.Container
		anchor, container, err = q.anchor(ctx, actor, resolved.Scope)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		path = containerPath(container)
		if err := q.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionRead,
			Path:       path,
			Action:     ItemReadAction,
			TokenScope: itemsRead,
			TargetType: containerTarget,
			TargetID:   container.ID,
		}); err != nil {
			return repository.ItemAggregateResult{}, err
		}
		scope = anchor.CollectionID
		if anchor.Kind == repository.AnchorHub {
			scope = anchor.HubID
		}
	}
	// A field hidden from the reader is no more summed than it is filtered by (UC-ID-27 check 3).
	if q.Gate.Sight.Wired() {
		lens, err := q.Gate.Sight.Lens(ctx, q.UnitOfWork, actor)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		keys := append(resolved.CustomKeys(), aggregation.CustomKeys()...)
		hidden, err := lens.HiddenFrom(ctx, path, keys)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		if err := refuseHidden(hidden); err != nil {
			return repository.ItemAggregateResult{}, err
		}
	}

	// The zone resolves as the placeholders' does: one the zone database no longer knows is UTC.
	location := "UTC"
	if _, err := time.LoadLocation(actor.TimeZone); err == nil && actor.TimeZone != "" {
		location = actor.TimeZone
	}

	var result repository.ItemAggregateResult
	err = q.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		shaped, err := shapeCustomFields(ctx, q.Fields, resolved, scope)
		if err != nil {
			return err
		}
		summary, err := shapeAggregation(ctx, q.Fields, aggregation, scope)
		if err != nil {
			return err
		}
		result, err = h.Aggregates.Aggregate(ctx, repository.ItemSearch{
			Anchor: anchor, Spec: shaped, Language: actor.Locale,
		}, summary, location)
		return err
	})
	return result, err
}

// workspace decides the collections a workspace summary reads: every live collection - archived
// ones too when the query keeps archived entries -, private hubs the actor is in included, kept where reading its entries is permitted at its own path - the
// question QueryItems asks of one collection, asked of each. A collection left out is nobody's
// refusal, so nothing is recorded per row (audit.md §4).
func (h AggregateItems) workspace(
	ctx context.Context, actor appshared.ActorContext, includeArchived bool,
) (repository.Anchor, error) {
	if err := actor.RequireScope(itemsRead); err != nil {
		return repository.Anchor{}, err
	}
	if h.Collections == nil || h.Reader == nil {
		return repository.Anchor{}, shared.ErrInternal.WithDetail("query.workspace_unwired")
	}

	var (
		rows   []repository.CollectionInHub
		shares []identityrepo.SharedEntry
	)
	err := h.Query.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if h.Shared != nil {
			read, err := h.Shared.SharedAcross(ctx, actor.AccountID, MaxSummaryShared+1)
			if err != nil {
				return err
			}
			if len(read) > MaxSummaryShared {
				return shared.ErrValidation.WithDetail("query.workspace_too_large").
					WithParams(map[string]string{"maximum": strconv.Itoa(MaxSummaryShared)}).
					WithFields(shared.FieldError{Path: "/scope/workspace", Code: "query.workspace_too_large"})
			}
			shares = read
		}
		page := repository.Page{Size: 200}
		for {
			read, err := h.Collections.InOrder(ctx, repository.CollectionQuery{
				IncludePrivate: true, IncludeArchived: includeArchived, Page: page,
			})
			if err != nil {
				return err
			}
			rows = append(rows, read.Collections...)
			if len(rows) > MaxSummaryCollections {
				return shared.ErrValidation.WithDetail("query.workspace_too_large").
					WithParams(map[string]string{"maximum": strconv.Itoa(MaxSummaryCollections)}).
					WithFields(shared.FieldError{Path: "/scope/workspace", Code: "query.workspace_too_large"})
			}
			if !read.Info.HasMore {
				return nil
			}
			page.Cursor = read.Info.NextCursor
		}
	})
	if err != nil {
		return repository.Anchor{}, err
	}

	anchor := repository.Anchor{Kind: repository.AnchorCollections, IncludeDescendants: true}
	if len(rows) == 0 {
		return anchor, nil
	}
	paths := make([][]identity.Scope, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, containerPath(row.Collection))
	}
	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: containerTarget,
	}, paths)
	if err != nil {
		return repository.Anchor{}, err
	}
	unread := make(map[shared.ID]domain.Container, len(rows))
	for i, row := range rows {
		if i < len(allowed) && allowed[i] {
			anchor.CollectionIDs = append(anchor.CollectionIDs, row.Collection.ID)
			continue
		}
		unread[row.Collection.ID] = row.Collection
	}
	anchor.ItemIDs, err = h.sharedReached(ctx, actor, shares, unread)
	return anchor, err
}

// sharedReached keeps the entries shared with the actor alone that lie in a live collection the
// actor does not read, each judged at its own path - an entry in a readable collection is
// counted already, and one in a collection the summary does not read is not counted at all.
func (h AggregateItems) sharedReached(
	ctx context.Context, actor appshared.ActorContext, shares []identityrepo.SharedEntry,
	unread map[shared.ID]domain.Container,
) ([]shared.ID, error) {
	var (
		candidates []shared.ID
		paths      [][]identity.Scope
	)
	for _, entry := range shares {
		collection, found := unread[entry.CollectionID]
		if !found {
			continue
		}
		candidates = append(candidates, entry.ItemID)
		paths = append(paths, append(containerPath(collection), identity.ItemScope(entry.ItemID)))
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	var kept []shared.ID
	for i, id := range candidates {
		if i < len(allowed) && allowed[i] {
			kept = append(kept, id)
		}
	}
	return kept, nil
}

// shapeAggregation puts the definitions' shapes on the custom fields the aggregation names, as
// shapeCustomFields does for the query.
func shapeAggregation(
	ctx context.Context, fields repository.CustomFields, aggregation view.Aggregation, scope shared.ID,
) (view.Aggregation, error) {
	keys := aggregation.CustomKeys()
	shapes := make(map[string]view.CustomShape, len(keys))
	if len(keys) > 0 {
		if fields == nil {
			return view.Aggregation{}, shared.ErrInternal.WithDetail("query.custom_fields_unwired")
		}
		kinds, err := fields.KindsByKey(ctx, keys, scope)
		if err != nil {
			return view.Aggregation{}, err
		}
		for _, key := range keys {
			shapes[key] = view.ShapeOf(kinds[key])
		}
	}
	return aggregation.Shaped(shapes)
}

// Descriptor is the catalogue entry: the query's scope and filter, and the aggregation.
func (h AggregateItems) Descriptor() usecase.Descriptor {
	query := QueryItems{}.Descriptor()
	input := []usecase.Field{}
	for _, field := range query.Input {
		switch field.Name {
		case "scope_container_id", "scope_item_id", "include_descendants", "filter",
			"include_archived", "include_trashed":
			input = append(input, field)
		}
	}
	input = append(input, usecase.Field{
		Name: "scope_workspace", Kind: usecase.KindBool,
		Description: "The whole workspace instead of one hub, collection or entry: every " +
			"collection the caller may read, at most 2000, archived ones with include_archived; " +
			"an entry shared with the caller alone in a collection they do not read is counted " +
			"too, at most 5000 of them.",
	})
	input = append(input, usecase.Field{
		Name: "aggregate", Kind: usecase.KindObject, Required: true,
		Description: "The summary: function (COUNT, SUM, AVG, MIN or MAX), field (a custom field " +
			"holding a number, money or a duration, or lead_time - creation to completion - or " +
			"scheduled_span - start to due -, both in minutes; not for COUNT), group_by (a field a " +
			"query may group by), and bucket (field, a timestamp, and period: day, week or month, " +
			"cut in the caller's time zone; week_start monday, the default, or sunday).",
	})
	return usecase.Descriptor{
		Name:       AggregateItemsName,
		Permission: service.PermissionRead,
		Summary: "Summarises the entries a query keeps inside one hub, collection or entry, or " +
			"the whole workspace: how many, and the sum, average, least or greatest of a custom " +
			"number, money or duration or of an entry's lead time or scheduled span, " +
			"optionally per value of a field and per day, week or month. At most 5000 rows; a " +
			"longer answer says it was cut.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input:       input,
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AggregateItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	spec, aggregation, err := summaryOf(in)
	if err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, spec, aggregation)
	if err != nil {
		return nil, err
	}
	return aggregateOutput(aggregation, result), nil
}

// summaryOf reads a summary's input - the scope, the filter, the lifecycle flags and the
// aggregation - from the flat use case input a request or a saved view's document is.
func summaryOf(in usecase.Input) (view.Spec, view.Aggregation, error) {
	containerID, err := in.ID("scope_container_id")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	itemID, err := in.ID("scope_item_id")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	includeDescendants := true
	if in.Present("include_descendants") {
		includeDescendants = in.Bool("include_descendants")
	}
	scope, err := view.ParseSummaryScope(
		containerID, itemID, includeDescendants, in.Bool("scope_workspace"), "/scope")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	filter, err := view.ParseFilter(in["filter"], "/filter")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	aggregation, err := view.ParseAggregation(in["aggregate"], "/aggregate")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	return view.Spec{
		Scope: scope, Filter: filter,
		IncludeArchived: in.Bool("include_archived"), IncludeTrashed: in.Bool("include_trashed"),
	}, aggregation, nil
}

// aggregateOutput is a summary's answer as every door returns it.
func aggregateOutput(aggregation view.Aggregation, result repository.ItemAggregateResult) usecase.Output {
	rows := make([]usecase.Output, 0, len(result.Rows))
	for _, row := range result.Rows {
		out := usecase.Output{"count": row.Count, "group": nil, "period": nil, "value": nil}
		if aggregation.IsGrouped() && row.Group != "" {
			out["group"] = row.Group
		}
		if aggregation.IsBucketed() && row.Period != "" {
			out["period"] = row.Period
		}
		if row.Value != nil {
			out["value"] = *row.Value
		}
		rows = append(rows, out)
	}
	return usecase.Output{"data": rows, "truncated": result.Truncated}
}
